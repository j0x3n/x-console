package music

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/settings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

const (
	maxAudioSize = 1 << 30
	maxLRCSize   = 1 << 20
)

// coverNames are the file names (without extension) that count as the cover
// of a folder, in order of preference.
var coverNames = []string{"cover", "folder", "front"}

var coverExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

// scanResult counts what a scan changed.
type scanResult struct{ Added, Updated, Removed int }

// folders returns the drive folder ids saved in the settings.
func (m *Module) folders(ctx context.Context) ([]int64, error) {
	var ids []int64
	if err := m.d.Settings.Get(ctx, FoldersKey, &ids); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return nil, err
	}
	return ids, nil
}

// Reconcile makes the library match the music folders: new songs are added,
// changed ones are read again, songs that left the folders are removed. Only
// one runs at a time. A failure to list the drive changes nothing.
func (m *Module) Reconcile(ctx context.Context) error {
	select {
	case m.scanLock <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-m.scanLock }()

	ids, err := m.folders(ctx)
	if err != nil {
		return err
	}
	drive, err := m.drive()
	if err != nil {
		return err
	}
	var all []contracts.DriveFile
	if len(ids) > 0 {
		exts := []string{".lrc"}
		for e := range audioMimes {
			exts = append(exts, e)
		}
		for e := range coverExts {
			exts = append(exts, e)
		}
		if all, err = drive.ListFiles(ctx, ids, exts); err != nil {
			return err
		}
	}
	roots := map[int64]bool{}
	for _, id := range ids {
		roots[id] = true
	}

	siblings := map[int64][]contracts.DriveFile{}
	var audio []contracts.DriveFile
	for _, f := range all {
		if _, ok := audioMimes[strings.ToLower(filepath.Ext(f.Name))]; ok {
			if f.Size > 0 && f.Size <= maxAudioSize {
				audio = append(audio, f)
			}
			continue
		}
		siblings[f.ParentID] = append(siblings[f.ParentID], f)
	}

	rows, err := m.q.ListTrackSync(ctx)
	if err != nil {
		return err
	}
	existing := map[int64]db.ListTrackSyncRow{}
	for _, r := range rows {
		existing[r.DriveItemID] = r
	}

	var res scanResult
	seen := map[int64]bool{}
	covers := map[string]string{} // cover image sha256 -> stored cover hash, "" when unusable
	for _, f := range audio {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen[f.ID] = true
		lrc, img := companions(f, siblings[f.ParentID])
		sig := sha(lrc) + "|" + sha(img)
		row, had := existing[f.ID]
		if had && row.Sha256 == f.SHA256 && row.Companion == sig {
			continue
		}
		added, err := m.indexFile(ctx, drive, f, lrc, img, sig, roots, covers, had)
		if err != nil {
			if errors.Is(err, contracts.ErrDriveNotFound) || ctx.Err() != nil {
				continue
			}
			slog.Warn("music: cannot index file", "file", f.ID, "err", err)
			continue
		}
		if added {
			res.Added++
		} else {
			res.Updated++
		}
	}
	for _, r := range rows {
		if !seen[r.DriveItemID] {
			if err := m.q.DeleteTrack(ctx, r.ID); err != nil {
				return err
			}
			res.Removed++
		}
	}
	if res != (scanResult{}) {
		m.d.Bus.Publish("music.library_changed", map[string]int{"added": res.Added, "updated": res.Updated, "removed": res.Removed})
	}
	return nil
}

func sha(f *contracts.DriveFile) string {
	if f == nil {
		return ""
	}
	return f.SHA256
}

// companions finds the .lrc with the same name and the cover image in the
// folder of a song.
func companions(song contracts.DriveFile, folder []contracts.DriveFile) (lrc, img *contracts.DriveFile) {
	base := strings.ToLower(strings.TrimSuffix(song.Name, filepath.Ext(song.Name)))
	rank := len(coverNames)
	for i := range folder {
		f := &folder[i]
		ext := strings.ToLower(filepath.Ext(f.Name))
		name := strings.ToLower(strings.TrimSuffix(f.Name, filepath.Ext(f.Name)))
		switch {
		case ext == ".lrc" && name == base && f.Size <= maxLRCSize:
			lrc = f
		case coverExts[ext] && f.Size <= maxCoverBytes:
			for r, n := range coverNames {
				if name == n && r < rank {
					rank, img = r, f
				}
			}
		}
	}
	return lrc, img
}

// indexFile reads one song and writes its row. added is true for a new row.
func (m *Module) indexFile(ctx context.Context, drive contracts.DriveFiles, f contracts.DriveFile, lrc, img *contracts.DriveFile,
	sig string, roots map[int64]bool, covers map[string]string, had bool) (added bool, err error) {

	info, readErr := m.readFromDrive(ctx, drive, f)
	if errors.Is(readErr, contracts.ErrDriveNotFound) {
		return false, readErr
	}
	if readErr != nil {
		// A file taglib cannot parse still gets a row, named after the file.
		slog.Debug("music: unreadable tags", "file", f.ID, "err", readErr)
	}

	ext := strings.ToLower(filepath.Ext(f.Name))
	nameTitle, nameArtist := titleFromName(f.Name)
	title, artist := info.Title, info.Artist
	if title == "" {
		title = nameTitle
		if artist == "" {
			artist = nameArtist
		}
	}
	if artist == "" && f.ParentID != 0 && !roots[f.ParentID] {
		artist = f.ParentName
	}

	lyrics, lyricsSource := info.Lyrics, "none"
	if lyrics != "" {
		lyricsSource = "embedded"
	} else if lrc != nil {
		if text := m.readText(ctx, drive, lrc.ID); strings.TrimSpace(text) != "" {
			lyrics, lyricsSource = text, "lrc"
		}
	}

	cover, coverSource := "", "none"
	if len(info.Cover) > 0 {
		if h, err := m.saveCover(ctx, info.Cover); err == nil {
			cover, coverSource = h, "embedded"
		}
	}
	if cover == "" && img != nil {
		h, seen := covers[img.SHA256]
		if !seen {
			if data := m.readBytes(ctx, drive, img.ID, maxCoverBytes); len(data) > 0 {
				if saved, serr := m.saveCover(ctx, data); serr == nil {
					h = saved
				}
			}
			covers[img.SHA256] = h
		}
		if h != "" {
			cover, coverSource = h, "folder"
		}
	}

	now := time.Now().UTC()
	synced := int64(0)
	if isSyncedLyrics(lyrics) {
		synced = 1
	}
	hasCover := int64(0)
	if cover != "" {
		hasCover = 1
	}

	if had {
		old, err := m.q.GetTrackByItem(ctx, f.ID)
		if err != nil {
			return false, err
		}
		if old.Manual != 0 {
			title, artist, info.Album, info.AlbumArtist = old.Title, old.Artist, old.Album, old.AlbumArtist
		}
		return false, m.q.UpdateTrackScan(ctx, db.UpdateTrackScanParams{
			Sha256: f.SHA256, Companion: sig, Title: title, Artist: artist, Album: info.Album, AlbumArtist: info.AlbumArtist,
			TrackNo: int64(info.TrackNo), DiscNo: int64(info.DiscNo), Year: int64(info.Year),
			DurationMs: int64(info.DurationMs), Bitrate: int64(info.Bitrate), Format: strings.TrimPrefix(ext, "."),
			HasCover: hasCover, CoverSource: coverSource, CoverKey: cover,
			LyricsSource: lyricsSource, LyricsSynced: synced, LyricsText: lyrics, UpdatedAt: now, ID: old.ID,
		})
	}
	_, err = m.q.InsertTrack(ctx, db.InsertTrackParams{
		DriveItemID: f.ID, Sha256: f.SHA256, Companion: sig, Title: title, Artist: artist, Album: info.Album, AlbumArtist: info.AlbumArtist,
		TrackNo: int64(info.TrackNo), DiscNo: int64(info.DiscNo), Year: int64(info.Year),
		DurationMs: int64(info.DurationMs), Bitrate: int64(info.Bitrate), Format: strings.TrimPrefix(ext, "."),
		HasCover: hasCover, CoverSource: coverSource, CoverKey: cover,
		LyricsSource: lyricsSource, LyricsSynced: synced, LyricsText: lyrics, CreatedAt: now, UpdatedAt: now,
	})
	return true, err
}

// readFromDrive copies the song to a temporary file, because the tag library
// reads from a path, and reads it.
func (m *Module) readFromDrive(ctx context.Context, drive contracts.DriveFiles, f contracts.DriveFile) (audioInfo, error) {
	rc, _, err := drive.Open(ctx, f.ID)
	if err != nil {
		return audioInfo{}, err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(m.tmpDir, "music-*"+strings.ToLower(filepath.Ext(f.Name)))
	if err != nil {
		return audioInfo{}, err
	}
	defer os.Remove(tmp.Name())
	defer contracts.TrackTemporaryFile(tmp.Name())()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return audioInfo{}, err
	}
	if err := tmp.Close(); err != nil {
		return audioInfo{}, err
	}
	return readAudio(tmp.Name())
}

func (m *Module) readBytes(ctx context.Context, drive contracts.DriveFiles, id int64, limit int64) []byte {
	rc, _, err := drive.Open(ctx, id)
	if err != nil {
		return nil
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit))
	if err != nil {
		return nil
	}
	return data
}

func (m *Module) readText(ctx context.Context, drive contracts.DriveFiles, id int64) string {
	return decodeLyricsFile(m.readBytes(ctx, drive, id, maxLRCSize))
}

package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.senan.xyz/taglib"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

const maxEmbeddedCover = 1200

// normalizeCover turns an image into a JPEG whose long edge is at most 1200
// pixels. This is what gets embedded in the file.
func normalizeCover(data []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxCoverPixels {
		return nil, errors.New("封面尺寸不对")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resizeCover(src, maxEmbeddedCover), &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// copyToTemp copies a drive file to a temporary file, because the tag library
// works on paths. cleanup removes it.
func (m *Module) copyToTemp(ctx context.Context, drive contracts.DriveFiles, f contracts.DriveFile) (path string, cleanup func(), err error) {
	rc, _, err := drive.Open(ctx, f.ID)
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(m.tmpDir, "music-wb-*"+strings.ToLower(filepath.Ext(f.Name)))
	if err != nil {
		return "", nil, err
	}
	untrack := contracts.TrackTemporaryFile(tmp.Name())
	cleanup = func() {
		untrack()
		os.Remove(tmp.Name())
	}
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		cleanup()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp.Name(), cleanup, nil
}

// writeBack embeds lyrics and a cover in the song file. It works on a copy,
// reads the copy back to check it, and only then replaces the file in the
// drive; any failure leaves the original untouched. Lyrics or cover that are
// empty are left alone. It returns the hash of the new content.
func (m *Module) writeBack(ctx context.Context, t db.MusicTrack, lyrics string, cover []byte) (string, error) {
	if lyrics == "" && len(cover) == 0 {
		return "", errors.New("nothing to write")
	}
	drive, err := m.drive()
	if err != nil {
		return "", err
	}
	file, err := drive.File(ctx, t.DriveItemID)
	if err != nil {
		return "", err
	}
	path, cleanup, err := m.copyToTemp(ctx, drive, file)
	if err != nil {
		return "", err
	}
	defer cleanup()

	before, err := taglib.ReadProperties(path)
	if err != nil {
		return "", fmt.Errorf("读不了这个文件: %w", err)
	}
	if before.Length <= 0 {
		// Not a playable audio file; do not touch it.
		return "", errors.New("这个文件读不出音频内容")
	}
	if lyrics != "" {
		if err := taglib.WriteTags(path, map[string][]string{taglib.Lyrics: {lyrics}}, 0); err != nil {
			return "", fmt.Errorf("写歌词: %w", err)
		}
	}
	var jpg []byte
	if len(cover) > 0 {
		if jpg, err = normalizeCover(cover); err != nil {
			return "", fmt.Errorf("处理封面: %w", err)
		}
		if err := taglib.WriteImage(path, jpg); err != nil {
			return "", fmt.Errorf("写封面: %w", err)
		}
	}

	// Check the copy: what was written reads back the same, and the audio
	// properties did not change.
	if lyrics != "" {
		tags, err := taglib.ReadTags(path)
		if err != nil || strings.TrimSpace(strings.Join(tags[taglib.Lyrics], "")) != strings.TrimSpace(lyrics) {
			return "", errors.New("写入后读不回歌词")
		}
	}
	if len(jpg) > 0 {
		img, err := taglib.ReadImage(path)
		if err != nil || !bytes.Equal(img, jpg) {
			return "", errors.New("写入后读不回封面")
		}
	}
	after, err := taglib.ReadProperties(path)
	if err != nil || after.Length != before.Length || after.SampleRate != before.SampleRate || after.Channels != before.Channels || after.BitRate != before.BitRate {
		return "", errors.New("写入后音频信息变了")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return "", err
	}
	if _, err := drive.ReplaceContent(ctx, file.ID, path, file.SHA256); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

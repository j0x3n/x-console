package music

import (
	"context"
	"errors"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// trackMetadata contains only fields matching is allowed to fill.
type trackMetadata struct {
	Artist, Album, AlbumArtist string
	Year                       int
}

func metadataOf(c candidate) trackMetadata {
	return trackMetadata{Artist: cleanTag(c.Artist), Album: cleanTag(c.Album), AlbumArtist: cleanTag(c.AlbumArtist), Year: max(0, c.Year)}
}

func (m trackMetadata) empty() bool {
	return m.Artist == "" && m.Album == "" && m.AlbumArtist == "" && m.Year <= 0
}

func (m *trackMetadata) merge(c candidate) {
	next := metadataOf(c)
	if m.Artist == "" {
		m.Artist = next.Artist
	}
	if m.Album == "" {
		m.Album = next.Album
	}
	if m.AlbumArtist == "" {
		m.AlbumArtist = next.AlbumArtist
	}
	if m.Year <= 0 {
		m.Year = next.Year
	}
}

func (m trackMetadata) missingFrom(t db.MusicTrack) trackMetadata {
	if t.Manual != 0 {
		return trackMetadata{}
	}
	if t.Artist != "" {
		m.Artist = ""
	}
	if t.Album != "" {
		m.Album = ""
	}
	if t.AlbumArtist != "" {
		m.AlbumArtist = ""
	}
	if t.Year != 0 {
		m.Year = 0
	}
	return m
}

func needsMetadata(t db.MusicTrack) bool {
	return t.Manual == 0 && (t.Artist == "" || t.Album == "" || t.AlbumArtist == "" || t.Year == 0)
}

// Persist the extra pending-candidate fields in the existing JSON store.
// This keeps the candidate table and older pending rows compatible without
// a schema migration. The data is removed along with the pending candidates.
type candidateMetadata struct {
	Source      string `json:"source"`
	SourceID    string `json:"sourceId"`
	AlbumArtist string `json:"albumArtist"`
	Year        int    `json:"year"`
}

func candidateMetadataKey(id int64) string {
	return "music.candidate_metadata." + strconv.FormatInt(id, 10)
}

func (m *Module) clearCandidates(ctx context.Context, id int64) error {
	if err := m.q.ClearCandidates(ctx, id); err != nil {
		return err
	}
	return m.d.Settings.Delete(ctx, candidateMetadataKey(id))
}

func (m *Module) candidateFromRow(ctx context.Context, row db.MusicCandidate) (candidate, error) {
	c := candidate{Source: row.Source, SourceID: row.SourceID, Title: row.Title, Artist: row.Artist, Album: row.Album, DurationMs: int(row.DurationMs),
		Lyrics: row.HasLyrics != 0, Cover: row.HasCover != 0, LyricsText: row.LyricsText, coverRef: row.CoverRef}
	var extra []candidateMetadata
	if err := m.d.Settings.Get(ctx, candidateMetadataKey(row.TrackID), &extra); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return c, err
	}
	for _, x := range extra {
		if x.Source == c.Source && x.SourceID == c.SourceID {
			c.AlbumArtist, c.Year = x.AlbumArtist, x.Year
			break
		}
	}
	return c, nil
}

package music

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

// Playlist shares (B149). A link plays only the songs of one playlist, needs no
// login, and can have a password and an end date. It follows the notes share
// (B72): the password is stored as bcrypt of its SHA-256, a wrong password
// five times in a row locks that address for 15 minutes, and every public
// answer is "not found" for links that do not exist or have ended.

var (
	errShareGone         = httpx.NewError(http.StatusNotFound, "share_not_found", "分享链接不存在或已失效")
	errShareCodeRequired = httpx.NewError(http.StatusUnauthorized, "share_code_required", "请输入密码")
)

const (
	shareAccessTTL   = time.Hour
	shareRatePerMin  = 120
	shareFailLimit   = 5
	shareLockTime    = 15 * time.Minute
	sharePasswordMax = 64
)

// PublicPaths implements module.PublicPather. Every public route checks the link itself.
func (m *Module) PublicPaths() []string { return []string{"/public/music/"} }

type shareLimits struct {
	mu    sync.Mutex
	hits  map[string]shareHit
	fails map[string]shareFail
}

type shareHit struct {
	count int
	until time.Time
}

type shareFail struct {
	count  int
	locked time.Time
	last   time.Time
}

func (m *Module) clientIP(r *http.Request) string {
	return httpx.ClientIP(r, m.d.Config.TrustedProxies)
}

func publicHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// rateLimited counts a request from this address and refuses when it is too many.
func (m *Module) rateLimited(w http.ResponseWriter, r *http.Request) bool {
	now := time.Now()
	ip := m.clientIP(r)
	l := &m.shares
	l.mu.Lock()
	if l.hits == nil {
		l.hits, l.fails = map[string]shareHit{}, map[string]shareFail{}
	}
	h := l.hits[ip]
	if !h.until.After(now) {
		h = shareHit{until: now.Add(time.Minute)}
	}
	h.count++
	l.hits[ip] = h
	for k, v := range l.hits { // keep the maps small
		if !v.until.After(now) {
			delete(l.hits, k)
		}
	}
	l.mu.Unlock()
	if h.count <= shareRatePerMin {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(h.until).Seconds())+1)))
	httpx.Fail(w, r, httpx.ErrTooManyRequests)
	return true
}

func (m *Module) resolveShare(r *http.Request, token string) (db.MusicPlaylistShare, error) {
	share, err := m.q.GetShareByToken(r.Context(), token)
	if errors.Is(err, sql.ErrNoRows) {
		return share, errShareGone
	}
	if err != nil {
		return share, err
	}
	if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now().UTC()) {
		return share, errShareGone
	}
	return share, nil
}

func (m *Module) accessMAC(id, expires int64, token string) []byte {
	key := hmac.New(sha256.New, m.d.Config.MasterKey)
	key.Write([]byte("x-console/music-share-access/v1"))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(strconv.FormatInt(id, 10) + "." + strconv.FormatInt(expires, 10) + "." + token))
	return mac.Sum(nil)
}

func (m *Module) newAccess(share db.MusicPlaylistShare) (string, time.Time) {
	expiry := time.Now().UTC().Add(shareAccessTTL)
	signed := strconv.FormatInt(share.ID, 10) + "." + strconv.FormatInt(expiry.Unix(), 10)
	return signed + "." + base64.RawURLEncoding.EncodeToString(m.accessMAC(share.ID, expiry.Unix(), share.Token)), expiry
}

func (m *Module) validAccess(share db.MusicPlaylistShare, access *string) bool {
	if access == nil {
		return false
	}
	parts := strings.Split(*access, ".")
	if len(parts) != 3 {
		return false
	}
	id, err1 := strconv.ParseInt(parts[0], 10, 64)
	exp, err2 := strconv.ParseInt(parts[1], 10, 64)
	mac, err3 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || id != share.ID || time.Now().Unix() >= exp {
		return false
	}
	return subtle.ConstantTimeCompare(mac, m.accessMAC(id, exp, share.Token)) == 1
}

// openShare is the check every public route starts with: rate limit, the link
// exists and has not ended, and the password was given when there is one.
func (m *Module) openShare(w http.ResponseWriter, r *http.Request, token string, access *string) (db.MusicPlaylistShare, bool) {
	publicHeaders(w)
	if m.rateLimited(w, r) {
		return db.MusicPlaylistShare{}, false
	}
	share, err := m.resolveShare(r, token)
	if fail(w, r, err) {
		return share, false
	}
	if share.PasswordHash != nil && !m.validAccess(share, access) {
		httpx.Fail(w, r, errShareCodeRequired)
		return share, false
	}
	return share, true
}

// openShareTrack also checks the song is in the shared playlist.
func (m *Module) openShareTrack(w http.ResponseWriter, r *http.Request, token string, access *string, trackID int64) bool {
	share, ok := m.openShare(w, r, token, access)
	if !ok {
		return false
	}
	n, err := m.q.PlaylistHasTrack(r.Context(), db.PlaylistHasTrackParams{PlaylistID: share.PlaylistID, TrackID: trackID})
	if fail(w, r, err) {
		return false
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return false
	}
	return true
}

func toShare(s db.MusicPlaylistShare) api.MusicShare {
	return api.MusicShare{Id: s.ID, PlaylistId: s.PlaylistID, Path: "/m/" + s.Token, HasPassword: s.PasswordHash != nil,
		ExpiresAt: s.ExpiresAt, ViewCount: int(s.ViewCount), CreatedAt: s.CreatedAt}
}

func passwordDigest(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

func (m *Module) ListMusicShares(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	ctx := r.Context()
	if _, err := m.playlistSummary(ctx, m.q, id); fail(w, r, err) {
		return
	}
	rows, err := m.q.ListShares(ctx, id)
	if fail(w, r, err) {
		return
	}
	out := make([]api.MusicShare, 0, len(rows))
	for _, s := range rows {
		out = append(out, toShare(s))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateMusicShare(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	ctx := r.Context()
	var body api.MusicShareInput
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if _, err := m.playlistSummary(ctx, m.q, id); fail(w, r, err) {
		return
	}
	var hash *string
	if body.Password != nil {
		if *body.Password == "" || len(*body.Password) > sharePasswordMax {
			httpx.Fail(w, r, httpx.Invalid("密码要 1 到 64 个字符"))
			return
		}
		b, err := bcrypt.GenerateFromPassword(passwordDigest(*body.Password), bcrypt.DefaultCost)
		if fail(w, r, err) {
			return
		}
		h := string(b)
		hash = &h
	}
	var expires *time.Time
	if body.ExpiresInDays != nil {
		if *body.ExpiresInDays < 1 || *body.ExpiresInDays > 365 {
			httpx.Fail(w, r, httpx.Invalid("有效期要在 1 到 365 天之间"))
			return
		}
		e := time.Now().UTC().Add(time.Duration(*body.ExpiresInDays) * 24 * time.Hour)
		expires = &e
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); fail(w, r, err) {
		return
	}
	share, err := m.q.InsertShare(ctx, db.InsertShareParams{PlaylistID: id, Token: base64.RawURLEncoding.EncodeToString(raw),
		PasswordHash: hash, ExpiresAt: expires, CreatedAt: time.Now().UTC()})
	if fail(w, r, err) {
		return
	}
	m.d.Audit.Record(ctx, "music.share_create", "playlist:"+strconv.FormatInt(id, 10), map[string]any{"share": share.ID, "password": hash != nil}, nil)
	httpx.JSON(w, http.StatusCreated, toShare(share))
}

func (m *Module) DeleteMusicShare(w http.ResponseWriter, r *http.Request, shareID int64) {
	n, err := m.q.DeleteShare(r.Context(), shareID)
	if fail(w, r, err) {
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m.d.Audit.Record(r.Context(), "music.share_delete", "share:"+strconv.FormatInt(shareID, 10), nil, nil)
	httpx.NoContent(w)
}

func (m *Module) GetPublicMusicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken, p api.GetPublicMusicShareParams) {
	share, ok := m.openShare(w, r, token, p.Access)
	if !ok {
		return
	}
	ctx := r.Context()
	pl, err := m.q.GetPlaylist(ctx, share.PlaylistID)
	if fail(w, r, mapNoRows(err)) {
		return
	}
	rows, err := m.q.ListPlaylistTracks(ctx, share.PlaylistID)
	if fail(w, r, err) {
		return
	}
	_ = m.q.BumpShareViews(ctx, share.ID)
	out := api.MusicPublicPlaylist{Name: pl.Name, Tracks: make([]api.MusicPublicTrack, 0, len(rows))}
	for _, t := range rows {
		out.Tracks = append(out.Tracks, api.MusicPublicTrack{Id: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
			DurationMs: int(t.DurationMs), HasCover: t.HasCover != 0, HasLyrics: t.LyricsText != ""})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UnlockPublicMusicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken) {
	publicHeaders(w)
	if m.rateLimited(w, r) {
		return
	}
	var body api.UnlockPublicMusicShareJSONBody
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	share, err := m.resolveShare(r, token)
	if fail(w, r, err) {
		return
	}
	if share.PasswordHash == nil {
		access, exp := m.newAccess(share)
		httpx.JSON(w, http.StatusOK, map[string]any{"access": access, "expiresAt": exp})
		return
	}
	key := m.clientIP(r) + "|" + share.Token
	now := time.Now()
	l := &m.shares
	l.mu.Lock()
	f := l.fails[key]
	if f.locked.After(now) {
		l.mu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(f.locked).Seconds())+1))
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return
	}
	l.mu.Unlock()
	if bcrypt.CompareHashAndPassword([]byte(*share.PasswordHash), passwordDigest(body.Code)) != nil {
		l.mu.Lock()
		f = l.fails[key]
		if f.last.Before(now.Add(-shareLockTime)) {
			f = shareFail{}
		}
		f.count++
		f.last = now
		if f.count >= shareFailLimit {
			f.locked, f.count = now.Add(shareLockTime), 0
		}
		l.fails[key] = f
		l.mu.Unlock()
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "share_code_invalid", "密码不对"))
		return
	}
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
	access, exp := m.newAccess(share)
	httpx.JSON(w, http.StatusOK, map[string]any{"access": access, "expiresAt": exp})
}

func (m *Module) StreamPublicMusicTrack(w http.ResponseWriter, r *http.Request, token api.ShareToken, trackID api.TrackId, p api.StreamPublicMusicTrackParams) {
	if m.openShareTrack(w, r, token, p.Access, trackID) {
		m.StreamMusicTrack(w, r, trackID)
	}
}

func (m *Module) GetPublicMusicTrackCover(w http.ResponseWriter, r *http.Request, token api.ShareToken, trackID api.TrackId, p api.GetPublicMusicTrackCoverParams) {
	if !m.openShareTrack(w, r, token, p.Access, trackID) {
		return
	}
	var size *api.GetMusicTrackCoverParamsSize
	if p.Size != nil {
		s := api.GetMusicTrackCoverParamsSize(*p.Size)
		size = &s
	}
	m.GetMusicTrackCover(w, r, trackID, api.GetMusicTrackCoverParams{Size: size})
}

func (m *Module) GetPublicMusicTrackLyrics(w http.ResponseWriter, r *http.Request, token api.ShareToken, trackID api.TrackId, p api.GetPublicMusicTrackLyricsParams) {
	if m.openShareTrack(w, r, token, p.Access, trackID) {
		m.GetMusicTrackLyrics(w, r, trackID)
	}
}

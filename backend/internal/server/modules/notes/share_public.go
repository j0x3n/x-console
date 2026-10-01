package notes

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
)

type noteShareRate struct {
	Count int
	Until time.Time
}
type noteShareFailure struct {
	Count       int
	LockedUntil time.Time
	Updated     time.Time
}

func (m *Module) PublicPaths() []string { return []string{"/public/notes/"} }

func noteClientIP(r *http.Request) string {
	if value := r.Header.Get("X-Forwarded-For"); value != "" {
		return strings.TrimSpace(strings.Split(value, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func notePublicHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (m *Module) shareClock() time.Time {
	if m.shareNow != nil {
		return m.shareNow()
	}
	return m.now()
}

func (m *Module) pruneNoteShareRates(now time.Time) {
	for key, rate := range m.shareRates {
		if !rate.Until.After(now) {
			delete(m.shareRates, key)
		}
	}
	for key, failure := range m.shareFailures {
		if !failure.LockedUntil.After(now) && failure.Updated.Before(now.Add(-15*time.Minute)) {
			delete(m.shareFailures, key)
		}
	}
	for key, at := range m.shareVisits {
		if !at.Add(30 * time.Minute).After(now) {
			delete(m.shareVisits, key)
		}
	}
}

func (m *Module) noteShareRateLimit(w http.ResponseWriter, r *http.Request, unlock bool) bool {
	now := m.shareClock()
	ip := noteClientIP(r)
	m.shareMu.Lock()
	defer m.shareMu.Unlock()
	m.pruneNoteShareRates(now)
	if unlock {
		failure := m.shareFailures[ip]
		if failure.LockedUntil.After(now) {
			return noteRateFailure(w, r, failure.LockedUntil, now)
		}
	}
	key, limit := "read:"+ip, 60
	if unlock {
		key, limit = "unlock:"+ip, 5
	}
	rate := m.shareRates[key]
	if !rate.Until.After(now) {
		rate = noteShareRate{Until: now.Add(time.Minute)}
	}
	rate.Count++
	m.shareRates[key] = rate
	if rate.Count > limit {
		return noteRateFailure(w, r, rate.Until, now)
	}
	return false
}

func noteRateFailure(w http.ResponseWriter, r *http.Request, until, now time.Time) bool {
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(until.Sub(now).Seconds())+1)))
	httpx.Fail(w, r, httpx.ErrTooManyRequests)
	return true
}

func (m *Module) noteShareMAC(s noteShareRow, expires int64) []byte {
	mac := hmac.New(sha256.New, m.d.Config.MasterKey)
	fmt.Fprintf(mac, "x-console/note-access/v1\x00%s\x00%d\x00%s", s.Token, expires, deref(s.PasswordHash))
	return mac.Sum(nil)
}

func (m *Module) validNoteAccess(s noteShareRow, access *string) bool {
	if access == nil {
		return false
	}
	parts := strings.Split(*access, ".")
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires <= m.shareClock().Unix() {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	return err == nil && hmac.Equal(signature, m.noteShareMAC(s, expires))
}

func (m *Module) publicNote(w http.ResponseWriter, r *http.Request, token string, access *string) (noteShareRow, db.Note, bool) {
	notePublicHeaders(w)
	if m.noteShareRateLimit(w, r, false) {
		return noteShareRow{}, db.Note{}, false
	}
	s, n, err := m.resolvePublicNote(r.Context(), token)
	if err != nil {
		httpx.Fail(w, r, err)
		return s, n, false
	}
	if s.PasswordHash != nil && !m.validNoteAccess(s, access) {
		httpx.Fail(w, r, httpx.NewError(401, "note_password_required", "请输入分享密码"))
		return s, n, false
	}
	return s, n, true
}

func (m *Module) UnlockPublicNote(w http.ResponseWriter, r *http.Request, token api.NoteShareToken) {
	notePublicHeaders(w)
	if m.noteShareRateLimit(w, r, true) {
		return
	}
	s, _, err := m.resolvePublicNote(r.Context(), token)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.UnlockPublicNoteJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if s.PasswordHash != nil {
		digest := sha256.Sum256([]byte(in.Password))
		if bcrypt.CompareHashAndPassword([]byte(*s.PasswordHash), digest[:]) != nil {
			now := m.shareClock()
			m.shareMu.Lock()
			failure := m.shareFailures[noteClientIP(r)]
			failure.Count++
			failure.Updated = now
			if failure.Count >= 10 {
				failure.LockedUntil = now.Add(15 * time.Minute)
			}
			m.shareFailures[noteClientIP(r)] = failure
			m.shareMu.Unlock()
			if failure.Count >= 10 {
				noteRateFailure(w, r, failure.LockedUntil, now)
			} else {
				httpx.Fail(w, r, httpx.NewError(403, "note_password_wrong", "分享密码不正确"))
			}
			return
		}
	}
	m.shareMu.Lock()
	delete(m.shareFailures, noteClientIP(r))
	m.shareMu.Unlock()
	expires := m.shareClock().Add(12 * time.Hour).Truncate(time.Second)
	access := strconv.FormatInt(expires.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(m.noteShareMAC(s, expires.Unix()))
	httpx.JSON(w, 200, map[string]any{"access": access, "expiresAt": expires})
}

func (m *Module) GetPublicNote(w http.ResponseWriter, r *http.Request, token api.NoteShareToken, params api.GetPublicNoteParams) {
	s, n, ok := m.publicNote(w, r, token, params.T)
	if !ok {
		return
	}
	body := rewriteNoteAttachments(n.Body, token, deref(params.T))
	identity := noteClientIP(r) + "\x00" + r.UserAgent()
	if params.T != nil && m.validNoteAccess(s, params.T) {
		identity = *params.T
	}
	key := sha256.Sum256([]byte(token + "\x00" + identity))
	now := m.shareClock()
	m.shareMu.Lock()
	if at, exists := m.shareVisits[key]; !exists || !at.Add(30*time.Minute).After(now) {
		_, err := m.d.DB.ExecContext(r.Context(), "UPDATE note_shares SET visits=visits+1,last_visit_at=? WHERE note_id=? AND token=?", now, s.NoteID, s.Token)
		if err != nil {
			m.shareMu.Unlock()
			httpx.Fail(w, r, err)
			return
		}
		m.shareVisits[key] = now
	}
	m.shareMu.Unlock()
	httpx.JSON(w, 200, api.PublicNote{Title: n.Title, Body: body, Color: new(api.NoteColor(n.Color)), UpdatedAt: n.UpdatedAt})
}

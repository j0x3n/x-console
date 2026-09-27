// Package core serves the M0 API: auth, audit, agents and notifications.
package core

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Version is set at build time with -ldflags "-X .../core.Version=...".
var Version = "dev"

// Handlers implements api.ServerInterface.
type Handlers struct {
	Auth   *auth.Service
	Agents *agenthub.Hub
	Notify *notify.Service
	Q      *db.Queries
}

var _ api.ServerInterface = (*Handlers)(nil)

func (h *Handlers) GetHealth(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

func (h *Handlers) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	required, err := h.Auth.SetupRequired(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.AuthStatus{SetupRequired: required}
	if s := auth.FromContext(r.Context()); s != nil {
		enabled, err := h.Auth.TOTPEnabled(r.Context())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out.Authenticated = true
		out.Username = &s.Username
		out.TotpEnabled = &enabled
		if s.Elevated() {
			out.ElevatedUntil = s.ElevatedUntil
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handlers) SetupAccount(w http.ResponseWriter, r *http.Request) {
	var body api.Credentials
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	secret, url, err := h.Auth.Setup(r.Context(), body.Username, body.Password)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.TotpEnrollment{Secret: secret, OtpauthUrl: url})
}

func (h *Handlers) ConfirmSetup(w http.ResponseWriter, r *http.Request) {
	var body api.TotpCode
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.ConfirmSetup(r.Context(), w, r, body.Code); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var body api.LoginRequest
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.Login(r.Context(), w, r, body.Username, body.Password, body.Code); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Auth.Logout(r.Context(), w); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) Elevate(w http.ResponseWriter, r *http.Request) {
	var body api.ElevateRequest
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	until, err := h.Auth.Elevate(r.Context(), body.Code, body.Password)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]time.Time{"elevatedUntil": until})
}

func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request, params api.ListAuditParams) {
	before, err := httpx.DecodeIDCursor(params.Cursor)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := httpx.Limit(params.Limit)
	rows, err := h.Q.ListAudit(r.Context(), db.ListAuditParams{ID: before, Limit: limit})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.AuditEntry, 0, len(rows))
	for _, row := range rows {
		var detail map[string]any
		_ = json.Unmarshal([]byte(row.Detail), &detail)
		items = append(items, api.AuditEntry{Id: row.ID, At: row.At, Actor: row.Actor, Action: row.Action,
			Target: row.Target, Result: row.Result, Detail: &detail})
	}
	out := map[string]any{"items": items}
	if int64(len(rows)) == limit {
		out["nextCursor"] = httpx.EncodeIDCursor(rows[len(rows)-1].ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.Agents.List(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.Agent, 0, len(agents))
	for _, a := range agents {
		out = append(out, AgentToAPI(a))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// AgentToAPI converts an agent for JSON responses. Modules reuse it.
func AgentToAPI(a agenthub.Agent) api.Agent {
	return api.Agent{Id: a.ID, Name: a.Name, Kind: api.AgentKind(a.Kind), Os: a.Os, Arch: a.Arch,
		Hostname: a.Hostname, Version: a.Version, Online: a.Online, Capabilities: a.Capabilities,
		CreatedAt: a.CreatedAt, LastSeenAt: a.LastSeenAt}
}

func (h *Handlers) CreatePairingCode(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.CreatePairingCodeJSONBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	code, expires, err := h.Agents.CreatePairingCode(r.Context(), body.Name, string(body.Kind))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"code": code, "expiresAt": expires})
}

func (h *Handlers) RevokeAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Agents.Revoke(r.Context(), agentID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) PairAgent(w http.ResponseWriter, r *http.Request) {
	var body api.PairRequest
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var caps []string
	if body.Capabilities != nil {
		caps = *body.Capabilities
	}
	id, token, err := h.Agents.Pair(r.Context(), body.Code, protocol.Hello{
		ProtocolVersion: protocol.Version, AgentVersion: body.Version, OS: body.Os, Arch: body.Arch,
		Hostname: body.Hostname, Capabilities: caps,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"agentId": id, "token": token})
}

func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request, params api.ListNotificationsParams) {
	before, err := httpx.DecodeIDCursor(params.Cursor)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := httpx.Limit(params.Limit)
	var unread int64
	if params.Unread != nil && *params.Unread {
		unread = 1
	}
	rows, err := h.Q.ListNotifications(r.Context(), db.ListNotificationsParams{Before: before, UnreadOnly: unread, Lim: limit})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	count, err := h.Q.CountUnreadNotifications(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]notify.API, 0, len(rows))
	for _, row := range rows {
		items = append(items, notify.ToAPI(row))
	}
	out := map[string]any{"items": items, "unreadCount": count}
	if int64(len(rows)) == limit {
		out["nextCursor"] = httpx.EncodeIDCursor(rows[len(rows)-1].ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handlers) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	if err := h.Q.MarkAllNotificationsRead(r.Context(), &now); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) MarkNotificationRead(w http.ResponseWriter, r *http.Request, id int64) {
	now := time.Now().UTC()
	if _, err := h.Q.MarkNotificationRead(r.Context(), db.MarkNotificationReadParams{ReadAt: &now, ID: id}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) DeleteNotification(w http.ResponseWriter, r *http.Request, id int64) {
	n, err := h.Q.DeleteNotification(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if n == 0 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	httpx.NoContent(w)
}

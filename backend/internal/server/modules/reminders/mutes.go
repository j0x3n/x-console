package reminders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// Mute rules (B113): a kind of notification, optionally limited to a scope
// such as one mailbox, is not sent to a target. A target is a channel name, or
// "webpush:<id>" for one Web Push device. The in-app bell is never muted.

const (
	scopeRemovedTopic = "notify.scope_removed"
	devicePrefix      = "webpush:"
)

var (
	kindPatternRE = regexp.MustCompile(`^[a-z0-9_.*-]{1,64}$`)
	scopeRE       = regexp.MustCompile(`^[a-z0-9_]{1,32}:[A-Za-z0-9_-]{1,64}$`)
)

// muted reports whether a rule keeps n from going to target.
func muted(rules []db.NotificationMute, n notify.Stored, target string) bool {
	for _, r := range rules {
		if r.Target == target && matchKind(r.KindPattern, n.Kind) && (r.Scope == "" || r.Scope == n.Scope) {
			return true
		}
	}
	return false
}

func (m *Module) loadMutes(ctx context.Context) []db.NotificationMute {
	rules, err := m.q.ListMutes(ctx)
	if err != nil {
		// Failing to read the rules must not stop notifications.
		m.d.Log.Warn("load notification mutes", "err", err)
		return nil
	}
	return rules
}

func muteToAPI(r db.NotificationMute) api.NotifyMute {
	return api.NotifyMute{Id: r.ID, KindPattern: r.KindPattern, Scope: r.Scope, Target: r.Target, CreatedAt: r.CreatedAt}
}

func muteItems(rows []db.NotificationMute) map[string]any {
	items := make([]api.NotifyMute, 0, len(rows))
	for _, r := range rows {
		items = append(items, muteToAPI(r))
	}
	return map[string]any{"items": items}
}

// validateMute checks one rule's parts. Channel names must exist, a device
// must be subscribed.
func (m *Module) validateMute(ctx context.Context, kind, scope, target string) error {
	if !kindPatternRE.MatchString(kind) {
		return httpx.Invalid("通知类型只能写小写字母、数字、点、下划线、减号和 *")
	}
	if _, err := path.Match(kind, "x"); err != nil {
		return httpx.Invalid("通知类型的写法不对")
	}
	if scope != "" && !scopeRE.MatchString(scope) {
		return httpx.Invalid("范围的写法不对，比如 mail:3")
	}
	if id, ok := strings.CutPrefix(target, devicePrefix); ok {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return httpx.Invalid("找不到这台设备")
		}
		if _, err := m.q.GetPushSubscriptionByID(ctx, n); errors.Is(err, sql.ErrNoRows) {
			return httpx.Invalid("找不到这台设备")
		} else if err != nil {
			return err
		}
		return nil
	}
	for _, name := range channelOrder {
		if target == name {
			return nil
		}
	}
	return httpx.Invalid("不认识这个发送目标")
}

// ListNotifyMutes implements api.ServerInterface.
func (m *Module) ListNotifyMutes(w http.ResponseWriter, r *http.Request, params api.ListNotifyMutesParams) {
	var rows []db.NotificationMute
	var err error
	if params.Scope != nil {
		rows, err = m.q.ListMutesInScope(r.Context(), *params.Scope)
	} else {
		rows, err = m.q.ListMutes(r.Context())
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, muteItems(rows))
}

// CreateNotifyMute implements api.ServerInterface.
func (m *Module) CreateNotifyMute(w http.ResponseWriter, r *http.Request) {
	var body api.CreateNotifyMuteJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	scope := ""
	if body.Scope != nil {
		scope = strings.TrimSpace(*body.Scope)
	}
	kind, target := strings.TrimSpace(body.KindPattern), strings.TrimSpace(body.Target)
	err := m.validateMute(r.Context(), kind, scope, target)
	var row db.NotificationMute
	if err == nil {
		row, err = m.q.InsertMute(r.Context(), db.InsertMuteParams{KindPattern: kind, Scope: scope, Target: target, CreatedAt: time.Now().UTC()})
		if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
			err = httpx.NewError(http.StatusConflict, "conflict", "这条规则已经有了")
		}
	}
	m.d.Audit.Record(r.Context(), "notify_mute.create", target, map[string]any{"kind": kind, "scope": scope}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.mutes_updated", map[string]any{"scope": scope})
	httpx.JSON(w, http.StatusCreated, muteToAPI(row))
}

// DeleteNotifyMute implements api.ServerInterface.
func (m *Module) DeleteNotifyMute(w http.ResponseWriter, r *http.Request, muteId int64) {
	n, err := m.q.DeleteMute(r.Context(), muteId)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "notify_mute.delete", strconv.FormatInt(muteId, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.mutes_updated", map[string]any{})
	httpx.NoContent(w)
}

// ReplaceScopeMutes implements api.ServerInterface.
func (m *Module) ReplaceScopeMutes(w http.ResponseWriter, r *http.Request) {
	var body api.ReplaceScopeMutesJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.replaceScope(r.Context(), strings.TrimSpace(body.KindPattern), strings.TrimSpace(body.Scope), body.Targets)
	m.d.Audit.Record(r.Context(), "notify_mute.replace", body.Scope, map[string]any{"kind": body.KindPattern, "targets": body.Targets}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.mutes_updated", map[string]any{"scope": body.Scope})
	httpx.JSON(w, http.StatusOK, muteItems(rows))
}

func (m *Module) replaceScope(ctx context.Context, kind, scope string, targets []string) ([]db.NotificationMute, error) {
	if scope == "" {
		return nil, httpx.Invalid("要写范围，比如 mail:3")
	}
	seen := map[string]bool{}
	var clean []string
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if seen[t] {
			continue
		}
		seen[t] = true
		if err := m.validateMute(ctx, kind, scope, t); err != nil {
			return nil, err
		}
		clean = append(clean, t)
	}
	if len(clean) == 0 {
		if err := m.validateMute(ctx, kind, scope, "webpush"); err != nil {
			return nil, err
		}
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteMutesOfScopeAndKind(ctx, db.DeleteMutesOfScopeAndKindParams{KindPattern: kind, Scope: scope}); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, t := range clean {
		if _, err := q.InsertMute(ctx, db.InsertMuteParams{KindPattern: kind, Scope: scope, Target: t, CreatedAt: now}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m.q.ListMutesInScope(ctx, scope)
}

// pruneMutes drops rules that point at a Web Push device that is gone.
func (m *Module) pruneMutes(ctx context.Context) {
	if err := m.q.PruneDeviceMutes(context.WithoutCancel(ctx)); err != nil {
		m.d.Log.Warn("prune notification mutes", "err", err)
	}
}

// watchScopeRemoved deletes the rules of a scope when the module that owns it
// says the scope is gone (a deleted mailbox). Modules do not touch each
// other's tables, so they say it with an event.
func (m *Module) watchScopeRemoved(ctx context.Context) {
	events, cancel := m.d.Bus.Subscribe(scopeRemovedTopic, 64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				var p struct {
					Scope string `json:"scope"`
				}
				raw, _ := json.Marshal(ev.Data)
				if err := json.Unmarshal(raw, &p); err != nil || p.Scope == "" {
					continue
				}
				if err := m.q.DeleteMutesOfScope(ctx, p.Scope); err != nil && ctx.Err() == nil {
					m.d.Log.Warn("delete notification mutes of scope", "scope", p.Scope, "err", err)
					continue
				}
				m.d.Bus.Publish("notify.mutes_updated", map[string]any{"scope": p.Scope})
			}
		}
	}()
}

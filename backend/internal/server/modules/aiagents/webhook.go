package aiagents

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// B47 Git webhook: POST /hooks/git/{connectionId}. GitHub signs with
// X-Hub-Signature-256 ("sha256=" + hex HMAC), Forgejo and Gitea with
// X-Forgejo-Signature or X-Gitea-Signature (hex HMAC), all over the raw
// body with the connection's secret. When a pull request of a coding task
// is merged, its card moves to done.

const maxHookBody = 5 << 20

var errBadSignature = httpx.NewError(http.StatusUnauthorized, "bad_signature", "签名不对")

// verifySignature checks the request's signature header against body.
func verifySignature(h http.Header, body []byte, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, name := range []string{"X-Hub-Signature-256", "X-Forgejo-Signature", "X-Gitea-Signature"} {
		v := strings.TrimSpace(h.Get(name))
		if v == "" {
			continue
		}
		v = strings.TrimPrefix(v, "sha256=")
		got, err := hex.DecodeString(v)
		if err == nil && hmac.Equal(got, want) {
			return true
		}
	}
	return false
}

func hookEvent(h http.Header) string {
	for _, name := range []string{"X-GitHub-Event", "X-Forgejo-Event", "X-Gitea-Event"} {
		if v := h.Get(name); v != "" {
			return v
		}
	}
	return ""
}

func (m *Module) hook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "connectionId"), 10, 64)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxHookBody+1))
	if err != nil || len(body) > maxHookBody {
		httpx.Fail(w, r, httpx.Invalid("请求体太大"))
		return
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		// The same answer as a bad signature: do not reveal which ids exist.
		httpx.Fail(w, r, errBadSignature)
		return
	}
	secret, err := m.d.Secrets.Open(c.WebhookSecretEnc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !verifySignature(r.Header, body, secret) {
		httpx.Fail(w, r, errBadSignature)
		return
	}
	if receiver, ok := module.Lookup[contracts.GitWebhookReceiver](m.d.Registry, contracts.GitWebhookKey); ok {
		delivery := ""
		for _, name := range []string{"X-GitHub-Delivery", "X-Forgejo-Delivery", "X-Gitea-Delivery"} {
			if v := r.Header.Get(name); v != "" {
				delivery = v
				break
			}
		}
		if err := receiver.ReceiveGitWebhook(ctx, contracts.GitWebhook{ConnectionID: id, Event: hookEvent(r.Header), DeliveryID: delivery, Body: body}); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if hookEvent(r.Header) != "pull_request" {
		httpx.NoContent(w) // ping and everything else
		return
	}
	var ev struct {
		Action      string `json:"action"`
		PullRequest struct {
			HTMLURL string `json:"html_url"`
			Merged  bool   `json:"merged"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		httpx.Fail(w, r, httpx.Invalid("请求体不是 JSON"))
		return
	}
	if ev.Action != "closed" || !ev.PullRequest.Merged || ev.PullRequest.HTMLURL == "" {
		httpx.NoContent(w)
		return
	}
	if err := m.merged(r, ev.PullRequest.HTMLURL); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// merged moves the cards of the tasks with this pull request to done.
func (m *Module) merged(r *http.Request, url string) error {
	ctx := r.Context()
	tasks, err := m.q.TasksByPR(ctx, url)
	if err != nil {
		return err
	}
	issues, _ := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	work, _ := m.issueWork()
	var errs []error
	for _, t := range tasks {
		m.d.Audit.Record(ctx, "git.pr_merged", strconv.FormatInt(t.ID, 10), map[string]any{"url": url, "issueKey": t.IssueKey}, nil)
		m.d.Bus.Publish("coding_task.merged", map[string]any{"taskId": t.ID, "prUrl": url, "issueKey": t.IssueKey})
		if t.IssueKey == "" || issues == nil {
			continue
		}
		if err := issues.SetStatus(ctx, t.IssueKey, "done"); err != nil {
			slog.Warn("aiagents: card to done", "issue", t.IssueKey, "err", err)
			errs = append(errs, err)
			continue
		}
		if work != nil {
			who := ""
			if t.AiAgentID != nil {
				who = author(*t.AiAgentID)
			}
			if err := work.Comment(ctx, t.IssueKey, who, "PR 合并了："+url); err != nil {
				slog.Warn("aiagents: comment merged", "issue", t.IssueKey, "err", err)
			}
		}
	}
	return errors.Join(errs...)
}

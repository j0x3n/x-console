package mail

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

// Every endpoint is in the contract but not built yet. See docs/specs/B53.md.

func (m *Module) ListMailAccounts(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CreateMailAccount(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DeleteMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UpdateMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) SyncMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListMailMessages(w http.ResponseWriter, r *http.Request, params api.ListMailMessagesParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetMailMessage(w http.ResponseWriter, r *http.Request, messageID api.MessageId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UpdateMailMessage(w http.ResponseWriter, r *http.Request, messageID api.MessageId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DownloadMailAttachment(w http.ResponseWriter, r *http.Request, messageID api.MessageId, index int) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetMailSummary(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

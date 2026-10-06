package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

var mailHostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

func (m *Module) accountConfig(a db.MailAccount) (imapConfig, error) {
	pw, err := m.d.Secrets.Open(a.PasswordEnc)
	if err != nil {
		return imapConfig{}, errors.New("密码解不开，请重新填一次")
	}
	return imapConfig{provider: api.MailProvider(a.Provider), host: a.ImapHost, port: int(a.ImapPort), username: a.Username, password: pw}, nil
}

func (m *Module) accountView(a db.MailAccount, unread int) api.MailAccount {
	s := m.state(a.ID)
	idle := s.Idle
	return api.MailAccount{
		Id: a.ID, Name: a.Name, Email: a.Email, Provider: api.MailProvider(a.Provider), ImapHost: a.ImapHost,
		ImapPort: int(a.ImapPort), Username: a.Username, Notify: a.Notify == 1, Status: s.Status, LastError: s.LastError,
		Idle: &idle, LastSyncAt: a.LastSyncAt, Unread: unread, CreatedAt: a.CreatedAt,
	}
}

func (m *Module) unreadCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := m.q.UnreadCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, r := range rows {
		out[r.AccountID] = int(r.Unread)
	}
	return out, nil
}

func (m *Module) getAccount(ctx context.Context, id int64) (db.MailAccount, error) {
	a, err := m.q.GetAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

// cleanServer validates host and port; the presets use their fixed server.
func cleanServer(p api.MailProvider, host *string, port *int) (string, int, error) {
	if fixed, ok := presetHosts[p]; ok {
		return fixed, 993, nil
	}
	h := strings.TrimSpace(deref(host))
	h = strings.TrimPrefix(strings.TrimPrefix(h, "imaps://"), "imap://")
	if h == "" {
		return "", 0, httpx.Invalid("请填 IMAP 服务器")
	}
	if !mailHostPattern.MatchString(h) {
		return "", 0, httpx.Invalid("IMAP 服务器只能是域名或 IP，例如 imap.example.com")
	}
	pt := 993
	if port != nil {
		pt = *port
	}
	if pt < 1 || pt > 65535 {
		return "", 0, httpx.Invalid("端口要在 1 到 65535 之间")
	}
	return h, pt, nil
}

// tryLogin checks the settings before they are saved.
func (m *Module) tryLogin(ctx context.Context, c imapConfig) error {
	cl, _, err := m.connect(ctx, c, nil)
	if err != nil {
		var ce *connError
		if errors.As(err, &ce) {
			return httpx.Invalid(ce.msg)
		}
		return httpx.Invalid("连不上邮箱：" + err.Error())
	}
	_ = cl.Logout().Wait()
	cl.Close()
	return nil
}

func (m *Module) ListMailAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListAccounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, err := m.unreadCounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.MailAccount, 0, len(rows))
	for _, a := range rows {
		out = append(out, m.accountView(a, counts[a.ID]))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateMailAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.MailAccountInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.createAccount(ctx, body)
	m.d.Audit.Record(ctx, "mail.account.create", strings.TrimSpace(body.Email), map[string]any{"provider": body.Provider}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.startWorker(a.ID)
	httpx.JSON(w, http.StatusCreated, m.accountView(a, 0))
}

func (m *Module) createAccount(ctx context.Context, in api.MailAccountInput) (db.MailAccount, error) {
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return db.MailAccount{}, httpx.Invalid("名称要写 1 到 50 个字")
	}
	if !strings.Contains(email, "@") || len(email) > 200 {
		return db.MailAccount{}, httpx.Invalid("邮箱地址不对")
	}
	if !in.Provider.Valid() {
		return db.MailAccount{}, httpx.Invalid("邮箱类型只能是 gmail、aliyun 或 other")
	}
	if in.Password == "" {
		return db.MailAccount{}, httpx.Invalid("请填密码")
	}
	host, port, err := cleanServer(in.Provider, in.ImapHost, in.ImapPort)
	if err != nil {
		return db.MailAccount{}, err
	}
	username := strings.TrimSpace(deref(in.Username))
	if username == "" {
		username = email
	}
	c := imapConfig{provider: in.Provider, host: host, port: port, username: username, password: in.Password}
	if err := m.tryLogin(ctx, c); err != nil {
		return db.MailAccount{}, err
	}
	sealed, err := m.d.Secrets.Seal(in.Password)
	if err != nil {
		return db.MailAccount{}, err
	}
	notify := int64(1)
	if in.Notify != nil && !*in.Notify {
		notify = 0
	}
	return m.q.CreateAccount(ctx, db.CreateAccountParams{
		Name: name, Email: email, Provider: string(in.Provider), ImapHost: host, ImapPort: int64(port),
		Username: username, PasswordEnc: sealed, Notify: notify, CreatedAt: m.now().UTC(),
	})
}

func (m *Module) UpdateMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	ctx := r.Context()
	var body api.MailAccountPatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	connChange := body.ImapHost != nil || body.ImapPort != nil || body.Username != nil || body.Password != nil
	if connChange {
		if err := auth.RequireElevated(ctx); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	a, reconnect, err := m.updateAccount(ctx, accountID, body)
	m.d.Audit.Record(ctx, "mail.account.update", a.Email, map[string]any{"id": accountID, "server": connChange}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if reconnect {
		m.startWorker(a.ID)
	}
	counts, err := m.unreadCounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("mail.updated", map[string]any{"accountId": a.ID})
	httpx.JSON(w, http.StatusOK, m.accountView(a, counts[a.ID]))
}

func (m *Module) updateAccount(ctx context.Context, id int64, in api.MailAccountPatch) (db.MailAccount, bool, error) {
	a, err := m.getAccount(ctx, id)
	if err != nil {
		return a, false, err
	}
	p := db.UpdateAccountParams{ID: a.ID, Name: a.Name, ImapHost: a.ImapHost, ImapPort: a.ImapPort, Username: a.Username, PasswordEnc: a.PasswordEnc, Notify: a.Notify}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > 50 {
			return a, false, httpx.Invalid("名称要写 1 到 50 个字")
		}
		p.Name = name
	}
	if in.Notify != nil {
		p.Notify = 0
		if *in.Notify {
			p.Notify = 1
		}
	}
	reconnect := false
	if in.ImapHost != nil || in.ImapPort != nil || in.Username != nil || in.Password != nil {
		host, port := in.ImapHost, in.ImapPort
		if host == nil {
			host = &a.ImapHost
		}
		if port == nil {
			old := int(a.ImapPort)
			port = &old
		}
		h, pt, err := cleanServer(api.MailProvider(a.Provider), host, port)
		if err != nil {
			return a, false, err
		}
		p.ImapHost, p.ImapPort = h, int64(pt)
		if in.Username != nil {
			if u := strings.TrimSpace(*in.Username); u != "" {
				p.Username = u
			}
		}
		c, err := m.accountConfig(a)
		if err != nil && in.Password == nil {
			return a, false, httpx.Invalid(err.Error())
		}
		c.host, c.port, c.username = p.ImapHost, pt, p.Username
		if in.Password != nil && *in.Password != "" {
			c.password = *in.Password
			if p.PasswordEnc, err = m.d.Secrets.Seal(*in.Password); err != nil {
				return a, false, err
			}
		}
		if err := m.tryLogin(ctx, c); err != nil {
			return a, false, err
		}
		reconnect = true
	}
	a, err = m.q.UpdateAccount(ctx, p)
	return a, reconnect, err
}

func (m *Module) DeleteMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, err := m.getAccount(ctx, accountID)
	if err == nil {
		m.stopWorker(a.ID)
		if err = m.q.DeleteAccountMessages(ctx, a.ID); err == nil {
			err = m.q.DeleteAccount(ctx, a.ID)
		}
		m.deleteFiles(ctx, itoa(a.ID))
	}
	m.d.Audit.Record(ctx, "mail.account.delete", a.Email, map[string]any{"id": accountID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("mail.updated", map[string]any{"accountId": accountID})
	// B113: mute rules of this mailbox go with it
	m.d.Bus.Publish("notify.scope_removed", map[string]any{"scope": fmt.Sprintf("mail:%d", accountID)})
	httpx.NoContent(w)
}

func (m *Module) SyncMailAccount(w http.ResponseWriter, r *http.Request, accountID api.AccountId) {
	if _, err := m.getAccount(r.Context(), accountID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.pokeWorker(accountID)
	w.WriteHeader(http.StatusAccepted)
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

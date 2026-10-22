package mail_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const (
	user = "me@example.com"
	pass = "app-password"
)

// fakeIMAP is an in-memory IMAP server behind TLS.
type fakeIMAP struct {
	addr string
	port int
	user *imapmemserver.User
	tls  *tls.Config // what clients use to trust it

	logins              atomic.Int32 // 登录过多少次，用来看有没有另开连接
	lossy               atomic.Bool  // 设上以后 STORE 不保存任何状态
	bodyFetches         atomic.Int32
	failBodies          atomic.Bool
	failEnvelope        atomic.Bool
	failHeaderFields    atomic.Bool
	reorderHeaderFields atomic.Bool
	headerFieldFetches  atomic.Int32
	fullHeaderFetches   atomic.Int32
	failCopy            atomic.Bool
	failCreate          atomic.Bool
	creates             atomic.Int32
	archiveFolder       string // LIST 返回带有 \\Archive 的文件夹
	allFolder           string // LIST 返回带有 \\All 的文件夹
	bodyStarted         chan struct{}
	bodyRelease         chan struct{}
}

// session is the fake server's session. In lossy mode STORE answers with the
// old flags, like a server that does not keep what it is told.
type session struct {
	*imapmemserver.UserSession
	f   *fakeIMAP
	usr *imapmemserver.User
}

func (s *session) Login(username, password string) error {
	if username != user {
		return imapserver.ErrAuthFailed
	}
	if err := s.usr.Login(username, password); err != nil {
		return err
	}
	s.f.logins.Add(1)
	s.UserSession = imapmemserver.NewUserSession(s.usr)
	return nil
}

func (s *session) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	if s.f.lossy.Load() {
		none := *flags
		none.Flags = nil
		flags = &none
	}
	return s.UserSession.Store(w, numSet, flags, options)
}

func (s *session) Copy(set imap.NumSet, folder string) (*imap.CopyData, error) {
	if s.f.failCopy.Load() {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "copy denied"}
	}
	return s.UserSession.Copy(set, folder)
}

func (s *session) Create(name string, options *imap.CreateOptions) error {
	s.f.creates.Add(1)
	if s.f.failCreate.Load() {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "create denied"}
	}
	return s.UserSession.Create(name, options)
}

func (s *session) Fetch(w *imapserver.FetchWriter, set imap.NumSet, options *imap.FetchOptions) error {
	if options.Envelope && s.f.failEnvelope.Load() {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "broken ENVELOPE"}
	}
	for _, section := range options.BodySection {
		if len(section.HeaderFields) > 0 {
			if s.f.reorderHeaderFields.Load() {
				for i, j := 0, len(section.HeaderFields)-1; i < j; i, j = i+1, j-1 {
					section.HeaderFields[i], section.HeaderFields[j] = section.HeaderFields[j], section.HeaderFields[i]
				}
			}
			s.f.headerFieldFetches.Add(1)
			if s.f.failHeaderFields.Load() {
				return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "invalid command or parameters"}
			}
		} else if section.Specifier == imap.PartSpecifierHeader {
			s.f.fullHeaderFetches.Add(1)
		}
		if len(section.Part) == 0 && section.Partial != nil && section.Partial.Size > 2048 {
			if s.f.bodyFetches.Add(1) == 1 && s.f.bodyStarted != nil {
				close(s.f.bodyStarted)
				<-s.f.bodyRelease
			}
			if s.f.failBodies.Load() {
				return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "body fetch failed"}
			}
		}
	}
	return s.UserSession.Fetch(w, set, options)
}

func (s *session) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	if s.f.allFolder != "" && options.ReturnSpecialUse {
		if err := w.WriteList(&imap.ListData{Mailbox: s.f.allFolder, Attrs: []imap.MailboxAttr{imap.MailboxAttrAll}, Delim: '/'}); err != nil {
			return err
		}
	}
	if s.f.archiveFolder != "" && options.ReturnSpecialUse {
		if err := w.WriteList(&imap.ListData{Mailbox: s.f.archiveFolder, Attrs: []imap.MailboxAttr{imap.MailboxAttrArchive}, Delim: '/'}); err != nil {
			return err
		}
	}
	return s.UserSession.List(w, ref, patterns, options)
}

func (s *session) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	// imapmemserver's Idle misses updates queued between the last command's
	// Poll and entering Idle. Flush pending updates too, as a real server does.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := s.UserSession.Poll(w, true); err != nil {
			return err
		}
		select {
		case <-stop:
			return nil
		case <-ticker.C:
		}
	}
}

func newFakeIMAP(t *testing.T, extraCaps ...imap.Cap) *fakeIMAP {
	t.Helper()
	h := httptest.NewTLSServer(http.NotFoundHandler()) // 借它的证书
	certs := h.TLS.Certificates
	pool := x509.NewCertPool()
	pool.AddCert(h.Certificate())
	h.Close()

	u := imapmemserver.NewUser(user, pass)
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	fake := &fakeIMAP{user: u}
	caps := imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIdle: {}}
	for _, cap := range extraCaps {
		caps[cap] = struct{}{}
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &session{f: fake, usr: u}, nil, nil
		},
		Caps:         caps,
		InsecureAuth: true,
		Logger:       quietLogger{},
	})
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certs})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	fake.addr, fake.port, fake.tls = ln.Addr().String(), ln.Addr().(*net.TCPAddr).Port, &tls.Config{RootCAs: pool}
	return fake
}

type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}

type literal struct{ *bytes.Reader }

func (l literal) Size() int64 { return int64(l.Len()) }

func (f *fakeIMAP) add(t *testing.T, raw string, at time.Time) {
	t.Helper()
	b := []byte(strings.ReplaceAll(raw, "\n", "\r\n"))
	if _, err := f.user.Append("INBOX", literal{bytes.NewReader(b)}, &imap.AppendOptions{Time: at}); err != nil {
		t.Fatal(err)
	}
}

// client opens a separate connection, like another device would.
func (f *fakeIMAP) client(t *testing.T) *imapclient.Client {
	t.Helper()
	cfg := f.tls.Clone()
	cfg.ServerName = "127.0.0.1"
	c, err := imapclient.DialTLS(f.addr, &imapclient.Options{TLSConfig: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login(user, pass).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func gbk(s string) []byte {
	b, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	return b
}

const plainMail = `From: Alice <alice@example.com>
To: me@example.com
Subject: Hello
Message-ID: <a1@example.com>
Date: Wed, 01 Oct 2026 08:00:00 +0800
Content-Type: text/plain; charset=utf-8

Hi there, this is the first mail.
`

func gbkMail() string {
	return fmt.Sprintf(`From: =?GBK?B?%s?= <wang@example.cn>
To: me@example.com
Subject: =?GBK?B?%s?=
Date: Wed, 01 Oct 2026 08:10:00 +0800
MIME-Version: 1.0
Content-Type: text/plain; charset=GBK
Content-Transfer-Encoding: base64

%s
`, base64.StdEncoding.EncodeToString(gbk("小王")), base64.StdEncoding.EncodeToString(gbk("你好世界")),
		base64.StdEncoding.EncodeToString(gbk("这是一封测试邮件，用的是 GBK 编码。")))
}

const htmlMail = `From: Bob <bob@example.com>
To: Me <me@example.com>
Cc: Carol <carol@example.com>
Subject: Report
Date: Wed, 01 Oct 2026 08:20:00 +0800
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="mix"

--mix
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=utf-8

<html><head><style>p{color:red}</style></head><body><p>Hello <b>report</b></p><img src="cid:logo@x"></body></html>
--rel
Content-Type: image/png
Content-ID: <logo@x>
Content-Disposition: inline
Content-Transfer-Encoding: base64

iVBORw0KGgo=
--rel--

--mix
Content-Type: application/pdf; name="r.pdf"
Content-Disposition: attachment; filename="r.pdf"
Content-Transfer-Encoding: base64

JVBERi0=
--mix--
`

func setup(t *testing.T, extraCaps ...imap.Cap) (*testutil.Env, *mail.Module, *fakeIMAP) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*mail.Module](env.App.Deps.Registry, mail.ServiceKey)
	if !ok {
		t.Fatal("mail module not registered")
	}
	f := newFakeIMAP(t, extraCaps...)
	mail.SetTLSConfig(m, f.tls)
	mail.SetTimings(m, 200*time.Millisecond, time.Second)
	env.Elevate()
	return env, m, f
}

func accountInput(f *fakeIMAP, password string) map[string]any {
	return map[string]any{"name": "测试邮箱", "email": user, "provider": "other", "imapHost": "127.0.0.1", "imapPort": f.port, "password": password}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func list(t *testing.T, env *testutil.Env, query string) []api.MailSummary {
	t.Helper()
	var page api.MailPage
	env.MustDo(http.MethodGet, "/mail/messages"+query, nil, &page)
	return page.Items
}

func bySubject(items []api.MailSummary, subject string) *api.MailSummary {
	for i := range items {
		if items[i].Subject == subject {
			return &items[i]
		}
	}
	return nil
}

func mailNotes(t *testing.T, env *testutil.Env) []struct{ Kind, Title, Link string } {
	t.Helper()
	var out struct {
		Items []struct{ Kind, Title, Link string }
	}
	env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	var keep []struct{ Kind, Title, Link string }
	for _, n := range out.Items {
		if n.Kind == "mail.new" {
			keep = append(keep, n)
		}
	}
	return keep
}

func TestAccountNeedsWorkingLogin(t *testing.T) {
	env, _, f := setup(t)
	status, body := env.Do(http.MethodPost, "/mail/accounts", accountInput(f, "wrong"), nil)
	if status != http.StatusBadRequest || !strings.Contains(string(body), "用户名或密码不对") {
		t.Fatalf("wrong password: %d %s", status, body)
	}
	var accounts []api.MailAccount
	env.MustDo(http.MethodGet, "/mail/accounts", nil, &accounts)
	if len(accounts) != 0 {
		t.Fatalf("saved anyway: %+v", accounts)
	}
	in := accountInput(f, pass)
	in["imapPort"] = 1 // 没人监听
	if status, body := env.Do(http.MethodPost, "/mail/accounts", in, nil); status != http.StatusBadRequest || !strings.Contains(string(body), "连不上服务器") {
		t.Fatalf("closed port: %d %s", status, body)
	}
}

func TestSyncContinuesWithoutEnvelope(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "initial healthy sync", func() bool { return len(list(t, env, "")) == 1 })

	// Simulate a server whose ENVELOPE stops working after the first sync.
	f.failEnvelope.Store(true)
	raw := strings.Replace(plainMail, "Subject: Hello", "Subject: Alice's \"status\"\n update", 1)
	raw = strings.Replace(raw, "From: Alice <alice@example.com>", "From: \"Alice's \\\"team\\\"\" <alice@example.com>", 1)
	f.add(t, raw, time.Now())
	eventually(t, "new message without ENVELOPE", func() bool { return len(list(t, env, "")) == 2 })
	x := bySubject(list(t, env, ""), "Alice's \"status\" update")
	if x == nil || x.From.Address != "alice@example.com" || x.From.Name == nil || *x.From.Name != "Alice's \"team\"" || !x.Unread {
		t.Fatalf("headers or unread state: %+v", x)
	}
	eventually(t, "new mail notification", func() bool { return len(mailNotes(t, env)) == 1 })
	// A later message must also arrive; the bad envelope must not stall the UID.
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Following mail", 1), time.Now())
	eventually(t, "following message", func() bool { return len(list(t, env, "")) == 3 })
	var accounts []api.MailAccount
	env.MustDo(http.MethodGet, "/mail/accounts", nil, &accounts)
	if len(accounts) != 1 || accounts[0].Status != api.Ok {
		t.Fatalf("account did not stay healthy: %+v", accounts)
	}
}

func TestSyncFallsBackToFullHeader(t *testing.T) {
	env, _, f := setup(t)
	// Simulate a server rejecting HEADER.FIELDS. Neither request may use ENVELOPE.
	f.failHeaderFields.Store(true)
	f.failEnvelope.Store(true)
	f.add(t, gbkMail(), time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "initial sync with full headers", func() bool { return len(list(t, env, "")) == 1 })
	first := bySubject(list(t, env, ""), "你好世界")
	if first == nil || first.From.Name == nil || *first.From.Name != "小王" || !first.Unread {
		t.Fatalf("fallback headers or unread state: %+v", first)
	}
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Header fallback", 1), time.Now())
	eventually(t, "incremental sync with full headers", func() bool { return len(list(t, env, "")) == 2 })
	following := bySubject(list(t, env, ""), "Header fallback")
	if following == nil || following.From.Address != "alice@example.com" || !following.Unread {
		t.Fatalf("incremental fallback: %+v", following)
	}
	eventually(t, "fallback mail notification", func() bool { return len(mailNotes(t, env)) == 1 })
	if f.headerFieldFetches.Load() < 2 || f.fullHeaderFetches.Load() < 2 {
		t.Fatalf("missing fallback requests: fields=%d header=%d", f.headerFieldFetches.Load(), f.fullHeaderFetches.Load())
	}
	other := f.client(t)
	msgs, err := other.Fetch(imap.UIDSetNum(1, 2), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil || len(msgs) != 2 {
		t.Fatalf("fetch flags: messages=%d err=%v", len(msgs), err)
	}
	for _, msg := range msgs {
		for _, flag := range msg.Flags {
			if flag == imap.FlagSeen {
				t.Fatalf("fallback marked UID %d read", msg.UID)
			}
		}
	}
}

func TestSyncIdleFlagsAndBody(t *testing.T) {
	env, m, f := setup(t)
	old := time.Now().Add(-time.Hour)
	f.add(t, plainMail, old)
	f.add(t, gbkMail(), old)
	f.add(t, htmlMail, old)

	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	_, raw := env.Do(http.MethodGet, "/mail/accounts", nil, nil)
	if strings.Contains(string(raw), pass) {
		t.Fatalf("password leaked: %s", raw)
	}
	eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 3 })

	items := list(t, env, "")
	g := bySubject(items, "你好世界")
	if g == nil || g.From.Name == nil || *g.From.Name != "小王" || !strings.Contains(g.Snippet, "这是一封测试邮件") {
		t.Fatalf("gbk: %+v", items)
	}
	rep := bySubject(items, "Report")
	if rep == nil || !rep.HasAttachments || rep.Snippet != "Hello report" || !rep.Unread {
		t.Fatalf("html summary: %+v", rep)
	}
	if hello := bySubject(items, "Hello"); hello == nil || hello.HasAttachments || !strings.HasPrefix(hello.Snippet, "Hi there") {
		t.Fatalf("plain summary: %+v", hello)
	}
	if got := list(t, env, "?q=report"); len(got) != 1 {
		t.Fatalf("search: %+v", got)
	}
	if got := list(t, env, "?limit=2"); len(got) != 2 {
		t.Fatalf("limit: %+v", got)
	}
	if n := mailNotes(t, env); len(n) != 0 {
		t.Fatalf("old mail pushed: %+v", n)
	}
	var accounts []api.MailAccount
	env.MustDo(http.MethodGet, "/mail/accounts", nil, &accounts)
	if len(accounts) != 1 || accounts[0].Status != api.Ok || accounts[0].Idle == nil || !*accounts[0].Idle || accounts[0].Unread != 3 {
		t.Fatalf("account: %+v", accounts)
	}

	// 新邮件通过 IDLE 几秒内就到，并且推送
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Fresh news", 1), time.Now())
	eventually(t, "idle delivery", func() bool { return len(list(t, env, "")) == 4 })
	// 邮件先入库再推送，列表里有了不代表推送已经写好。
	eventually(t, "idle notification", func() bool { return len(mailNotes(t, env)) > 0 })
	notes := mailNotes(t, env)
	if len(notes) != 1 || notes[0].Title != "Alice：Fresh news" || !strings.HasPrefix(notes[0].Link, "/mail?m=") {
		t.Fatalf("notify: %+v", notes)
	}

	// 正文、附件
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", rep.Id), nil, &msg)
	if msg.Html == nil || !strings.Contains(*msg.Html, "cid:logo@x") || !strings.Contains(msg.Text, "Hello report") || strings.Contains(msg.Text, "color") {
		t.Fatalf("body: %+v", msg)
	}
	if len(msg.To) != 1 || msg.To[0].Address != user || len(msg.Cc) != 1 || len(msg.Attachments) != 2 {
		t.Fatalf("addresses or attachments: %+v", msg)
	}
	logo, pdf := msg.Attachments[0], msg.Attachments[1]
	if logo.ContentId == nil || *logo.ContentId != "logo@x" || logo.Mime != "image/png" || pdf.Name != "r.pdf" || pdf.Size != 5 {
		t.Fatalf("attachments: %+v", msg.Attachments)
	}
	resp, err := env.Client.Get(env.URL(fmt.Sprintf("/mail/messages/%d/attachments/0", rep.Id)))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") || resp.Header.Get("X-Content-Type-Options") != "nosniff" || len(b) != 8 {
		t.Fatalf("inline image: %d %v %d", resp.StatusCode, resp.Header, len(b))
	}
	resp, err = env.Client.Get(env.URL(fmt.Sprintf("/mail/messages/%d/attachments/1", rep.Id)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("pdf: %v", resp.Header)
	}
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 2 {
		t.Fatal("attachments not stored")
	}
	var gm api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", g.Id), nil, &gm)
	if !strings.Contains(gm.Text, "GBK 编码") {
		t.Fatalf("gbk body: %q", gm.Text)
	}

	// 标成已读后，邮箱服务器上也有 \Seen
	var sum api.MailSummary
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", rep.Id), map[string]any{"unread": false}, &sum)
	if sum.Unread {
		t.Fatal("still unread")
	}
	other := f.client(t)
	seen, err := other.Fetch(imap.UIDSetNum(3), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil || len(seen) != 1 || !hasFlag(seen[0].Flags, imap.FlagSeen) {
		t.Fatalf("server flags: %+v %v", seen, err)
	}

	// 别的设备上标星，这里也跟着变
	if err := other.Store(imap.UIDSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagFlagged, imap.FlagSeen}}, nil).Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "remote flag", func() bool {
		h := bySubject(list(t, env, ""), "Hello")
		return h != nil && h.Flagged && !h.Unread
	})
	if got := list(t, env, "?unread=true"); len(got) != 2 {
		t.Fatalf("unread filter: %+v", got)
	}
	var summary api.MailSummaryView
	env.MustDo(http.MethodGet, "/mail/summary", nil, &summary)
	if summary.Unread != 2 || len(summary.Accounts) != 1 || len(summary.Latest) != 2 {
		t.Fatalf("summary: %+v", summary)
	}

	// UIDVALIDITY 变了：重连后本地重新同步
	if err := f.user.Delete("INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := f.user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	f.add(t, plainMail, old)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/accounts/%d", acc.Id), map[string]any{"password": pass}, nil)
	eventually(t, "resync", func() bool {
		got := list(t, env, "")
		return len(got) == 1 && got[0].Subject == "Hello"
	})
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 0 {
		t.Fatal("old attachments kept after UIDVALIDITY change")
	}

	// 删除账号：邮件和文件都删掉
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", list(t, env, "")[0].Id), nil, nil)
	env.MustDo(http.MethodDelete, fmt.Sprintf("/mail/accounts/%d", acc.Id), nil, nil)
	if got := list(t, env, ""); len(got) != 0 {
		t.Fatalf("messages left: %+v", got)
	}
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 0 {
		t.Fatal("files left")
	}
}

func TestManyNewMailsAreGrouped(t *testing.T) {
	env, _, f := setup(t)
	for i := range 5 {
		f.add(t, strings.Replace(plainMail, "Subject: Hello", fmt.Sprintf("Subject: Batch %d", i), 1), time.Now())
	}
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), nil)
	eventually(t, "batch", func() bool { return len(list(t, env, "")) == 5 })
	eventually(t, "notice", func() bool { return len(mailNotes(t, env)) > 0 })
	notes := mailNotes(t, env)
	if len(notes) != 1 || notes[0].Title != "测试邮箱 收到 5 封新邮件" {
		t.Fatalf("grouped: %+v", notes)
	}
}

func TestSnippetText(t *testing.T) {
	// base64 被截断在中间也能解出前面的字
	raw := base64.StdEncoding.EncodeToString(gbk("这是一封很长的测试邮件"))
	got := mail.SnippetText([]byte(raw[:len(raw)-3]), "base64", "gb2312", false)
	if !strings.HasPrefix(got, "这是一封") {
		t.Fatalf("gbk base64: %q", got)
	}
	got = mail.SnippetText([]byte("Caf=C3=A9 =\r\nau lait"), "quoted-printable", "utf-8", false)
	if got != "Café au lait" {
		t.Fatalf("qp: %q", got)
	}
	got = mail.SnippetText([]byte("<style>x{}</style><p>Hi</p><p>there</p>"), "7bit", "", true)
	if got != "Hi there" {
		t.Fatalf("html: %q", got)
	}
	// B92：营销邮件的 Outlook 条件注释、放在纯文本里的 HTML、被截断的注释
	cases := map[string]string{
		"<!--[if !mso]><!--><div>你好&nbsp;世界</div><!--<![endif]-->":         "你好 世界",
		"<!--[if mso]><table><tr><td>旧版</td></tr></table><![endif]-->本周优惠": "本周优惠",
		"[if !mso]><!--> 活动开始了 <![endif]-->":                               "活动开始了",
		"&lt;p&gt;转义过的 HTML&lt;/p&gt;":                                     "转义过的 HTML",
		"价格 a < b，照常显示":                                                    "价格 a < b，照常显示",
	}
	for in, want := range cases {
		if got := mail.CleanText(in); got != want {
			t.Errorf("clean %q: %q, want %q", in, got, want)
		}
	}
	got = mail.SnippetText([]byte("<!--[if !mso]><!-->\r\n<p>Hello</p>"), "7bit", "", false)
	if got != "Hello" {
		t.Fatalf("html in text/plain: %q", got)
	}
}

func hasFlag(flags []imap.Flag, f imap.Flag) bool {
	for _, x := range flags {
		if strings.EqualFold(string(x), string(f)) {
			return true
		}
	}
	return false
}

// captureRouter keeps every notification the service routes and sends it
// nowhere.
type captureRouter struct {
	mu  sync.Mutex
	got []notify.Stored
}

func (c *captureRouter) Route(_ context.Context, n notify.Stored) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, n)
	return nil
}

func (c *captureRouter) mail() []notify.Stored {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []notify.Stored
	for _, n := range c.got {
		if n.Kind == "mail.new" {
			out = append(out, n)
		}
	}
	return out
}

// B113: new mail says which mailbox it came to, so mute rules can name it, and
// a deleted mailbox tells the notification rules to forget it.
func TestNewMailCarriesItsMailboxAndDeleteAnnouncesIt(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	capture := &captureRouter{}
	env.App.Deps.Notify.SetRouter(capture)
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 1 })

	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Fresh news", 1), time.Now())
	eventually(t, "notification", func() bool { return len(capture.mail()) == 1 })
	n := capture.mail()[0]
	if n.Scope != fmt.Sprintf("mail:%d", acc.Id) {
		t.Fatalf("scope = %q", n.Scope)
	}

	removed, cancel := env.App.Deps.Bus.Subscribe("notify.scope_removed", 4)
	defer cancel()
	env.MustDo(http.MethodDelete, fmt.Sprintf("/mail/accounts/%d", acc.Id), nil, nil)
	select {
	case ev := <-removed:
		data, _ := ev.Data.(map[string]any)
		if data["scope"] != fmt.Sprintf("mail:%d", acc.Id) {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no notify.scope_removed event")
	}
}

// serverFlags returns the flags the server holds for one UID of INBOX.
func serverFlags(t *testing.T, c *imapclient.Client, uid imap.UID) []imap.Flag {
	t.Helper()
	got, err := c.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil || len(got) != 1 {
		t.Fatalf("fetch flags of %d: %+v %v", uid, got, err)
	}
	return got[0].Flags
}

// connected waits until the account's connection is up and idling.
func connected(t *testing.T, env *testutil.Env, id int64) {
	t.Helper()
	eventually(t, "account connected", func() bool {
		var accounts []api.MailAccount
		env.MustDo(http.MethodGet, "/mail/accounts", nil, &accounts)
		for _, a := range accounts {
			if a.Id == id && a.Status == api.Ok && a.Idle != nil && *a.Idle {
				return true
			}
		}
		return false
	})
	time.Sleep(200 * time.Millisecond) // 第一次同步做完，进到 IDLE
}

// B217: 标成已读以后，后面几次同步不能把它改回未读。
func TestReadStaysReadAcrossSyncs(t *testing.T) {
	env, _, f := setup(t)
	old := time.Now().Add(-time.Hour)
	for _, s := range []string{"One", "Two", "Three"} {
		f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: "+s, 1), old)
	}
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 3 })
	connected(t, env, acc.Id)

	one := bySubject(list(t, env, ""), "One")
	other := f.client(t)
	logins := f.logins.Load()
	var sum api.MailSummary
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", one.Id), map[string]any{"unread": false}, &sum)
	if sum.Unread {
		t.Fatal("still unread")
	}
	if n := f.logins.Load(); n != logins {
		t.Fatalf("PATCH opened %d extra connections, want it done on the live one", n-logins)
	}
	if !hasFlag(serverFlags(t, other, 1), imap.FlagSeen) {
		t.Fatal("server has no \\Seen")
	}

	// 让同步跑几次：别的设备给另外两封加星标，每次都会触发一次同步
	for i, subject := range []string{"Two", "Three"} {
		if err := other.Store(imap.UIDSetNum(imap.UID(i+2)), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagFlagged}}, nil).Close(); err != nil {
			t.Fatal(err)
		}
		eventually(t, subject+" flagged", func() bool {
			x := bySubject(list(t, env, ""), subject)
			return x != nil && x.Flagged
		})
		if x := bySubject(list(t, env, ""), "One"); x == nil || x.Unread {
			t.Fatalf("after sync %d, One is unread again: %+v", i+1, x)
		}
	}
	if !hasFlag(serverFlags(t, other, 1), imap.FlagSeen) {
		t.Fatal("server lost \\Seen")
	}

	// 标星、取消星标、改回未读，也走同一条连接
	for _, step := range []struct {
		body map[string]any
		want func(api.MailSummary) bool
		flag bool
		seen bool
	}{
		{map[string]any{"flagged": true}, func(s api.MailSummary) bool { return s.Flagged && !s.Unread }, true, true},
		{map[string]any{"flagged": false, "unread": true}, func(s api.MailSummary) bool { return !s.Flagged && s.Unread }, false, false},
	} {
		env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", one.Id), step.body, &sum)
		fl := serverFlags(t, other, 1)
		if !step.want(sum) || hasFlag(fl, imap.FlagFlagged) != step.flag || hasFlag(fl, imap.FlagSeen) != step.seen {
			t.Fatalf("patch %v: %+v, server %v", step.body, sum, fl)
		}
	}
	if n := f.logins.Load(); n != logins {
		t.Fatalf("PATCH opened %d extra connections", n-logins)
	}
}

// B217: 服务器说存上了但状态没变，要回 502，本地不改。
func TestPatchFailsWhenServerKeepsNothing(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 1 })
	connected(t, env, acc.Id)
	id := list(t, env, "")[0].Id

	f.lossy.Store(true)
	status, body := env.Do(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", id), map[string]any{"unread": false}, nil)
	if status != http.StatusBadGateway || !strings.Contains(string(body), "mail_unavailable") || !strings.Contains(string(body), "没有保存") {
		t.Fatalf("lossy server: %d %s", status, body)
	}
	if got := list(t, env, ""); len(got) != 1 || !got[0].Unread {
		t.Fatalf("local changed anyway: %+v", got)
	}

	f.lossy.Store(false)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", id), map[string]any{"unread": false}, nil)
	if got := list(t, env, ""); got[0].Unread {
		t.Fatalf("still unread: %+v", got)
	}
}

type batchResult = api.MailBatchResult

func batch(t *testing.T, env *testutil.Env, action string, ids ...int64) batchResult {
	t.Helper()
	var out batchResult
	env.MustDo(http.MethodPost, "/mail/messages/batch", map[string]any{"ids": ids, "action": action}, &out)
	return out
}

func idsOf(t *testing.T, items []api.MailSummary, subjects ...string) []int64 {
	t.Helper()
	var ids []int64
	for _, s := range subjects {
		x := bySubject(items, s)
		if x == nil {
			t.Fatalf("no mail %q in %+v", s, items)
		}
		ids = append(ids, x.Id)
	}
	return ids
}

func batchSetup(t *testing.T) (*testutil.Env, *mail.Module, *fakeIMAP, api.MailAccount) {
	t.Helper()
	env, m, f := setup(t)
	for _, s := range []string{"One", "Two", "Three", "Four"} {
		f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: "+s, 1), time.Now().Add(-time.Hour))
	}
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 4 })
	connected(t, env, acc.Id)
	return env, m, f, acc
}

// B219
func TestBatchReadUnreadStar(t *testing.T) {
	env, _, f, _ := batchSetup(t)
	other := f.client(t)
	items := list(t, env, "")
	ids := idsOf(t, items, "One", "Two")

	if r := batch(t, env, "read", append(ids, ids[0])...); r.Done != 2 || len(r.Errors) != 0 { // 重复的 id 只算一次
		t.Fatalf("read: %+v", r)
	}
	items = list(t, env, "")
	for _, s := range []string{"One", "Two"} {
		if x := bySubject(items, s); x.Unread {
			t.Fatalf("%s still unread", s)
		}
	}
	if x := bySubject(items, "Three"); !x.Unread {
		t.Fatal("Three should stay unread")
	}
	for _, uid := range []imap.UID{1, 2} {
		if !hasFlag(serverFlags(t, other, uid), imap.FlagSeen) {
			t.Fatalf("server: uid %d has no \\Seen", uid)
		}
	}
	if hasFlag(serverFlags(t, other, 3), imap.FlagSeen) {
		t.Fatal("server: uid 3 marked read")
	}

	if r := batch(t, env, "star", ids...); r.Done != 2 {
		t.Fatalf("star: %+v", r)
	}
	if !hasFlag(serverFlags(t, other, 1), imap.FlagFlagged) || !bySubject(list(t, env, ""), "One").Flagged {
		t.Fatal("star not saved")
	}
	if r := batch(t, env, "unstar", ids[0]); r.Done != 1 {
		t.Fatalf("unstar: %+v", r)
	}
	if hasFlag(serverFlags(t, other, 1), imap.FlagFlagged) || bySubject(list(t, env, ""), "One").Flagged {
		t.Fatal("unstar not saved")
	}
	if r := batch(t, env, "unread", ids...); r.Done != 2 {
		t.Fatalf("unread: %+v", r)
	}
	if hasFlag(serverFlags(t, other, 2), imap.FlagSeen) || !bySubject(list(t, env, ""), "Two").Unread {
		t.Fatal("unread not saved")
	}

	// 同步以后状态不变
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Five", 1), time.Now().Add(-time.Hour))
	eventually(t, "sync", func() bool { return len(list(t, env, "")) == 5 })
	if x := bySubject(list(t, env, ""), "Two"); !x.Unread || !x.Flagged {
		t.Fatalf("after sync: %+v", x)
	}

	// 不存在的 id 忽略
	if r := batch(t, env, "read", 99999); r.Done != 0 || len(r.Errors) != 0 {
		t.Fatalf("unknown id: %+v", r)
	}
}

func TestBatchValidation(t *testing.T) {
	env, _, _, _ := batchSetup(t)
	tooMany := make([]int64, 501)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for name, body := range map[string]map[string]any{
		"empty":    {"ids": []int64{}, "action": "read"},
		"too many": {"ids": tooMany, "action": "read"},
		"action":   {"ids": []int64{1}, "action": "burn"},
	} {
		if status, out := env.Do(http.MethodPost, "/mail/messages/batch", body, nil); status != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, status, out)
		}
	}
	if status, out := env.Do(http.MethodPost, "/mail/read-all", map[string]any{"accountId": 9999}, nil); status != http.StatusNotFound {
		t.Fatalf("read-all unknown account: %d %s", status, out)
	}
}

// B219: 有“已删除”文件夹就移过去。
func TestBatchDeleteMovesToTrash(t *testing.T) {
	env, m, f, acc := batchSetup(t)
	if err := f.user.Create("Trash", nil); err != nil {
		t.Fatal(err)
	}
	items := list(t, env, "")
	rep := idsOf(t, items, "One")[0]
	// 先看一封带附件的，删除时附件文件也要删
	f.add(t, htmlMail, time.Now().Add(-time.Hour))
	eventually(t, "report", func() bool { return bySubject(list(t, env, ""), "Report") != nil })
	report := bySubject(list(t, env, ""), "Report")
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", report.Id), nil, nil)
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 2 {
		t.Fatal("attachments not stored")
	}

	if r := batch(t, env, "delete", rep, report.Id); r.Done != 2 || len(r.Errors) != 0 {
		t.Fatalf("delete: %+v", r)
	}
	left := list(t, env, "")
	if len(left) != 3 || bySubject(left, "One") != nil || bySubject(left, "Report") != nil {
		t.Fatalf("local: %+v", left)
	}
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 0 {
		t.Fatal("attachment files left")
	}
	other := f.client(t)
	inbox, err := other.Fetch(imap.UIDSetNum(1, 2, 3, 4, 5), &imap.FetchOptions{UID: true, Envelope: true}).Collect()
	if err != nil || len(inbox) != 3 {
		t.Fatalf("server INBOX: %+v %v", inbox, err)
	}
	for _, b := range inbox {
		if b.Envelope.Subject == "One" || b.Envelope.Subject == "Report" {
			t.Fatalf("still in INBOX: %s", b.Envelope.Subject)
		}
	}
	sel, err := other.Select("Trash", nil).Wait()
	if err != nil || sel.NumMessages != 2 {
		t.Fatalf("Trash: %+v %v", sel, err)
	}
	// 之后同步不会把它们带回来
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Six", 1), time.Now().Add(-time.Hour))
	eventually(t, "sync", func() bool { return bySubject(list(t, env, ""), "Six") != nil })
	if got := list(t, env, ""); len(got) != 4 {
		t.Fatalf("after sync: %+v", got)
	}
}

// B219: 没有“已删除”文件夹就彻底删除。
func TestBatchDeleteWithoutTrash(t *testing.T) {
	env, _, f, _ := batchSetup(t)
	ids := idsOf(t, list(t, env, ""), "Two", "Three")
	if r := batch(t, env, "delete", ids...); r.Done != 2 || len(r.Errors) != 0 {
		t.Fatalf("delete: %+v", r)
	}
	if got := list(t, env, ""); len(got) != 2 || bySubject(got, "Two") != nil {
		t.Fatalf("local: %+v", got)
	}
	other := f.client(t)
	inbox, err := other.Fetch(imap.UIDSetNum(1, 2, 3, 4), &imap.FetchOptions{UID: true}).Collect()
	if err != nil || len(inbox) != 2 || inbox[0].UID != 1 || inbox[1].UID != 4 {
		t.Fatalf("server INBOX: %+v %v", inbox, err)
	}
}

// B219: 账号连不上时只报那个账号的错，本地不改。
func TestBatchReportsFailedAccount(t *testing.T) {
	env, _, f, acc := batchSetup(t)
	ids := idsOf(t, list(t, env, ""), "One")
	f.lossy.Store(true)
	var r batchResult
	env.MustDo(http.MethodPost, "/mail/messages/batch", map[string]any{"ids": ids, "action": "read"}, &r)
	if r.Done != 0 || len(r.Errors) != 1 || !strings.HasPrefix(r.Errors[0], "测试邮箱：") {
		t.Fatalf("result: %+v (account %d)", r, acc.Id)
	}
	if x := bySubject(list(t, env, ""), "One"); !x.Unread {
		t.Fatal("local changed anyway")
	}
}

func TestMarkAllRead(t *testing.T) {
	env, _, f, acc := batchSetup(t)
	other := f.client(t)
	if err := other.Store(imap.UIDSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "one read", func() bool { return len(list(t, env, "?unread=true")) == 3 })

	var r batchResult
	env.MustDo(http.MethodPost, "/mail/read-all", map[string]any{"accountId": acc.Id}, &r)
	if r.Done != 3 || len(r.Errors) != 0 {
		t.Fatalf("read-all: %+v", r)
	}
	if got := list(t, env, "?unread=true"); len(got) != 0 {
		t.Fatalf("still unread: %+v", got)
	}
	var summary api.MailSummaryView
	env.MustDo(http.MethodGet, "/mail/summary", nil, &summary)
	if summary.Unread != 0 {
		t.Fatalf("summary: %+v", summary)
	}
	for uid := imap.UID(1); uid <= 4; uid++ {
		if !hasFlag(serverFlags(t, other, uid), imap.FlagSeen) {
			t.Fatalf("server: uid %d unread", uid)
		}
	}

	// 没有未读了，再来一次什么也不做；不传 accountId 就是所有邮箱
	env.MustDo(http.MethodPost, "/mail/read-all", map[string]any{}, &r)
	if r.Done != 0 || len(r.Errors) != 0 {
		t.Fatalf("second read-all: %+v", r)
	}
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Late", 1), time.Now().Add(-time.Hour))
	eventually(t, "late mail", func() bool { return len(list(t, env, "?unread=true")) == 1 })
	env.MustDo(http.MethodPost, "/mail/read-all", map[string]any{}, &r)
	if r.Done != 1 || len(list(t, env, "?unread=true")) != 0 {
		t.Fatalf("read-all without account: %+v", r)
	}
}

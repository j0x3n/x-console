package mail_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
}

func newFakeIMAP(t *testing.T) *fakeIMAP {
	t.Helper()
	h := httptest.NewTLSServer(http.NotFoundHandler()) // 借它的证书
	certs := h.TLS.Certificates
	pool := x509.NewCertPool()
	pool.AddCert(h.Certificate())
	h.Close()

	mem := imapmemserver.New()
	u := imapmemserver.NewUser(user, pass)
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIdle: {}},
		InsecureAuth: true,
		Logger:       quietLogger{},
	})
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certs})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return &fakeIMAP{addr: ln.Addr().String(), port: ln.Addr().(*net.TCPAddr).Port, user: u, tls: &tls.Config{RootCAs: pool}}
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

func setup(t *testing.T) (*testutil.Env, *mail.Module, *fakeIMAP) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*mail.Module](env.App.Deps.Registry, mail.ServiceKey)
	if !ok {
		t.Fatal("mail module not registered")
	}
	f := newFakeIMAP(t)
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
}

func hasFlag(flags []imap.Flag, f imap.Flag) bool {
	for _, x := range flags {
		if strings.EqualFold(string(x), string(f)) {
			return true
		}
	}
	return false
}

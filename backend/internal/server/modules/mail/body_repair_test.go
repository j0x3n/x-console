package mail_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

func TestInlineTextContentIDIsBody(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, `From: Sender <sender@example.com>
To: me@example.com
Subject: Inline HTML
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8
Content-ID: <body@example.com>

<html><body><p>Visible inline content</p></body></html>`, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "inline message cached", func() bool { return cachedBodies(t, env, acc.Id) == 1 })
	items := list(t, env, "")
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", items[0].Id), nil, &msg)
	if msg.Html == nil || !strings.Contains(msg.Text, "Visible inline content") || len(msg.Attachments) != 0 {
		t.Fatalf("inline body treated as attachment: %+v", msg)
	}
}

func TestManualSyncRepairsEmptyLegacyBodyAndSnippet(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "initial body cached", func() bool { return cachedBodies(t, env, acc.Id) == 1 })
	id := list(t, env, "")[0].Id
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", id), map[string]any{"unread": false, "flagged": true}, nil)
	if _, err := env.App.Deps.DB.Exec(`UPDATE mail_messages SET subject = '', from_address = '', snippet = '', body_json = '{"to":[],"cc":[],"text":"","attachments":[]}' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/mail/accounts/%d/sync", acc.Id), nil, nil)
	eventually(t, "metadata and body repaired", func() bool {
		var subject, snippet, body string
		if err := env.App.Deps.DB.QueryRow("SELECT subject, snippet, body_json FROM mail_messages WHERE id = ?", id).Scan(&subject, &snippet, &body); err != nil {
			return false
		}
		return subject == "Hello" && strings.Contains(snippet, "Hi there") && strings.Contains(body, "Hi there")
	})
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", id), nil, &msg)
	if msg.Unread || !msg.Flagged || !strings.Contains(msg.Text, "Hi there") || len(list(t, env, "")) != 1 {
		t.Fatalf("repair changed message state: %+v", msg)
	}
	if len(mailNotes(t, env)) != 0 {
		t.Fatal("repair sent historical mail notifications")
	}
}

func TestHistoricalBodyFailureDoesNotBlockLaterMessages(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Following message", 1), time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "initial bodies cached", func() bool { return cachedBodies(t, env, acc.Id) == 2 })
	f.failBodyUID.Store(1)
	if _, err := env.App.Deps.DB.Exec(`UPDATE mail_messages SET subject = '', from_address = '', snippet = '', body_json = '{"to":[],"cc":[],"text":"","attachments":[]}' WHERE account_id = ?`, acc.Id); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/mail/accounts/%d/sync", acc.Id), nil, nil)
	eventually(t, "later message restored despite first body failure", func() bool {
		var subject, body string
		return env.App.Deps.DB.QueryRow("SELECT subject, body_json FROM mail_messages WHERE account_id = ? AND uid = 2", acc.Id).Scan(&subject, &body) == nil && subject == "Following message" && strings.Contains(body, "Hi there")
	})
	if len(mailNotes(t, env)) != 0 {
		t.Fatal("historical recovery sent notifications")
	}
}

func TestIncompleteMIMEKeepsReadableBody(t *testing.T) {
	_, m, _ := setup(t)
	raw := "From: sender@example.com\r\nSubject: Partial\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=body\r\n\r\n--body\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Readable despite missing closing boundary</p>"
	text, hasHTML, err := mail.ParseBodyText(m, raw)
	if err != nil || !hasHTML || !strings.Contains(text, "Readable despite missing closing boundary") {
		t.Fatalf("usable body discarded: text=%q html=%v err=%v", text, hasHTML, err)
	}
	_, _, err = mail.ParseBodyText(m, "From: sender@example.com\r\nContent-Type: multipart/mixed; boundary=body\r\n\r\n--body")
	if err == nil {
		t.Fatal("incomplete empty body must remain an error")
	}
}

package mail_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

func TestRepairHistoricalSummariesAndCachedNames(t *testing.T) {
	env, _, f := setup(t)
	f.reorderHeaderFields.Store(true)
	f.failEnvelope.Store(true)
	f.add(t, htmlMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "reordered summary", func() bool { return bySubject(list(t, env, ""), "Report") != nil })
	id := bySubject(list(t, env, ""), "Report").Id
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", id), nil, &msg)
	if len(msg.Attachments) != 2 {
		t.Fatalf("fixture attachments: %+v", msg)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/messages/%d", id), map[string]any{"unread": false, "flagged": true}, nil)
	var cached string
	if err := env.App.Deps.DB.QueryRow("SELECT body_json FROM mail_messages WHERE id = ?", id).Scan(&cached); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(cached), &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"to", "cc"} {
		body[field].([]any)[0].(map[string]any)["name"] = "=?UTF-8?B?5bCP546L?="
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.App.Deps.DB.Exec("UPDATE mail_messages SET subject = '', from_name = '', from_address = '', message_id = '', body_json = ? WHERE id = ?", string(raw), id); err != nil {
		t.Fatal(err)
	}
	// Reconnect as happens on deployment. The last UID stays advanced.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/mail/accounts/%d", acc.Id), map[string]any{"password": pass}, nil)
	eventually(t, "legacy summary repaired without re-adding account", func() bool {
		items := list(t, env, "")
		return len(items) == 1 && items[0].Id == id && items[0].Subject == "Report" && items[0].From.Address == "bob@example.com"
	})
	beforeOpen := f.logins.Load()
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", id), nil, &msg)
	if msg.Subject != "Report" || msg.Unread || !msg.Flagged || len(msg.Attachments) != 2 || !strings.Contains(msg.Text, "Hello report") {
		t.Fatalf("repair changed message state or cache: %+v", msg)
	}
	for _, addresses := range [][]api.MailAddress{msg.To, msg.Cc} {
		if len(addresses) != 1 || addresses[0].Name == nil || *addresses[0].Name != "小王" {
			t.Fatalf("cached encoded name: %+v", addresses)
		}
	}
	if f.logins.Load() != beforeOpen {
		t.Fatal("cache repair opened an extra IMAP connection")
	}
	if err := env.App.Deps.DB.QueryRow("SELECT body_json FROM mail_messages WHERE id = ?", id).Scan(&cached); err != nil || strings.Contains(cached, "=?") {
		t.Fatalf("cache repair not saved: err=%v body=%s", err, cached)
	}
	if len(mailNotes(t, env)) != 0 {
		t.Fatal("historical repair sent a duplicate notification")
	}
	other := f.client(t)
	remote, err := other.Fetch(imap.UIDSetNum(1), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil || len(remote) != 1 {
		t.Fatalf("server message disappeared: %v", err)
	}
	seen, flagged := false, false
	for _, flag := range remote[0].Flags {
		seen = seen || flag == imap.FlagSeen
		flagged = flagged || flag == imap.FlagFlagged
	}
	if !seen || !flagged {
		t.Fatalf("server flags changed: %v", remote[0].Flags)
	}
	// Subsequent mail uses the same reordered section and still sends one push.
	f.add(t, strings.Replace(plainMail, "Subject: Hello", "Subject: Following repair", 1), time.Now())
	eventually(t, "new mail after repair", func() bool { return bySubject(list(t, env, ""), "Following repair") != nil })
	eventually(t, "one new mail notification", func() bool { return len(mailNotes(t, env)) == 1 })
}

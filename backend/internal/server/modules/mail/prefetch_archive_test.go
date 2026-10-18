package mail_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func cachedBodies(t *testing.T, env *testutil.Env, accountID int64) int {
	t.Helper()
	var n int
	if err := env.App.Deps.DB.QueryRow("SELECT COUNT(*) FROM mail_messages WHERE account_id = ? AND body_json IS NOT NULL", accountID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSyncPrefetchesBodiesWithoutReadingMail(t *testing.T) {
	env, m, f := setup(t)
	old := time.Now().Add(-time.Hour)
	f.add(t, plainMail, old)
	f.add(t, htmlMail, old)
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "cached bodies", func() bool { return cachedBodies(t, env, acc.Id) == 2 })
	if got := f.bodyFetches.Load(); got != 1 {
		t.Fatalf("body FETCH commands = %d, want 1 batch", got)
	}
	other := f.client(t)
	logins := f.logins.Load()
	for _, msg := range list(t, env, "") {
		var body api.MailMessage
		env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", msg.Id), nil, &body)
		if body.Text == "" || !body.Unread || hasFlag(serverFlags(t, other, imap.UID(msg.Id)), imap.FlagSeen) {
			t.Fatalf("prefetched mail: %+v", body)
		}
	}
	if f.logins.Load() != logins || mail.FileCount(m, fmt.Sprint(acc.Id)) != 2 {
		t.Fatal("opening cached mail connected again or lost attachments")
	}
	// IDLE 收到的新邮件同样预取，复用现有连接。
	f.add(t, gbkMail(), time.Now())
	eventually(t, "IDLE cached body", func() bool { return cachedBodies(t, env, acc.Id) == 3 })
	if f.logins.Load() != logins || hasFlag(serverFlags(t, other, 3), imap.FlagSeen) {
		t.Fatal("IDLE prefetch connected again or marked mail read")
	}
}

func TestPrefetchOnlyLatest50Within30Days(t *testing.T) {
	env, _, f := setup(t)
	now := time.Now()
	for i := range 55 {
		f.add(t, strings.Replace(plainMail, "Subject: Hello", fmt.Sprintf("Subject: Recent %02d", i), 1), now.Add(-time.Duration(i)*time.Hour))
	}
	// 旧邮件的 UID 更大，必须按日期选取。
	f.add(t, plainMail, now.Add(-31*24*time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "50 cached bodies", func() bool { return cachedBodies(t, env, acc.Id) == 50 })
	var unexpected int
	if err := env.App.Deps.DB.QueryRow("SELECT COUNT(*) FROM mail_messages WHERE account_id = ? AND uid > 50 AND body_json IS NOT NULL", acc.Id).Scan(&unexpected); err != nil || unexpected != 0 {
		t.Fatalf("older messages cached: %d, %v", unexpected, err)
	}
	// 再同步不能继续把第 51 封以后的正文取下来。
	connected(t, env, acc.Id)
	env.MustDo(http.MethodPost, fmt.Sprintf("/mail/accounts/%d/sync", acc.Id), nil, nil)
	connected(t, env, acc.Id)
	if n := cachedBodies(t, env, acc.Id); n != 50 {
		t.Fatalf("cached after second sync = %d", n)
	}
}

func TestPrefetchSkipsMailOlderThan30Days(t *testing.T) {
	env, _, f := setup(t)
	f.add(t, plainMail, time.Now().Add(-31*24*time.Hour))
	f.add(t, htmlMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "recent body cached", func() bool { return cachedBodies(t, env, acc.Id) == 1 })
	var oldCached bool
	if err := env.App.Deps.DB.QueryRow("SELECT body_json IS NOT NULL FROM mail_messages WHERE account_id = ? AND uid = 1", acc.Id).Scan(&oldCached); err != nil || oldCached {
		t.Fatalf("old body cached: %v, %v", oldCached, err)
	}
}

func TestPrefetchFailureKeepsSyncAndOpenFallback(t *testing.T) {
	env, _, f := setup(t)
	f.failBodies.Store(true)
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	eventually(t, "prefetch attempted", func() bool { return f.bodyFetches.Load() > 0 })
	connected(t, env, acc.Id)
	items := list(t, env, "")
	if len(items) != 1 || cachedBodies(t, env, acc.Id) != 0 {
		t.Fatal("failed prefetch affected sync")
	}
	f.failBodies.Store(false)
	var body api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", items[0].Id), nil, &body)
	if !strings.Contains(body.Text, "Hi there") || cachedBodies(t, env, acc.Id) != 1 {
		t.Fatalf("fallback body: %+v", body)
	}
}

func TestPrefetchAndConcurrentOpenKeepAttachments(t *testing.T) {
	env, m, f := setup(t)
	f.bodyStarted, f.bodyRelease = make(chan struct{}), make(chan struct{})
	defer close(f.bodyRelease)
	f.add(t, htmlMail, time.Now().Add(-time.Hour))
	var acc api.MailAccount
	env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
	select {
	case <-f.bodyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("prefetch not started")
	}
	items := list(t, env, "")
	path := fmt.Sprintf("/mail/messages/%d", items[0].Id)
	results := make(chan string, 8)
	for range 8 {
		go func() {
			var body api.MailMessage
			status, raw := env.Do(http.MethodGet, path, nil, &body)
			if status != http.StatusOK || len(body.Attachments) != 2 {
				results <- fmt.Sprintf("%d %s", status, raw)
				return
			}
			results <- ""
		}()
	}
	for range 8 {
		if result := <-results; result != "" {
			t.Fatal(result)
		}
	}
	// 打开时已缓存正文。预取随后完成时不能清掉这些附件。
	f.bodyRelease <- struct{}{}
	connected(t, env, acc.Id)
	if mail.FileCount(m, fmt.Sprint(acc.Id)) != 2 {
		t.Fatal("duplicate or missing attachment files")
	}
	for index := range 2 {
		env.MustDo(http.MethodGet, fmt.Sprintf("%s/attachments/%d", path, index), nil, nil)
	}
}

func TestBatchArchive(t *testing.T) {
	for _, tc := range []struct {
		name, folder  string
		special, move bool
		allFolder     string
		failCreate    bool
	}{
		{name: "special-use MOVE", folder: "Stored mail", special: true, move: true, allFolder: "[Gmail]/All Mail"},
		{name: "special-use COPY fallback", folder: "Stored mail", special: true},
		{name: "Chinese name", folder: "归档", allFolder: "[Gmail]/所有邮件"},
		{name: "case insensitive", folder: "aRcHiVeS"},
		{name: "Gmail All Mail", allFolder: "[Gmail]/All Mail", move: true},
		{name: "Gmail Chinese All Mail", allFolder: "[Gmail]/所有邮件"},
		{name: "create Archive"},
		{name: "create denied", failCreate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := []imap.Cap{}
			if tc.special || tc.allFolder != "" {
				caps = append(caps, imap.CapSpecialUse, imap.CapListExtended)
			}
			if tc.move {
				caps = append(caps, imap.CapMove)
			}
			env, m, f := setup(t, caps...)
			f.failCreate.Store(tc.failCreate)
			if tc.allFolder != "" {
				if err := f.user.Create(tc.allFolder, nil); err != nil {
					t.Fatal(err)
				}
				f.allFolder = tc.allFolder
			}
			if tc.folder != "" {
				if err := f.user.Create(tc.folder, nil); err != nil {
					t.Fatal(err)
				}
			}
			if tc.special {
				f.archiveFolder = tc.folder
				if err := f.user.Create("Archive", nil); err != nil {
					t.Fatal(err)
				}
			}
			f.add(t, plainMail, time.Now().Add(-time.Hour))
			f.add(t, htmlMail, time.Now().Add(-time.Hour))
			var acc api.MailAccount
			env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
			eventually(t, "cached bodies", func() bool { return cachedBodies(t, env, acc.Id) == 2 })
			items := list(t, env, "")
			ids := []int64{items[0].Id, items[1].Id}
			updates, cancel := env.App.Deps.Bus.Subscribe("mail.", 32)
			defer cancel()
			var result api.MailBatchResult
			status, raw := env.Do(http.MethodPost, "/mail/messages/batch", map[string]any{"ids": ids, "action": "archive"}, &result)
			other := f.client(t)
			inbox, err := other.Fetch(imap.UIDSetNum(1, 2), &imap.FetchOptions{UID: true, Flags: true}).Collect()
			if err != nil {
				t.Fatal(err)
			}
			if tc.failCreate {
				if status != http.StatusBadRequest || !strings.Contains(string(raw), "这个邮箱没有归档文件夹，也建不了") || len(inbox) != 2 || len(list(t, env, "")) != 2 || f.creates.Load() != 1 {
					t.Fatalf("missing archive: %d %s, inbox=%+v", status, raw, inbox)
				}
				for _, msg := range inbox {
					if hasFlag(msg.Flags, imap.FlagDeleted) {
						t.Fatal("mail marked deleted")
					}
				}
				return
			}
			if status != http.StatusOK || result.Done != 2 || len(result.Errors) != 0 || len(inbox) != 0 || len(list(t, env, "")) != 0 || mail.FileCount(m, fmt.Sprint(acc.Id)) != 0 {
				t.Fatalf("archive: %d %s, inbox=%+v", status, raw, inbox)
			}
			folder := tc.folder
			if folder == "" {
				folder = tc.allFolder
			}
			if folder == "" {
				folder = "Archive"
				if f.creates.Load() != 1 {
					t.Fatal("Archive was not created")
				}
			} else if f.creates.Load() != 0 {
				t.Fatal("created a folder despite an existing archive destination")
			}
			selected, err := other.Select(folder, nil).Wait()
			if err != nil || selected.NumMessages != 2 {
				t.Fatalf("archive folder: %+v %v", selected, err)
			}
			select {
			case <-updates:
			case <-time.After(time.Second):
				t.Fatal("missing mail event")
			}
		})
	}
}

func TestDeleteDoesNotUseAllMail(t *testing.T) {
	for _, trash := range []bool{false, true} {
		t.Run(fmt.Sprintf("trash=%v", trash), func(t *testing.T) {
			env, _, f := setup(t, imap.CapSpecialUse, imap.CapListExtended)
			f.allFolder = "[Gmail]/All Mail"
			if err := f.user.Create(f.allFolder, nil); err != nil {
				t.Fatal(err)
			}
			if trash {
				if err := f.user.Create("Trash", nil); err != nil {
					t.Fatal(err)
				}
			}
			f.add(t, plainMail, time.Now().Add(-time.Hour))
			var acc api.MailAccount
			env.MustDo(http.MethodPost, "/mail/accounts", accountInput(f, pass), &acc)
			eventually(t, "first sync", func() bool { return len(list(t, env, "")) == 1 })
			if result := batch(t, env, "delete", list(t, env, "")[0].Id); result.Done != 1 || len(result.Errors) != 0 {
				t.Fatalf("delete: %+v", result)
			}
			other := f.client(t)
			selected, err := other.Select(f.allFolder, nil).Wait()
			if err != nil || selected.NumMessages != 0 || f.creates.Load() != 0 {
				t.Fatalf("delete used All Mail or created Archive: %+v %v", selected, err)
			}
			if trash {
				selected, err := other.Select("Trash", nil).Wait()
				if err != nil || selected.NumMessages != 1 {
					t.Fatalf("Trash: %+v %v", selected, err)
				}
			}
		})
	}
}

func TestArchiveAlreadyInAllMailDoesNotMove(t *testing.T) {
	f := newFakeIMAP(t, imap.CapSpecialUse, imap.CapListExtended)
	f.allFolder = "[Gmail]/All Mail"
	if err := f.user.Create(f.allFolder, nil); err != nil {
		t.Fatal(err)
	}
	f.add(t, plainMail, time.Now().Add(-time.Hour))
	c := f.client(t)
	if _, err := c.Copy(imap.UIDSetNum(1), f.allFolder).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Select(f.allFolder, nil).Wait(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "All Mail selected", func() bool {
		selected := c.Mailbox()
		return selected != nil && selected.Name == f.allFolder
	})
	if err := mail.ArchiveOnServer(c, imap.UIDSetNum(1)); err != nil {
		t.Fatal(err)
	}
	msgs, err := c.Fetch(imap.UIDSetNum(1), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil || len(msgs) != 1 || hasFlag(msgs[0].Flags, imap.FlagDeleted) || f.creates.Load() != 0 {
		t.Fatalf("already archived message changed: %+v %v", msgs, err)
	}
}

func TestArchiveCopyFailureKeepsInboxMail(t *testing.T) {
	env, _, f, _ := batchSetup(t)
	if err := f.user.Create("Archive", nil); err != nil {
		t.Fatal(err)
	}
	f.failCopy.Store(true)
	ids := idsOf(t, list(t, env, ""), "One", "Two")
	result := batch(t, env, "archive", ids...)
	if result.Done != 0 || len(result.Errors) != 1 || len(list(t, env, "")) != 4 {
		t.Fatalf("failed copy: %+v", result)
	}
	other := f.client(t)
	msgs, err := other.Fetch(imap.UIDSetNum(1, 2), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil || len(msgs) != 2 {
		t.Fatalf("inbox after failed copy: %+v %v", msgs, err)
	}
	for _, msg := range msgs {
		if hasFlag(msg.Flags, imap.FlagDeleted) {
			t.Fatal("failed copy marked mail deleted")
		}
	}
}

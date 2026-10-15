package mail_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func todo(t *testing.T, env *testutil.Env) []api.MailSummary {
	t.Helper()
	var page api.MailPage
	env.MustDo(http.MethodGet, "/mail/todo", nil, &page)
	return page.Items
}

func setDone(env *testutil.Env, id int64, done bool) int {
	status, _ := env.Do(http.MethodPut, fmt.Sprintf("/mail/messages/%d/done", id), map[string]any{"done": done}, nil)
	return status
}

// addMails adds mails after the first sync, which makes them new.
func addMails(t *testing.T, f *fakeIMAP, subjects map[string]time.Time) {
	t.Helper()
	for s, d := range subjects {
		f.add(t, subjectMail(s), d)
		time.Sleep(20 * time.Millisecond)
	}
}

// syncNow asks every account to sync, in case the fake server's news about a
// new message was missed.
func syncNow(env *testutil.Env) {
	var accounts []api.MailAccount
	env.MustDo(http.MethodGet, "/mail/accounts", nil, &accounts)
	for _, a := range accounts {
		env.Do(http.MethodPost, fmt.Sprintf("/mail/accounts/%d/sync", a.Id), nil, nil)
	}
}

func waitSorted(t *testing.T, env *testutil.Env, subjects ...string) {
	t.Helper()
	eventually(t, "sorted", func() bool {
		syncNow(env)
		var page api.MailPage
		env.MustDo(http.MethodGet, "/mail/messages", nil, &page)
		for _, s := range subjects {
			if m := bySubject(page.Items, s); m == nil || m.Triage == nil {
				return false
			}
		}
		return true
	})
}

func TestTriageHasDecisionOnlyForAction(t *testing.T) {
	env, _, f := setup(t)
	ai := withAI(t, env)
	firstSync(t, env, f, true)
	ai.override = func(index int, subject string) map[string]any {
		// The AI says decision for every mail, also for notice and promo.
		cat := "notice"
		switch {
		case strings.Contains(subject, "批准"), strings.Contains(subject, "合同"):
			cat = "action"
		case strings.Contains(subject, "促销"):
			cat = "promo"
		}
		return map[string]any{"index": index, "category": cat, "decision": true, "summary": "x", "dates": []any{}}
	}
	now := time.Now()
	addMails(t, f, map[string]time.Time{"请批准报销": now, "合同请签字": now, "双十一促销": now, "账单已出": now})
	waitSorted(t, env, "请批准报销", "合同请签字", "双十一促销", "账单已出")
	items := list(t, env, "")
	want := map[string]bool{"请批准报销": true, "合同请签字": true, "双十一促销": false, "账单已出": false}
	for subject, decision := range want {
		tr := bySubject(items, subject).Triage
		if tr.Decision != decision || tr.DoneAt != nil {
			t.Errorf("%s: %+v", subject, tr)
		}
	}
	if !strings.Contains(ai.systems[0], "decision") {
		t.Errorf("the prompt does not explain decision: %q", ai.systems[0])
	}
	// Sorting on request gives the same result.
	var tr api.MailTriage
	env.MustDo(http.MethodPost, fmt.Sprintf("/mail/messages/%d/triage", bySubject(items, "账单已出").Id), nil, &tr)
	if tr.Category != api.Notice || tr.Decision {
		t.Errorf("on request: %+v", tr)
	}
}

func TestTriageDefaultAIDecision(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true)
	now := time.Now()
	addMails(t, f, map[string]time.Time{"请批准报销": now, "合同请签字": now})
	waitSorted(t, env, "请批准报销", "合同请签字")
	items := list(t, env, "")
	if tr := bySubject(items, "请批准报销").Triage; tr.Category != api.Action || !tr.Decision {
		t.Errorf("approve: %+v", tr)
	}
	if tr := bySubject(items, "合同请签字").Triage; tr.Category != api.Action || tr.Decision {
		t.Errorf("sign: %+v", tr)
	}
}

func TestMailTodoListsTodaysOpenActionMail(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true)
	now := time.Now()
	addMails(t, f, map[string]time.Time{
		"合同请签字": now.Add(-2 * time.Minute),
		"请批准报销": now.Add(-time.Minute),
		"双十一促销": now,
		"账单已出":  now,
	})
	waitSorted(t, env, "合同请签字", "请批准报销", "双十一促销", "账单已出")
	// Automatic triage only looks at mail from the last minutes, so older
	// mail is sorted on request.
	addMails(t, f, map[string]time.Time{
		"旧合同请签字": now.AddDate(0, 0, -40),
		"昨天的合同":  now.AddDate(0, 0, -1),
	})
	eventually(t, "old mail", func() bool {
		syncNow(env)
		items := list(t, env, "")
		return bySubject(items, "旧合同请签字") != nil && bySubject(items, "昨天的合同") != nil
	})
	for _, subject := range []string{"旧合同请签字", "昨天的合同"} {
		old := bySubject(list(t, env, ""), subject)
		env.MustDo(http.MethodPost, fmt.Sprintf("/mail/messages/%d/triage", old.Id), nil, nil)
		if x := bySubject(list(t, env, ""), subject); x.Triage == nil || x.Triage.Category != api.Action {
			t.Fatalf("%s: %+v", subject, x)
		}
	}
	got := todo(t, env)
	if len(got) != 2 || got[0].Subject != "请批准报销" || got[1].Subject != "合同请签字" {
		t.Fatalf("todo: %+v", got)
	}
	for _, x := range got {
		if x.Triage == nil || x.Triage.Category != api.Action {
			t.Errorf("no triage: %+v", x)
		}
	}
	if !got[0].Triage.Decision || got[1].Triage.Decision {
		t.Errorf("decision: %+v / %+v", got[0].Triage, got[1].Triage)
	}
	// Mark done: it goes away. Undo: it comes back.
	id := got[0].Id
	if status := setDone(env, id, true); status != http.StatusNoContent {
		t.Fatalf("done: %d", status)
	}
	if got := todo(t, env); len(got) != 1 || got[0].Subject != "合同请签字" {
		t.Errorf("after done: %+v", got)
	}
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", id), nil, &msg)
	if msg.Triage == nil || msg.Triage.DoneAt == nil || time.Since(*msg.Triage.DoneAt) > time.Minute {
		t.Errorf("doneAt: %+v", msg.Triage)
	}
	if msg.Unread {
		t.Error("marking handled left the message unread")
	}
	syncNow(env)
	eventually(t, "stays read", func() bool {
		x := bySubject(list(t, env, ""), "请批准报销")
		return x != nil && !x.Unread
	})
	if status := setDone(env, id, false); status != http.StatusNoContent {
		t.Fatalf("undo: %d", status)
	}
	if got := todo(t, env); len(got) != 2 || got[0].Id != id || got[0].Triage.DoneAt != nil {
		t.Errorf("after undo: %+v", got)
	}
	if bySubject(list(t, env, ""), "请批准报销").Unread {
		t.Error("undoing handled marked the message unread again")
	}
}

func TestMailTodoIsCappedAtTwenty(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true)
	now := time.Now()
	subjects := map[string]time.Time{}
	for i := 0; i < 22; i++ {
		subjects[fmt.Sprintf("合同%02d", i)] = now.Add(-time.Duration(i) * 10 * time.Second)
	}
	addMails(t, f, subjects)
	var all []string
	for s := range subjects {
		all = append(all, s)
	}
	waitSorted(t, env, all...)
	got := todo(t, env)
	if len(got) != 20 || got[0].Subject != "合同00" || got[19].Subject != "合同19" {
		t.Errorf("todo: %d items, first %q", len(got), got[0].Subject)
	}
}

func TestSortingAgainKeepsDone(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true)
	addMails(t, f, map[string]time.Time{"合同请签字": time.Now()})
	waitSorted(t, env, "合同请签字")
	id := bySubject(list(t, env, ""), "合同请签字").Id
	if status := setDone(env, id, true); status != http.StatusNoContent {
		t.Fatalf("done: %d", status)
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/mail/messages/%d/triage", id), nil, nil)
	var msg api.MailMessage
	env.MustDo(http.MethodGet, fmt.Sprintf("/mail/messages/%d", id), nil, &msg)
	if msg.Triage == nil || msg.Triage.DoneAt == nil {
		t.Errorf("done_at was cleared: %+v", msg.Triage)
	}
	if got := todo(t, env); len(got) != 0 {
		t.Errorf("todo after sorting again: %+v", got)
	}
}

func TestSetMailDoneNeedsASortedMessage(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true) // the first mail is not sorted
	id := list(t, env, "")[0].Id
	if status := setDone(env, id, true); status != http.StatusNotFound {
		t.Errorf("unsorted message: %d", status)
	}
	if status := setDone(env, 9999, true); status != http.StatusNotFound {
		t.Errorf("unknown message: %d", status)
	}
}

func TestMailSummaryLatestHasTriage(t *testing.T) {
	env, _, f := setup(t)
	withAI(t, env)
	firstSync(t, env, f, true)
	addMails(t, f, map[string]time.Time{"请批准报销": time.Now()})
	waitSorted(t, env, "请批准报销")
	var sum api.MailSummaryView
	env.MustDo(http.MethodGet, "/mail/summary", nil, &sum)
	var sorted, plain int
	for _, x := range sum.Latest {
		switch x.Subject {
		case "请批准报销":
			if x.Triage == nil || x.Triage.Category != api.Action || !x.Triage.Decision {
				t.Errorf("sorted mail: %+v", x.Triage)
			}
			sorted++
		case "Hello":
			if x.Triage != nil {
				t.Errorf("unsorted mail has triage: %+v", x.Triage)
			}
			plain++
		}
	}
	if sorted != 1 || plain != 1 {
		t.Errorf("latest: %+v", sum.Latest)
	}
}

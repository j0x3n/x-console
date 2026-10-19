package mail_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

// sortingAI sorts by the subject: 促销 is promo, 合同 needs me, everything else
// is a notice. A notice is important when its subject says 异常. The AI says
// "important" for promo and action mail too, which must not stick.
func sortingAI(idx int, subject string) map[string]any {
	item := map[string]any{"index": idx, "category": "notice", "summary": subject, "dates": []any{}, "important": true}
	switch {
	case strings.Contains(subject, "促销"):
		item["category"] = "promo"
	case strings.Contains(subject, "合同"):
		item["category"] = "action"
	default:
		item["important"] = strings.Contains(subject, "异常")
	}
	return item
}

func TestMailSortingSettingsDefaultOffAndSave(t *testing.T) {
	env, _, _ := setup(t)
	var got api.MailSorting
	env.MustDo(http.MethodGet, "/mail/sorting", nil, &got)
	if got.MarkPromoRead || got.ImportantOnly {
		t.Fatalf("defaults: %+v", got)
	}
	env.MustDo(http.MethodPut, "/mail/sorting", map[string]any{"markPromoRead": true, "importantOnly": false}, &got)
	if !got.MarkPromoRead || got.ImportantOnly {
		t.Fatalf("saved: %+v", got)
	}
	got = api.MailSorting{}
	env.MustDo(http.MethodGet, "/mail/sorting", nil, &got)
	if !got.MarkPromoRead || got.ImportantOnly {
		t.Fatalf("read back: %+v", got)
	}
}

func TestImportantOnlyPushesImportantNoticesAndActionMail(t *testing.T) {
	env, _, f := setup(t)
	ai := withAI(t, env)
	ai.override = sortingAI
	capture := &captureRouter{}
	env.App.Deps.Notify.SetRouter(capture)
	firstSync(t, env, f, true)
	env.MustDo(http.MethodPut, "/mail/sorting", map[string]any{"markPromoRead": false, "importantOnly": true}, nil)
	subjects := []string{"账单已出", "账号异常登录", "合同请签字", "双十一促销来了"}
	for _, s := range subjects {
		f.add(t, subjectMail(s), time.Now())
		time.Sleep(20 * time.Millisecond)
	}
	eventually(t, "sorted and pushed", func() bool {
		items := list(t, env, "")
		for _, s := range subjects {
			if m := bySubject(items, s); m == nil || m.Triage == nil {
				return false
			}
		}
		return len(capture.mail()) == 2
	})
	time.Sleep(200 * time.Millisecond)
	if got := len(capture.mail()); got != 2 {
		t.Fatalf("notifications: %d", got)
	}
	var important, action bool
	for _, n := range capture.mail() {
		switch {
		case strings.Contains(n.Title, "账号异常登录"):
			important = n.Immediate && strings.HasPrefix(n.Title, "（重要）")
		case strings.Contains(n.Title, "合同请签字"):
			action = n.Immediate && strings.HasPrefix(n.Title, "（要处理）")
		default:
			t.Errorf("unexpected push: %+v", n)
		}
	}
	if !important || !action {
		t.Fatalf("important=%v action=%v: %+v", important, action, capture.mail())
	}
	items := list(t, env, "")
	if tr := bySubject(items, "账号异常登录").Triage; !tr.Important {
		t.Errorf("important notice: %+v", tr)
	}
	for _, s := range []string{"账单已出", "合同请签字", "双十一促销来了"} {
		if tr := bySubject(items, s).Triage; tr.Important {
			t.Errorf("%s should not be important: %+v", s, tr)
		}
	}
}

func TestNoticesAreAllPushedWhenImportantOnlyIsOff(t *testing.T) {
	env, _, f := setup(t)
	ai := withAI(t, env)
	ai.override = sortingAI
	capture := &captureRouter{}
	env.App.Deps.Notify.SetRouter(capture)
	firstSync(t, env, f, true)
	f.add(t, subjectMail("账单已出"), time.Now())
	eventually(t, "unimportant notice pushed", func() bool { return len(capture.mail()) == 1 })
	if n := capture.mail()[0]; n.Immediate || strings.Contains(n.Title, "（重要）") {
		t.Fatalf("push: %+v", n)
	}
}

func TestMarkPromoReadOnArrival(t *testing.T) {
	env, _, f := setup(t)
	ai := withAI(t, env)
	ai.override = sortingAI
	firstSync(t, env, f, true)
	env.MustDo(http.MethodPut, "/mail/sorting", map[string]any{"markPromoRead": true, "importantOnly": false}, nil)
	f.add(t, subjectMail("双十一促销来了"), time.Now()) // uid 2
	time.Sleep(20 * time.Millisecond)
	f.add(t, subjectMail("账单已出"), time.Now()) // uid 3
	eventually(t, "promo read", func() bool {
		items := list(t, env, "")
		promo, notice := bySubject(items, "双十一促销来了"), bySubject(items, "账单已出")
		return promo != nil && notice != nil && promo.Triage != nil && notice.Triage != nil && !promo.Unread
	})
	other := f.client(t)
	if !hasFlag(serverFlags(t, other, 2), imap.FlagSeen) {
		t.Error("the server did not get \\Seen for the promo mail")
	}
	if hasFlag(serverFlags(t, other, 3), imap.FlagSeen) || !bySubject(list(t, env, ""), "账单已出").Unread {
		t.Error("the notice should stay unread")
	}
}

func TestPromoStaysUnreadWhenMarkPromoReadIsOff(t *testing.T) {
	env, _, f := setup(t)
	ai := withAI(t, env)
	ai.override = sortingAI
	firstSync(t, env, f, true)
	f.add(t, subjectMail("双十一促销来了"), time.Now())
	eventually(t, "promo sorted", func() bool {
		m := bySubject(list(t, env, ""), "双十一促销来了")
		return m != nil && m.Triage != nil
	})
	if !bySubject(list(t, env, ""), "双十一促销来了").Unread || hasFlag(serverFlags(t, f.client(t), 2), imap.FlagSeen) {
		t.Fatal("promo was marked read with the setting off")
	}
}

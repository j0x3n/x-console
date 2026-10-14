package watchers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
)

// Step is one recorded action. Optional steps are skipped when their element
// is missing, for example the login form when the saved login still works.
type Step struct {
	Action   string           `json:"action"` // goto, click, type, fill, select, press, wait, ask
	URL      string           `json:"url,omitempty"`
	Loc      *browser.Locator `json:"loc,omitempty"`
	Text     string           `json:"text,omitempty"`  // type: text, select: option, press: key
	Field    string           `json:"field,omitempty"` // fill: username or password
	Seconds  int              `json:"seconds,omitempty"`
	Optional bool             `json:"optional,omitempty"`
	Note     string           `json:"note"`
}

// Recipe is what the AI recorded: the steps to the value and where it is.
type Recipe struct {
	Steps      []Step          `json:"steps"`
	Target     browser.Locator `json:"target"`
	TargetNote string          `json:"targetNote"`
}

func parseRecipe(raw string) (Recipe, bool) {
	var r Recipe
	if raw == "" || json.Unmarshal([]byte(raw), &r) != nil || len(r.Steps) == 0 {
		return r, false
	}
	return r, true
}

// replay follows the recorded steps and reads the value. It does not call AI.
func (m *Module) replay(ctx context.Context, rs *runState, w db.Watcher, sess *browser.Session, rec *recorder, r Recipe) result {
	login := m.login(w)
	_, words := m.options(ctx)
	skipped, filled := false, false
	for i, st := range r.Steps {
		if ctx.Err() != nil {
			return result{err: ctx.Err(), noRetry: true}
		}
		if st.Action == "press" && skipped {
			continue // the Enter that went with a skipped field
		}
		skipped = false
		var ref string
		if st.Loc != nil {
			var err error
			ref, err = sess.Locate(ctx, *st.Loc)
			if errors.Is(err, browser.ErrNoElement) && st.Optional {
				skipped = true
				continue
			}
			if err != nil {
				rec.shot(ctx, sess)
				return result{err: fmt.Errorf("第 %d 步（%s）找不到要操作的地方", i+1, st.Note)}
			}
		}
		var err error
		switch st.Action {
		case "goto":
			rec.add(st.Note)
			err = sess.Navigate(ctx, st.URL)
		case "click":
			rec.add(st.Note)
			if word := dangerText(ctx, sess, ref, words); word != "" {
				rec.shot(ctx, sess)
				return result{err: fmt.Errorf("第 %d 步要点的“%s”像是会改数据的按钮，没有点", i+1, word)}
			}
			err = sess.Click(ctx, ref)
		case "type":
			rec.add(st.Note)
			err = sess.Type(ctx, ref, st.Text)
		case "fill":
			if login == nil {
				rec.shot(ctx, sess)
				return result{err: errNeedLogin, noRetry: true, login: true}
			}
			value := login.Username
			if st.Field == "password" {
				value = login.Password
			}
			rec.add(st.Note)
			err = sess.Fill(ctx, ref, value)
			filled = true
		case "select":
			rec.add(st.Note)
			err = sess.Select(ctx, ref, st.Text)
		case "press":
			err = sess.Press(ctx, st.Text)
		case "wait":
			err = sess.Wait(ctx, time.Duration(st.Seconds)*time.Second)
		case "ask":
			c, cerr := sess.Check(ctx)
			if cerr != nil || c.Captcha == "" {
				continue
			}
			if res, done := m.askCaptcha(ctx, rs, w, sess, rec, c.Captcha); done {
				return res
			}
		}
		if err != nil {
			rec.shot(ctx, sess)
			return result{err: fmt.Errorf("第 %d 步（%s）出错：%v", i+1, st.Note, err)}
		}
	}
	// Still on a login page: the password is wrong, or a captcha appeared
	// that the AI did not see when it explored.
	for tries := 0; tries < 2; tries++ {
		c, err := sess.Check(ctx)
		if err != nil || !c.Password {
			break
		}
		kind := c.Captcha
		if kind == "" {
			if filled {
				rec.shot(ctx, sess)
				return result{err: errors.New("登录没成功，看看用户名和密码对不对"), login: true, noRetry: true}
			}
			// The steps have no login (the user logged in by hand when the
			// AI explored), so the user has to log in again.
			kind = "登录"
		}
		if res, done := m.askCaptcha(ctx, rs, w, sess, rec, kind); done {
			return res
		}
	}
	ref, err := sess.Locate(ctx, r.Target)
	if err != nil {
		rec.shot(ctx, sess)
		return result{err: errors.New("找不到要盯的内容，页面可能改版了")}
	}
	value, err := sess.Text(ctx, ref)
	if err != nil {
		return result{err: err}
	}
	if value == "" {
		rec.shot(ctx, sess)
		return result{err: errors.New("要盯的地方是空的，页面可能改版了")}
	}
	rec.add("读到：" + clip(value, 200))
	res := result{value: value, found: true}
	if value != w.CurrentValue && w.CurrentValue != "" {
		res.shot = rec.shot(ctx, sess)
	}
	return res
}

// askCaptcha hands a captcha to the user. done is true when the run must end
// with res.
func (m *Module) askCaptcha(ctx context.Context, rs *runState, w db.Watcher, sess *browser.Session, rec *recorder, kind string) (res result, done bool) {
	msg := "网站要" + kind + "，打开接管页面处理，弄好了点“我弄好了”。"
	switch m.ask(ctx, rs, w, sess, rec, "captcha", msg, rs.kind != "manual") {
	case "done":
		_ = sess.Wait(ctx, time.Second)
		return result{}, false
	case "timeout":
		return result{err: errNoAnswer, login: true, noRetry: true}, true
	case "limit":
		return result{err: errAskLimit, login: true, noRetry: true}, true
	default:
		return result{err: errCancelled, noRetry: true}, true
	}
}

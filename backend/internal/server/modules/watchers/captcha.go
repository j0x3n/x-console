package watchers

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
)

// B214: a character captcha on a login page is read by the page model and
// typed into its field. A wrong read gets a new image, at most
// maxCaptchaReads reads in all; then the user takes over. SMS codes, QR
// codes, sliders and picture puzzles always go to the user.

// maxCaptchaReads is the first read and 2 retries (the user asked for 2).
const maxCaptchaReads = 3

const captchaSystem = `你在读网站登录页上的图形验证码。图片里是几个字母或数字。
只回答图片里的字符，按顺序写，不要空格，不要别的字。看不清就回答“看不清”。`

// captchaStepNote is how the captcha step reads in the recorded steps.
const captchaStepNote = "有图形验证码时 AI 读了填上，读不出来交给你"

// passwordWrong matches the words a site uses when the user name or
// password is wrong, so a wrong password is not taken for a wrong captcha.
var passwordWrong = regexp.MustCompile(`(密码|口令)(错误|不正确|有误|不对)|(用户名|账号|帐号)或密码(错误|不正确|有误|不对)`)

// solveCaptcha reads the captcha on the page and types the answer in.
// refresh clicks the image first for a new one. It says why when it could
// not; the caller then hands the captcha to the user.
func (m *Module) solveCaptcha(ctx context.Context, w db.Watcher, sess *browser.Session, rec *recorder, refresh bool) error {
	ctx = m.agentCtx(ctx)
	chat := m.chat()
	if chat == nil || !chat.Available(ctx, "agent") {
		return errNoAI
	}
	if m.overBudget(ctx) {
		return errors.New("今天的 AI 花费到上限了")
	}
	parts, ok, err := sess.Captcha(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("不是字母或数字的图形验证码，或者找不到填验证码的地方")
	}
	if refresh {
		// Most sites show a new image when it is clicked.
		if err := sess.Click(ctx, parts.Image); err != nil {
			return err
		}
		_ = sess.Wait(ctx, time.Second)
		if parts, ok, err = sess.Captcha(ctx); err != nil || !ok {
			return errors.New("换了验证码以后找不到它")
		}
	}
	img, err := sess.ElementShot(ctx, parts.Image)
	if err != nil {
		return err
	}
	// The image goes only to the chosen model. It is not stored.
	ctx = contracts.WithAIUsage(ctx, "watchers", strconv.FormatInt(w.ID, 10))
	reply, err := chat.Chat(ctx, "agent", captchaSystem, []contracts.ChatMessage{{Role: "user", Content: "这是验证码图片。",
		Images: []contracts.ChatImage{{MIME: "image/png", Data: img}}}}, nil)
	if err != nil {
		return err
	}
	code := captchaCode(reply.Text)
	if code == "" {
		rec.add("AI 没看清验证码")
		return errors.New("AI 没看清验证码")
	}
	if err := sess.Type(ctx, parts.Input, code); err != nil {
		return err
	}
	rec.add("AI 读了图形验证码，填好了")
	return nil
}

// captchaCode keeps the letters and digits of the model's answer. An answer
// too short or too long is not a captcha.
func captchaCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	if b.Len() < 3 || b.Len() > 8 {
		return ""
	}
	return b.String()
}

// captchaWrong reports whether the page is still a login page with a
// character captcha and no word about a wrong password, so the captcha was
// likely read wrong.
func captchaWrong(ctx context.Context, sess *browser.Session) bool {
	c, err := sess.Check(ctx)
	if err != nil || !c.Password || c.Captcha != "图形验证码" {
		return false
	}
	p, err := sess.Snapshot(ctx, snapshotChars)
	return err == nil && !passwordWrong.MatchString(p.Text)
}

package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	firstSyncLimit = 200 // 第一次同步只取最近这么多封
	flagSyncLimit  = 500 // 每次同步对最近这么多封核对已读和星标
	snippetBytes   = 2048
	notifyWindow   = 10 * time.Minute // 只推这么久以内收到的
	notifyGroup    = 3                // 一次来了超过这么多封就合成一条
)

// accountState is the live connection state shown in GET /mail/accounts.
type accountState struct {
	Status    api.MailAccountStatus
	LastError string
	Idle      bool
}

type worker struct {
	id     int64
	poke   chan struct{} // sync now: new mail, flag changes, or the sync button
	cancel context.CancelFunc
	done   chan struct{}

	mu   sync.Mutex
	live bool   // B217: 连着，能收请求
	jobs []*job // 等这条连接去改邮箱的请求
}

func (w *worker) wake() {
	select {
	case w.poke <- struct{}{}:
	default:
	}
}

func (m *Module) state(id int64) accountState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.states[id]; ok {
		return s
	}
	return accountState{Status: api.Connecting}
}

func (m *Module) setState(id int64, s accountState) {
	m.mu.Lock()
	old, had := m.states[id]
	if _, running := m.workers[id]; !running {
		m.mu.Unlock()
		return
	}
	m.states[id] = s
	m.mu.Unlock()
	if !had || old != s {
		m.d.Bus.Publish("mail.updated", map[string]any{"accountId": id})
	}
}

// startWorker (re)starts the goroutine of one account. Before Start has run
// it does nothing; Start starts every account.
func (m *Module) startWorker(id int64) {
	m.mu.Lock()
	base := m.ctx
	if base == nil {
		m.mu.Unlock()
		return
	}
	old := m.workers[id]
	if old != nil {
		old.cancel()
	}
	ctx, cancel := context.WithCancel(base)
	w := &worker{id: id, poke: make(chan struct{}, 1), cancel: cancel, done: make(chan struct{})}
	m.workers[id] = w
	m.states[id] = accountState{Status: api.Connecting}
	m.mu.Unlock()
	go func() {
		if old != nil {
			<-old.done
		}
		m.run(ctx, w)
	}()
}

// stopWorker stops the goroutine of one account and waits for it.
func (m *Module) stopWorker(id int64) {
	m.mu.Lock()
	w := m.workers[id]
	delete(m.workers, id)
	delete(m.states, id)
	m.mu.Unlock()
	if w != nil {
		w.cancel()
		<-w.done
	}
}

// pokeWorker asks the account's goroutine to sync now.
func (m *Module) pokeWorker(id int64) {
	m.mu.Lock()
	w := m.workers[id]
	m.mu.Unlock()
	if w == nil {
		m.startWorker(id)
		return
	}
	w.wake()
}

// run keeps one account connected until ctx ends, reconnecting with growing
// pauses. A wrong password is not retried until the account changes or the
// user presses sync.
func (m *Module) run(ctx context.Context, w *worker) {
	defer close(w.done)
	attempt := 0
	for {
		connected, err := m.session(ctx, w)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			return // 账号已经删了
		}
		if connected {
			attempt = 0
		}
		m.d.Log.Warn("mail: connection lost", "account", w.id, "err", err)
		m.setState(w.id, accountState{Status: api.Error, LastError: err.Error()})
		var wait <-chan time.Time
		if !isAuthError(err) {
			wait = time.After(m.retry[min(attempt, len(m.retry)-1)])
			attempt++
		}
		select {
		case <-ctx.Done():
			return
		case <-wait:
		case <-w.poke:
		}
	}
}

// session is one connection: sync, then wait for news and sync again.
func (m *Module) session(ctx context.Context, w *worker) (connected bool, err error) {
	acc, err := m.q.GetAccount(ctx, w.id)
	if err != nil {
		return false, err
	}
	cfg, err := m.accountConfig(acc)
	if err != nil {
		return false, err
	}
	handler := &imapclient.UnilateralDataHandler{
		Mailbox: func(d *imapclient.UnilateralDataMailbox) {
			if d.NumMessages != nil {
				w.wake()
			}
		},
		Fetch: func(msg *imapclient.FetchMessageData) {
			_, _ = msg.Collect()
			w.wake()
		},
		Expunge: func(uint32) { w.wake() },
	}
	c, sel, err := m.connect(ctx, cfg, handler)
	if err != nil {
		return false, err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	defer w.failJobs() // 还在等的请求改走短连接

	caps := c.Caps()
	idle := caps.Has(imap.CapIdle) || caps.Has(imap.CapIMAP4rev2)
	w.setLive(true)
	m.setState(w.id, accountState{Status: api.Ok, Idle: idle})

	if err := m.checkValidity(ctx, acc, sel.UIDValidity); err != nil {
		return true, err
	}
	for {
		m.runJobs(ctx, c, w)
		if err := m.syncOnce(ctx, c, w.id, sel.NumMessages); err != nil {
			return true, fmt.Errorf("同步失败: %w", err)
		}
		if idle {
			if err := m.waitIdle(ctx, c, w); err != nil {
				return true, err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-c.Closed():
			return true, errors.New("连接断开了")
		case <-w.poke:
		case <-time.After(m.poll):
		}
	}
}

// waitIdle sits in IDLE until the server reports news, someone asks for a
// sync, or it is time to renew the IDLE.
func (m *Module) waitIdle(ctx context.Context, c *imapclient.Client, w *worker) error {
	cmd, err := c.Idle()
	if err != nil {
		return fmt.Errorf("IDLE 失败: %w", err)
	}
	timer := time.NewTimer(m.idleRestart)
	defer timer.Stop()
	closed := false
	select {
	case <-ctx.Done():
	case <-c.Closed():
		closed = true
	case <-w.poke:
	case <-timer.C:
	}
	if err := cmd.Close(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	if closed {
		return errors.New("连接断开了")
	}
	return ctx.Err()
}

// checkValidity drops the local copy when the inbox's UIDVALIDITY changed:
// the old UIDs no longer name the same messages.
func (m *Module) checkValidity(ctx context.Context, acc db.MailAccount, validity uint32) error {
	if acc.UidValidity == int64(validity) {
		return nil
	}
	if acc.UidValidity != 0 {
		m.d.Log.Info("mail: UIDVALIDITY changed, syncing again", "account", acc.ID)
		if err := m.q.DeleteAccountMessages(ctx, acc.ID); err != nil {
			return err
		}
		m.deleteFiles(ctx, fmt.Sprint(acc.ID))
	}
	return m.q.SetSyncState(ctx, db.SetSyncStateParams{UidValidity: int64(validity), LastUid: 0, LastSyncAt: acc.LastSyncAt, ID: acc.ID})
}

// syncOnce fetches new messages, then checks flags of the recent ones.
// selected is the message count the SELECT reported. go-imap sets
// c.Mailbox() a moment after Select().Wait() returns, so right after
// connecting it can still be nil; without the fallback the first sync would
// see an empty inbox and wait in IDLE for mail that is already there.
func (m *Module) syncOnce(ctx context.Context, c *imapclient.Client, id int64, selected uint32) error {
	acc, err := m.q.GetAccount(ctx, id)
	if err != nil {
		return err
	}
	var set imap.NumSet
	if acc.LastUid == 0 {
		n := selected
		if mb := c.Mailbox(); mb != nil {
			n = mb.NumMessages
		}
		if n > 0 {
			start := uint32(1)
			if n > firstSyncLimit {
				start = n - firstSyncLimit + 1
			}
			var s imap.SeqSet
			s.AddRange(start, n)
			set = s
		}
	} else {
		var s imap.UIDSet
		s.AddRange(imap.UID(acc.LastUid+1), 0) // 0 是 *
		set = s
	}

	var fresh []db.MailMessage
	changed := false
	lastUID := acc.LastUid
	if set != nil {
		msgs, err := c.Fetch(set, &imap.FetchOptions{
			UID: true, Flags: true, InternalDate: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
			BodySection: []*imap.FetchItemBodySection{summaryHeaderSection()},
		}).Collect()
		if err != nil {
			return err
		}
		msgs = slices.DeleteFunc(msgs, func(b *imapclient.FetchMessageBuffer) bool { return int64(b.UID) <= acc.LastUid })
		snippets := m.fetchSnippets(c, msgs)
		for _, b := range msgs {
			row := messageRow(acc.ID, b, snippets[b.UID])
			n, err := m.q.InsertMessage(ctx, row)
			if err != nil {
				return err
			}
			lastUID = max(lastUID, int64(b.UID))
			if n == 0 {
				continue
			}
			changed = true
			if row.Unread == 1 && m.now().Sub(b.InternalDate) <= notifyWindow {
				if saved, err := m.q.GetMessageByUID(ctx, db.GetMessageByUIDParams{AccountID: acc.ID, Uid: row.Uid}); err == nil {
					fresh = append(fresh, db.MailMessage{ID: saved.ID, Uid: saved.Uid, FromName: saved.FromName,
						FromAddress: saved.FromAddress, Subject: saved.Subject, Snippet: saved.Snippet, Date: saved.Date})
				}
			}
		}
	}
	flagsChanged, err := m.syncFlags(ctx, c, acc.ID)
	if err != nil {
		return err
	}
	now := m.now().UTC()
	if err := m.q.SetSyncState(ctx, db.SetSyncStateParams{UidValidity: acc.UidValidity, LastUid: lastUID, LastSyncAt: &now, ID: acc.ID}); err != nil {
		return err
	}
	if changed || flagsChanged {
		m.d.Bus.Publish("mail.updated", map[string]any{"accountId": acc.ID})
	}
	// B136: sort first. Action mail and verification codes are pushed at once.
	// Marketing mail is not pushed.
	sorted := m.triageArrived(ctx, acc, fresh)
	// B256: marketing mail can be marked read at once (setting).
	m.markPromoRead(ctx, c, acc, fresh, sorted)
	if acc.Notify == 1 {
		m.notifyNew(ctx, acc, fresh, sorted)
	}
	// After the pushes, so fetching bodies never delays a verification code.
	// Reuse the selected connection before IDLE; errors never fail the sync.
	if err := m.prefetchBodies(ctx, c, acc.ID); err != nil {
		m.d.Log.Warn("mail: prefetch bodies", "account", acc.ID, "err", err)
	}
	return nil
}

// syncFlags copies \Seen and \Flagged of the recent messages from the
// server, and drops the ones deleted there.
func (m *Module) syncFlags(ctx context.Context, c *imapclient.Client, id int64) (bool, error) {
	rows, err := m.q.RecentFlags(ctx, db.RecentFlagsParams{AccountID: id, Limit: flagSyncLimit})
	if err != nil || len(rows) == 0 {
		return false, err
	}
	var set imap.UIDSet
	for _, r := range rows {
		set.AddNum(imap.UID(r.Uid))
	}
	msgs, err := c.Fetch(set, &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return false, err
	}
	remote := make(map[int64][]imap.Flag, len(msgs))
	for _, b := range msgs {
		remote[int64(b.UID)] = b.Flags
	}
	changed := false
	for _, r := range rows {
		flags, ok := remote[r.Uid]
		if !ok {
			if err := m.deleteLocalMessage(ctx, id, r.Uid); err != nil {
				return changed, err
			}
			changed = true
			continue
		}
		unread, flagged := flagValues(flags)
		if unread != r.Unread || flagged != r.Flagged {
			if err := m.q.SetFlagsByUID(ctx, db.SetFlagsByUIDParams{Unread: unread, Flagged: flagged, AccountID: id, Uid: r.Uid}); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	return changed, nil
}

func flagValues(flags []imap.Flag) (unread, flagged int64) {
	unread = 1
	for _, f := range flags {
		switch {
		case strings.EqualFold(string(f), string(imap.FlagSeen)):
			unread = 0
		case strings.EqualFold(string(f), string(imap.FlagFlagged)):
			flagged = 1
		}
	}
	return unread, flagged
}

func messageRow(accountID int64, b *imapclient.FetchMessageBuffer, snippet string) db.InsertMessageParams {
	row := db.InsertMessageParams{AccountID: accountID, Uid: int64(b.UID), Snippet: snippet}
	row.Unread, row.Flagged = flagValues(b.Flags)
	date := b.InternalDate
	h := summaryHeader(b.FindBodySection(summaryHeaderSection()))
	subject, _ := h.Subject()
	row.Subject = cleanLine(subject)
	row.MessageID, _ = h.MessageID()
	if from, _ := h.AddressList("From"); len(from) > 0 {
		row.FromName = cleanLine(from[0].Name)
		row.FromAddress = from[0].Address
	}
	if date.IsZero() {
		date, _ = h.Date()
	}
	if date.IsZero() {
		date = time.Now()
	}
	row.Date = date.UTC().Truncate(time.Second)
	if b.BodyStructure != nil && hasAttachments(b.BodyStructure) {
		row.HasAttachments = 1
	}
	return row
}

func cleanLine(s string) string {
	s = strings.ToValidUTF8(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// notifyNew pushes new mail: one notice each, or one for all when many
// arrived at once. Promo mail is left out. Unsorted mail is still pushed.
// B256: with the "important only" setting, unimportant notices are left out too.
func (m *Module) notifyNew(ctx context.Context, acc db.MailAccount, msgs []db.MailMessage, sorted map[int64]api.MailTriage) {
	if len(msgs) == 0 {
		return
	}
	send := func(n notify.Notification, immediate bool) {
		n.Kind, n.Source = "mail.new", "mail"
		n.Scope = fmt.Sprintf("mail:%d", acc.ID) // B113: mute rules can name one mailbox
		// B136: mail that needs me is not held back for the digest (B127).
		// B225: a verification code is pushed at once too. Promo is not sent.
		n.Immediate = immediate
		if _, err := m.d.Notify.Send(ctx, n); err != nil {
			m.d.Log.Warn("mail: notify", "err", err)
		}
	}
	importantOnly := m.sorting(ctx).ImportantOnly
	var kept []db.MailMessage
	immediate := false
	for _, x := range msgs {
		skip, now, _ := mailPush(sorted, x.ID, importantOnly)
		if skip {
			continue
		}
		immediate = immediate || now
		kept = append(kept, x)
	}
	if len(kept) == 0 {
		return
	}
	if len(kept) > notifyGroup {
		var lines []string
		for _, x := range kept[len(kept)-notifyGroup:] {
			lines = append(lines, subjectOrNone(x.Subject))
		}
		slices.Reverse(lines)
		send(notify.Notification{
			Title: fmt.Sprintf("%s 收到 %d 封新邮件", acc.Name, len(kept)), Body: strings.Join(lines, "\n"),
			Link: fmt.Sprintf("/mail?a=%d", acc.ID), Data: map[string]any{"accountId": acc.ID, "count": len(kept)},
		}, immediate)
		return
	}
	for _, x := range kept {
		from := x.FromName
		if from == "" {
			from = x.FromAddress
		}
		title := from + "：" + subjectOrNone(x.Subject)
		_, now, prefix := mailPush(sorted, x.ID, importantOnly)
		if prefix != "" {
			title = prefix + title
		}
		send(notify.Notification{
			Title: title, Body: cutRunes(cleanText(x.Snippet), 100),
			Link: fmt.Sprintf("/mail?m=%d", x.ID), Data: map[string]any{"accountId": acc.ID, "messageId": x.ID},
		}, now)
	}
}

// mailPush decides how one sorted message is pushed. With importantOnly, a
// notice that is not important is not pushed, and an important one is pushed
// at once (B256).
func mailPush(sorted map[int64]api.MailTriage, id int64, importantOnly bool) (skip bool, immediate bool, prefix string) {
	t, ok := sorted[id]
	if !ok {
		return false, false, ""
	}
	if t.Category == api.Promo {
		return true, false, ""
	}
	if t.Code {
		return false, true, "（验证码）"
	}
	if t.Category == api.Action {
		return false, true, "（要处理）"
	}
	if importantOnly && t.Category == api.Notice {
		if !t.Important {
			return true, false, ""
		}
		return false, true, "（重要）"
	}
	return false, false, ""
}

func subjectOrNone(s string) string {
	if s == "" {
		return "（无主题）"
	}
	return s
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

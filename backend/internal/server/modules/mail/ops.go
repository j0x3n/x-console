package mail

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
)

// B217: 已读、星标、删除这些改动交给账号自己那条常连的连接去做。
//
// 阿里企业邮箱的已读状态改完又变回未读：原来另开一条连接 STORE，常连的那条
// 连接核对已读时拿到的还是旧状态，于是把本地改回未读。现在 STORE 和之后的核对
// 在同一条连接上，改完还看服务器返回的状态，没存上就报错。
// 常连的连接没连上时，才另开一条短连接。

// opKind is one change sent to the server.
type opKind int

const (
	opSeen opKind = iota
	opUnseen
	opFlag
	opUnflag
	opDelete
	opArchive
)

// jobWait is how long a request waits for the worker before it falls back
// to a short connection of its own.
const jobWait = 15 * time.Second

// opChunk is the most UIDs sent in one command.
const opChunk = 500

var errWorkerGone = errors.New("mail: worker connection ended")

var errNoArchive = httpx.Invalid("这个邮箱没有归档文件夹，也建不了")

type job struct {
	kind opKind
	uids []int64
	done chan error
}

func (w *worker) setLive(v bool) {
	w.mu.Lock()
	w.live = v
	w.mu.Unlock()
}

// push queues a job. It returns false when the connection is not up.
func (w *worker) push(j *job) bool {
	w.mu.Lock()
	if !w.live {
		w.mu.Unlock()
		return false
	}
	w.jobs = append(w.jobs, j)
	w.mu.Unlock()
	w.wake()
	return true
}

func (w *worker) take() []*job {
	w.mu.Lock()
	defer w.mu.Unlock()
	jobs := w.jobs
	w.jobs = nil
	return jobs
}

// unqueue takes a job back out of the queue. It returns false when the
// worker already took it.
func (w *worker) unqueue(j *job) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	i := slices.Index(w.jobs, j)
	if i < 0 {
		return false
	}
	w.jobs = slices.Delete(w.jobs, i, i+1)
	return true
}

// failJobs tells everyone still waiting that this connection is gone.
func (w *worker) failJobs() {
	w.mu.Lock()
	w.live = false
	jobs := w.jobs
	w.jobs = nil
	w.mu.Unlock()
	for _, j := range jobs {
		j.done <- errWorkerGone
	}
}

// runJobs does the queued changes on the worker's own connection.
func (m *Module) runJobs(ctx context.Context, c *imapclient.Client, w *worker) {
	for _, j := range w.take() {
		j.done <- m.execOp(ctx, c, w.id, j.kind, j.uids)
	}
}

// apply changes messages on the server. It uses the account's live
// connection when there is one.
func (m *Module) apply(ctx context.Context, accountID int64, kind opKind, uids []int64) error {
	if len(uids) == 0 {
		return nil
	}
	m.mu.Lock()
	w := m.workers[accountID]
	live := w != nil && m.states[accountID].Status == api.Ok
	m.mu.Unlock()
	if live {
		j := &job{kind: kind, uids: uids, done: make(chan error, 1)}
		if !w.push(j) {
			return m.applyDirect(ctx, accountID, kind, uids)
		}
		timer := time.NewTimer(jobWait)
		defer timer.Stop()
		select {
		case err := <-j.done:
			if !errors.Is(err, errWorkerGone) {
				return err
			}
		case <-timer.C:
			if !w.unqueue(j) { // 已经在做了，等它做完
				select {
				case err := <-j.done:
					if !errors.Is(err, errWorkerGone) {
						return err
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		case <-ctx.Done():
			w.unqueue(j)
			return ctx.Err()
		}
	}
	return m.applyDirect(ctx, accountID, kind, uids)
}

// applyDirect opens a short connection for the change.
func (m *Module) applyDirect(ctx context.Context, accountID int64, kind opKind, uids []int64) error {
	acc, err := m.getAccount(ctx, accountID)
	if err != nil {
		return err
	}
	cfg, err := m.accountConfig(acc)
	if err != nil {
		return err
	}
	c, _, err := m.connect(ctx, cfg, nil)
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	if err := m.execOp(ctx, c, accountID, kind, uids); err != nil {
		return err
	}
	_ = c.Logout().Wait()
	return nil
}

// execOp sends one change on a connection that has INBOX selected.
func (m *Module) execOp(ctx context.Context, c *imapclient.Client, accountID int64, kind opKind, uids []int64) error {
	// 全部已读时可能有几千个 uid，一条命令太长有的服务器不收
	if len(uids) > opChunk {
		for chunk := range slices.Chunk(uids, opChunk) {
			if err := m.execOp(ctx, c, accountID, kind, chunk); err != nil {
				return err
			}
		}
		return nil
	}
	var set imap.UIDSet
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	if kind == opDelete {
		return m.deleteOnServer(ctx, c, accountID, set)
	}
	if kind == opArchive {
		return archiveOnServer(c, set)
	}
	flag, on := imap.FlagSeen, true
	switch kind {
	case opUnseen:
		on = false
	case opFlag:
		flag = imap.FlagFlagged
	case opUnflag:
		flag, on = imap.FlagFlagged, false
	}
	op := imap.StoreFlagsDel
	if on {
		op = imap.StoreFlagsAdd
	}
	// 不用 .SILENT：服务器回出改完的状态，可以核对有没有存上
	cmd := c.Store(set, &imap.StoreFlags{Op: op, Flags: []imap.Flag{flag}}, nil)
	var bad error
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		var uid imap.UID
		var flags []imap.Flag
		got := false
		for item := msg.Next(); item != nil; item = msg.Next() {
			switch d := item.(type) {
			case imapclient.FetchItemDataUID:
				uid = d.UID
			case imapclient.FetchItemDataFlags:
				flags, got = d.Flags, true // 没有状态时是空列表，要和“没回状态”分开
			}
		}
		if !got {
			continue // 这条回应没带状态，没法核对
		}
		if slices.ContainsFunc(flags, func(f imap.Flag) bool { return strings.EqualFold(string(f), string(flag)) }) != on {
			m.d.Log.Warn("mail: server did not keep flag", "account", accountID, "uid", uid, "flag", flag, "on", on, "flags", flags)
			bad = fmt.Errorf("邮箱服务器没有保存这个状态（%s）", flag)
		}
	}
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("改邮箱上的状态失败：%w", err)
	}
	return bad
}

// trashNames are folder names used for deleted mail when the server does
// not mark one with \Trash.
var trashNames = []string{"trash", "deleted messages", "deleted items", "deleted", "已删除", "已删除邮件", "垃圾箱"}

// deleteOnServer moves messages to the trash folder. Without one it marks
// them \Deleted and expunges.
func (m *Module) deleteOnServer(ctx context.Context, c *imapclient.Client, accountID int64, set imap.UIDSet) error {
	trash, err := trashFolder(c)
	if err != nil {
		return fmt.Errorf("读邮箱文件夹失败：%w", err)
	}
	if trash != "" {
		if _, err := c.Move(set, trash).Wait(); err != nil {
			return fmt.Errorf("移到%s失败：%w", trash, err)
		}
		return nil
	}
	m.d.Log.Info("mail: no trash folder, deleting for good", "account", accountID)
	return deletePermanently(c, set)
}

func deletePermanently(c *imapclient.Client, set imap.UIDSet) error {
	if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return fmt.Errorf("删除失败：%w", err)
	}
	var exp *imapclient.ExpungeCommand
	if c.Caps().Has(imap.CapUIDPlus) {
		exp = c.UIDExpunge(set)
	} else {
		exp = c.Expunge()
	}
	if err := exp.Close(); err != nil {
		return fmt.Errorf("删除失败：%w", err)
	}
	return nil
}

// trashFolder finds the trash: first the one marked \Trash, then by name.
func trashFolder(c *imapclient.Client) (string, error) {
	return specialFolder(c, imap.MailboxAttrTrash, trashNames)
}

var archiveNames = []string{"archive", "archives", "归档", "已归档"}

func archiveOnServer(c *imapclient.Client, set imap.UIDSet) error {
	folder, err := specialFolder(c, imap.MailboxAttrArchive, archiveNames)
	if err != nil {
		return fmt.Errorf("读邮箱文件夹失败：%w", err)
	}
	if folder == "" {
		// Gmail archives into the folder marked \\All. Keep this fallback
		// separate from specialFolder so trash lookup is unchanged.
		folder, err = specialFolder(c, imap.MailboxAttrAll, nil)
		if err != nil {
			return fmt.Errorf("读邮箱文件夹失败：%w", err)
		}
		if folder != "" {
			if selected := c.Mailbox(); selected != nil && selected.Name == folder {
				return nil
			}
		}
	}
	if folder == "" {
		folder = "Archive"
		if err := c.Create(folder, nil).Wait(); err != nil {
			return errNoArchive
		}
	}
	if c.Caps().Has(imap.CapMove) {
		if _, err := c.Move(set, folder).Wait(); err != nil {
			return fmt.Errorf("归档失败：%w", err)
		}
		return nil
	}
	// Wait for COPY to succeed before setting \\Deleted. The client's Move
	// fallback pipelines these commands and could delete mail if COPY fails.
	if _, err := c.Copy(set, folder).Wait(); err != nil {
		return fmt.Errorf("归档失败：%w", err)
	}
	return deletePermanently(c, set)
}

// specialFolder prefers a special-use attribute over a case-insensitive name.
func specialFolder(c *imapclient.Client, attr imap.MailboxAttr, names []string) (string, error) {
	var opts *imap.ListOptions
	if c.Caps().Has(imap.CapSpecialUse) {
		opts = &imap.ListOptions{ReturnSpecialUse: true}
	}
	boxes, err := c.List("", "*", opts).Collect()
	if err != nil {
		return "", err
	}
	byName := ""
	for _, b := range boxes {
		if strings.EqualFold(b.Mailbox, "INBOX") || slices.Contains(b.Attrs, imap.MailboxAttrNoSelect) {
			continue
		}
		if slices.Contains(b.Attrs, attr) {
			return b.Mailbox, nil
		}
		name := strings.ToLower(b.Mailbox)
		if b.Delim != 0 {
			if i := strings.LastIndexByte(name, byte(b.Delim)); i >= 0 {
				name = name[i+1:]
			}
		}
		if byName == "" && slices.Contains(names, name) {
			byName = b.Mailbox
		}
	}
	return byName, nil
}

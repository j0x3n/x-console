package mail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // GBK、GB2312 这类编码
	gomail "github.com/emersion/go-message/mail"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

const (
	maxMessageBytes = 25 << 20 // 整封邮件最多取这么多
	maxHTMLBytes    = 2 << 20
	maxTextBytes    = 2 << 20
)

// bodyJSON is what mail_messages.body_json holds.
type bodyJSON struct {
	To          []api.MailAddress    `json:"to"`
	Cc          []api.MailAddress    `json:"cc"`
	HTML        *string              `json:"html,omitempty"`
	Text        string               `json:"text"`
	Attachments []api.MailAttachment `json:"attachments"`
}

func summaryView(r db.ListMessagesRow) api.MailSummary {
	from := api.MailAddress{Address: r.FromAddress}
	if r.FromName != "" {
		name := r.FromName
		from.Name = &name
	}
	return api.MailSummary{
		Id: r.ID, AccountId: r.AccountID, From: from, Subject: r.Subject, Snippet: cleanText(r.Snippet), Date: r.Date,
		Unread: r.Unread == 1, Flagged: r.Flagged == 1, HasAttachments: r.HasAttachments == 1,
	}
}

func rowSummary(r db.MailMessage) api.MailSummary {
	return summaryView(db.ListMessagesRow{
		ID: r.ID, AccountID: r.AccountID, Uid: r.Uid, MessageID: r.MessageID, FromName: r.FromName, FromAddress: r.FromAddress,
		Subject: r.Subject, Snippet: cleanText(r.Snippet), Date: r.Date, Unread: r.Unread, Flagged: r.Flagged, HasAttachments: r.HasAttachments,
	})
}

func (m *Module) getMessage(ctx context.Context, id int64) (db.MailMessage, error) {
	row, err := m.q.GetMessage(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

func (m *Module) ListMailMessages(w http.ResponseWriter, r *http.Request, params api.ListMailMessagesParams) {
	limit := 50
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 100)
	}
	p := db.ListMessagesParams{OnlyUnread: 0, Q: strings.TrimSpace(deref(params.Q)), Lim: int64(limit + 1)}
	if utf8.RuneCountInString(p.Q.(string)) > 100 {
		httpx.Fail(w, r, httpx.Invalid("搜索词太长了"))
		return
	}
	if params.AccountId != nil {
		p.AccountID = *params.AccountId
	}
	if params.Unread != nil && *params.Unread {
		p.OnlyUnread = 1
	}
	if params.Before != nil {
		p.Before = params.Before.UTC()
	}
	rows, err := m.q.ListMessages(r.Context(), p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.MailPage{Items: make([]api.MailSummary, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		next := rows[limit-1].Date
		out.NextBefore = &next
	}
	for _, row := range rows {
		out.Items = append(out.Items, summaryView(row))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetMailMessage(w http.ResponseWriter, r *http.Request, messageID api.MessageId) {
	ctx := r.Context()
	row, err := m.getMessage(ctx, messageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body, err := m.loadBody(ctx, row)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s := rowSummary(row)
	httpx.JSON(w, http.StatusOK, api.MailMessage{
		Id: s.Id, AccountId: s.AccountId, From: s.From, Subject: s.Subject, Snippet: s.Snippet, Date: s.Date,
		Unread: s.Unread, Flagged: s.Flagged, HasAttachments: s.HasAttachments,
		To: body.To, Cc: body.Cc, Html: body.HTML, Text: body.Text, Attachments: body.Attachments,
	})
}

// loadBody returns the cached body, or fetches the whole message once.
func (m *Module) loadBody(ctx context.Context, row db.MailMessage) (bodyJSON, error) {
	var body bodyJSON
	if row.BodyJson != nil {
		if err := json.Unmarshal([]byte(*row.BodyJson), &body); err == nil {
			return body, nil
		}
	}
	acc, err := m.getAccount(ctx, row.AccountID)
	if err != nil {
		return body, err
	}
	cfg, err := m.accountConfig(acc)
	if err != nil {
		return body, httpx.NewError(http.StatusBadGateway, "mail_unavailable", err.Error())
	}
	c, _, err := m.connect(ctx, cfg, nil)
	if err != nil {
		return body, httpx.NewError(http.StatusBadGateway, "mail_unavailable", err.Error())
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	prefix := fmt.Sprintf("%d/%d", row.AccountID, row.Uid)
	m.deleteFiles(ctx, prefix) // 上次没取完留下的
	cmd := c.Fetch(imap.UIDSetNum(imap.UID(row.Uid)), &imap.FetchOptions{
		UID: true, BodySection: []*imap.FetchItemBodySection{{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: maxMessageBytes}}},
	})
	found := false
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		for item := msg.Next(); item != nil; item = msg.Next() {
			sec, ok := item.(imapclient.FetchItemDataBodySection)
			if !ok || sec.Literal == nil || found {
				continue
			}
			found = true
			if body, err = m.parseMessage(ctx, sec.Literal, prefix); err != nil {
				_ = cmd.Close()
				return body, httpx.NewError(http.StatusBadGateway, "mail_unavailable", "邮件解析失败："+err.Error())
			}
		}
	}
	if err := cmd.Close(); err != nil {
		return body, httpx.NewError(http.StatusBadGateway, "mail_unavailable", "取邮件失败："+err.Error())
	}
	if !found {
		return body, httpx.NewError(http.StatusNotFound, "not_found", "这封邮件在邮箱里已经没有了")
	}
	_ = c.Logout().Wait()
	raw, err := json.Marshal(body)
	if err != nil {
		return body, err
	}
	s := string(raw)
	if err := m.q.SetMessageBody(ctx, db.SetMessageBodyParams{BodyJson: &s, ID: row.ID}); err != nil {
		return body, err
	}
	return body, nil
}

// parseMessage reads a whole message. Attachments and inline images go to
// the file store under prefix/<index>.
func (m *Module) parseMessage(ctx context.Context, r io.Reader, prefix string) (bodyJSON, error) {
	body := bodyJSON{To: []api.MailAddress{}, Cc: []api.MailAddress{}, Attachments: []api.MailAttachment{}}
	mr, err := gomail.CreateReader(r)
	if err != nil && !message.IsUnknownCharset(err) {
		if mr == nil {
			return body, err
		}
	}
	body.To = addresses(mr.Header, "To")
	body.Cc = addresses(mr.Header, "Cc")
	var htmlBody string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil && (p == nil || !message.IsUnknownCharset(err)) {
			m.d.Log.Warn("mail: part", "err", err)
			break // 留下已经读到的部分
		}
		var (
			mediaType string
			params    map[string]string
			name      string
			cid       string
			isAttach  bool
		)
		switch h := p.Header.(type) {
		case *gomail.InlineHeader:
			mediaType, params, _ = h.ContentType()
			cid = strings.Trim(strings.TrimSpace(h.Get("Content-Id")), "<>")
			name = params["name"]
		case *gomail.AttachmentHeader:
			mediaType, params, _ = h.ContentType()
			cid = strings.Trim(strings.TrimSpace(h.Get("Content-Id")), "<>")
			name, _ = h.Filename()
			if name == "" {
				name = params["name"]
			}
			isAttach = true
		default:
			continue
		}
		mediaType = strings.ToLower(mediaType)
		if !isAttach && cid == "" {
			switch {
			case mediaType == "text/plain" && body.Text == "":
				b, _ := io.ReadAll(io.LimitReader(p.Body, maxTextBytes))
				body.Text = strings.ToValidUTF8(string(b), "")
				continue
			case mediaType == "text/html" && htmlBody == "":
				b, _ := io.ReadAll(io.LimitReader(p.Body, maxHTMLBytes))
				htmlBody = strings.ToValidUTF8(string(b), "")
				continue
			case strings.HasPrefix(mediaType, "text/") || strings.HasPrefix(mediaType, "multipart/"):
				continue
			}
		}
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		idx := len(body.Attachments)
		if name == "" {
			name = fmt.Sprintf("附件 %d", idx+1)
			if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
				name += exts[0]
			}
		}
		cr := &countingReader{r: p.Body}
		if err := m.files.Put(ctx, prefix+"/"+strconv.Itoa(idx), cr, -1); err != nil {
			return body, fmt.Errorf("存附件失败: %w", err)
		}
		att := api.MailAttachment{Index: idx, Name: name, Size: cr.n, Mime: mediaType}
		if cid != "" {
			att.ContentId = &cid
		}
		body.Attachments = append(body.Attachments, att)
	}
	if htmlBody != "" {
		body.HTML = &htmlBody
		if body.Text == "" {
			body.Text = htmlToText(htmlBody)
		}
	}
	return body, nil
}

func addresses(h gomail.Header, key string) []api.MailAddress {
	list, _ := h.AddressList(key)
	out := make([]api.MailAddress, 0, len(list))
	for _, a := range list {
		x := api.MailAddress{Address: a.Address}
		if a.Name != "" {
			name := a.Name
			x.Name = &name
		}
		out = append(out, x)
	}
	return out
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (m *Module) UpdateMailMessage(w http.ResponseWriter, r *http.Request, messageID api.MessageId) {
	ctx := r.Context()
	var body api.MailMessagePatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.getMessage(ctx, messageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Unread == nil && body.Flagged == nil {
		httpx.JSON(w, http.StatusOK, rowSummary(row))
		return
	}
	if err := m.storeFlags(ctx, row, body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Unread != nil {
		row.Unread = boolInt(*body.Unread)
	}
	if body.Flagged != nil {
		row.Flagged = boolInt(*body.Flagged)
	}
	if err := m.q.SetMessageFlags(ctx, db.SetMessageFlagsParams{Unread: row.Unread, Flagged: row.Flagged, ID: row.ID}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("mail.updated", map[string]any{"accountId": row.AccountID})
	httpx.JSON(w, http.StatusOK, rowSummary(row))
}

// storeFlags changes \Seen and \Flagged on the server with UID STORE.
func (m *Module) storeFlags(ctx context.Context, row db.MailMessage, p api.MailMessagePatch) error {
	acc, err := m.getAccount(ctx, row.AccountID)
	if err != nil {
		return err
	}
	cfg, err := m.accountConfig(acc)
	if err != nil {
		return httpx.NewError(http.StatusBadGateway, "mail_unavailable", err.Error())
	}
	c, _, err := m.connect(ctx, cfg, nil)
	if err != nil {
		return httpx.NewError(http.StatusBadGateway, "mail_unavailable", err.Error())
	}
	defer c.Close()
	uid := imap.UIDSetNum(imap.UID(row.Uid))
	change := func(flag imap.Flag, on bool) error {
		op := imap.StoreFlagsDel
		if on {
			op = imap.StoreFlagsAdd
		}
		return c.Store(uid, &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{flag}}, nil).Close()
	}
	if p.Unread != nil {
		if err := change(imap.FlagSeen, !*p.Unread); err != nil {
			return httpx.NewError(http.StatusBadGateway, "mail_unavailable", "改邮箱上的状态失败："+err.Error())
		}
	}
	if p.Flagged != nil {
		if err := change(imap.FlagFlagged, *p.Flagged); err != nil {
			return httpx.NewError(http.StatusBadGateway, "mail_unavailable", "改邮箱上的状态失败："+err.Error())
		}
	}
	_ = c.Logout().Wait()
	return nil
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// inlineTypes are the image types shown inline. SVG is not one of them: it
// can carry scripts.
var inlineTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/jpg": true, "image/gif": true, "image/webp": true, "image/bmp": true, "image/avif": true,
}

func (m *Module) DownloadMailAttachment(w http.ResponseWriter, r *http.Request, messageID api.MessageId, index int) {
	ctx := r.Context()
	row, err := m.getMessage(ctx, messageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body bodyJSON
	if row.BodyJson == nil || json.Unmarshal([]byte(*row.BodyJson), &body) != nil || index < 0 || index >= len(body.Attachments) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	att := body.Attachments[index]
	f, info, err := m.files.Get(ctx, fmt.Sprintf("%d/%d/%d", row.AccountID, row.Uid, index))
	if errors.Is(err, files.ErrNotFound) {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer f.Close()
	disposition, ctype := "attachment", att.Mime
	if att.ContentId != nil && inlineTypes[att.Mime] {
		disposition = "inline"
	} else if !strings.Contains(ctype, "/") {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": att.Name}))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func (m *Module) GetMailSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accounts, err := m.q.ListAccounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	counts, err := m.unreadCounts(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	latest, err := m.q.LatestUnread(ctx, 5)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.MailSummaryView{Latest: make([]api.MailSummary, 0, len(latest))}
	for _, a := range accounts {
		out.Unread += counts[a.ID]
		out.Accounts = append(out.Accounts, struct {
			Id     int64                 `json:"id"`
			Name   string                `json:"name"`
			Status api.MailAccountStatus `json:"status"`
			Unread int                   `json:"unread"`
		}{Id: a.ID, Name: a.Name, Status: m.state(a.ID).Status, Unread: counts[a.ID]})
	}
	if out.Accounts == nil {
		out.Accounts = out.Accounts[:0]
	}
	for _, x := range latest {
		out.Latest = append(out.Latest, summaryView(db.ListMessagesRow(x)))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// deleteFiles removes every stored file under prefix ("<account>" or
// "<account>/<uid>").
func (m *Module) deleteFiles(ctx context.Context, prefix string) {
	var keys []string
	for info, err := range m.files.List(ctx, prefix) {
		if err != nil {
			break
		}
		keys = append(keys, info.Key)
	}
	for _, k := range keys {
		if err := m.files.Delete(ctx, k); err != nil {
			m.d.Log.Warn("mail: delete file", "key", k, "err", err)
		}
	}
}

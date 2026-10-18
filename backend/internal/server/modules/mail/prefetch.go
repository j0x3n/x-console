package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

const (
	bodyPrefetchLimit = 50
	bodyPrefetchAge   = 30 * 24 * time.Hour
)

func bodySection() *imap.FetchItemBodySection {
	return &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: maxMessageBytes}}
}

// prefetchBodies fetches one bounded UID batch on the worker connection.
// Collect one message at a time so a batch cannot hold 50 large bodies in RAM.
func (m *Module) prefetchBodies(ctx context.Context, c *imapclient.Client, accountID int64) error {
	rows, err := m.q.RecentUncachedBodies(ctx, db.RecentUncachedBodiesParams{
		AccountID: accountID, Date: m.now().Add(-bodyPrefetchAge).UTC(), Limit: bodyPrefetchLimit,
	})
	if err != nil || len(rows) == 0 {
		return err
	}
	byUID := make(map[imap.UID]db.MailMessage, len(rows))
	var set imap.UIDSet
	for _, row := range rows {
		uid := imap.UID(row.Uid)
		set.AddNum(uid)
		byUID[uid] = row
	}
	section := bodySection()
	cmd := c.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}})
	defer cmd.Close()
	for msg := cmd.Next(); msg != nil; msg = cmd.Next() {
		buf, err := msg.Collect()
		if err != nil {
			return err
		}
		row, ok := byUID[buf.UID]
		raw := buf.FindBodySection(section)
		if !ok || raw == nil {
			continue
		}
		if err := m.cachePrefetchedBody(ctx, row, raw); err != nil {
			m.d.Log.Warn("mail: prefetch body", "account", accountID, "uid", row.Uid, "err", err)
		}
	}
	return cmd.Close()
}

func (m *Module) cachePrefetchedBody(ctx context.Context, row db.MailMessage, raw []byte) error {
	unlock := m.lockBody(row.AccountID, row.Uid)
	defer unlock()
	row, err := m.getMessage(ctx, row.ID)
	if err != nil {
		return err
	}
	var body bodyJSON
	if row.BodyJson != nil && json.Unmarshal([]byte(*row.BodyJson), &body) == nil {
		return nil // An open already cached this body and its attachments.
	}
	_, err = m.storeBody(ctx, row, bytes.NewReader(raw))
	return err
}

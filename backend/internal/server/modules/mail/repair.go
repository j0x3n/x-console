package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

const summaryRepairBatch = 200

// Repair legacy summaries in bounded batches on the existing connection.
// The cursor advances even for genuinely empty headers or vanished UIDs.
// Good cached bodies, IDs, triage and message flags stay intact. Empty legacy
// bodies are fetched again with PEEK instead of being cached forever.
func (m *Module) repairSummaryBatch(ctx context.Context, c *imapclient.Client, accountID, cursor int64) (int64, bool, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT uid FROM mail_messages
		WHERE account_id = ? AND uid > ? AND (subject = '' OR from_address = '' OR
			CASE WHEN json_valid(body_json) THEN
				COALESCE(json_extract(body_json, '$.version'), 0) = 0 AND
				COALESCE(json_extract(body_json, '$.text'), '') = '' AND
				COALESCE(json_extract(body_json, '$.html'), '') = ''
			ELSE body_json IS NOT NULL END)
		ORDER BY uid LIMIT ?`, accountID, cursor, summaryRepairBatch)
	if err != nil {
		return cursor, false, err
	}
	var set imap.UIDSet
	count := 0
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return cursor, false, err
		}
		set.AddNum(imap.UID(uid))
		cursor = uid
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil || count == 0 {
		return cursor, count == 0, err
	}
	msgs, err := fetchSummaries(c, set)
	if err != nil {
		return cursor, false, err
	}
	changed := false
	for _, b := range msgs {
		if !set.Contains(b.UID) {
			continue
		}
		row := messageRow(accountID, b, "")
		result, err := m.d.DB.ExecContext(ctx, `UPDATE mail_messages SET
			subject = CASE WHEN ? <> '' THEN ? ELSE subject END,
			from_name = CASE WHEN ? <> '' THEN ? ELSE from_name END,
			from_address = CASE WHEN ? <> '' THEN ? ELSE from_address END,
			message_id = CASE WHEN ? <> '' THEN ? ELSE message_id END
			WHERE account_id = ? AND uid = ?`, row.Subject, row.Subject, row.FromName, row.FromName,
			row.FromAddress, row.FromAddress, row.MessageID, row.MessageID, accountID, row.Uid)
		if err != nil {
			return cursor, false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return cursor, false, err
		}
		changed = changed || n > 0
		existing, err := m.q.GetMessageByUID(ctx, db.GetMessageByUIDParams{AccountID: accountID, Uid: int64(b.UID)})
		if err != nil {
			return cursor, false, err
		}
		message, err := m.getMessage(ctx, existing.ID)
		if err != nil {
			return cursor, false, err
		}
		if err := m.repairMessageBody(ctx, c, message); err != nil {
			return cursor, false, err
		}
	}
	if changed {
		m.d.Bus.Publish("mail.updated", map[string]any{"accountId": accountID})
	}
	return cursor, count < summaryRepairBatch, nil
}

func (m *Module) repairMessageBody(ctx context.Context, c *imapclient.Client, row db.MailMessage) error {
	var cached bodyJSON
	if row.BodyJson != nil && json.Unmarshal([]byte(*row.BodyJson), &cached) == nil && !bodyNeedsRepair(cached) {
		return m.mergeBodySummary(ctx, row, cached)
	}
	section := bodySection()
	msgs, err := c.Fetch(imap.UIDSetNum(imap.UID(row.Uid)), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return fmt.Errorf("补取邮件正文: %w", err)
	}
	for _, b := range msgs {
		if b.UID != imap.UID(row.Uid) {
			continue
		}
		if raw := messageBodyBytes(b); raw != nil {
			unlock := m.lockBody(row.AccountID, row.Uid)
			defer unlock()
			// An open may have filled the cache while this FETCH was running.
			current, err := m.getMessage(ctx, row.ID)
			if err != nil {
				return err
			}
			if current.BodyJson != nil && json.Unmarshal([]byte(*current.BodyJson), &cached) == nil && !bodyNeedsRepair(cached) {
				return m.mergeBodySummary(ctx, current, cached)
			}
			_, err = m.storeBody(ctx, current, bytes.NewReader(raw))
			return err
		}
	}
	return nil // A message may have been removed remotely during the batch.
}

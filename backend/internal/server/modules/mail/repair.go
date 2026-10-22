package mail

import (
	"context"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

const summaryRepairBatch = 200

// Repair legacy summaries in bounded batches on the existing connection.
// The cursor advances even for genuinely empty headers or vanished UIDs.
// Only metadata is updated. IDs, bodies, triage and message flags stay intact.
func (m *Module) repairSummaryBatch(ctx context.Context, c *imapclient.Client, accountID, cursor int64) (int64, bool, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT uid FROM mail_messages
		WHERE account_id = ? AND uid > ? AND (subject = '' OR from_address = '')
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
	}
	if changed {
		m.d.Bus.Publish("mail.updated", map[string]any{"accountId": accountID})
	}
	return cursor, count < summaryRepairBatch, nil
}

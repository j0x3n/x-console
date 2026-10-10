package documents

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// kindLabels are the names used in notifications.
var kindLabels = map[api.DocumentKind]string{
	api.Passport: "护照", api.IdCard: "身份证", api.DriverLicense: "驾照", api.Visa: "签证",
	api.Contract: "合同", api.Insurance: "保险", api.Item: "物品保修", api.Other: "证件",
}

// remindAll is the hourly job.
func (m *Module) remindAll(ctx context.Context) error {
	rows, err := m.q.ListDocuments(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		m.remindNow(ctx, row)
	}
	return nil
}

// remindNow tells the user about one entry if a reminder day has come and was
// not told yet. When several days came at once (an entry added late), one
// notification is sent and all of them count as told, so there is no burst.
// Day 0 is always a reminder day: the expiry day and after it.
func (m *Module) remindNow(ctx context.Context, row db.Document) {
	if row.ArchivedAt != nil || row.ExpiresOn == "" {
		return
	}
	left := daysLeft(row.ExpiresOn, m.today())
	if left == nil {
		return
	}
	told := []int{}
	if row.NotifiedFor == row.ExpiresOn {
		_ = json.Unmarshal([]byte(row.NotifiedJson), &told)
	}
	due := []int{}
	for _, d := range append(parseRemind(row.RemindDays), 0) {
		if *left <= d && !slices.Contains(told, d) {
			due = append(due, d)
		}
	}
	if len(due) == 0 {
		return
	}
	v, err := m.view(row, m.today())
	if err != nil {
		m.d.Log.Warn("document remind", "id", row.ID, "err", err)
		return
	}
	n := notify.Notification{
		Kind: "documents.expiring", Source: "documents", Link: "/documents",
		Title:    reminderTitle(v, *left),
		Body:     reminderBody(v, *left),
		Priority: notify.PriorityNormal,
		Scope:    "documents:" + strconv.FormatInt(row.ID, 10),
	}
	if *left <= 7 {
		n.Priority = notify.PriorityHigh
	}
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("document remind", "id", row.ID, "err", err)
		return
	}
	told = append(told, due...)
	raw, _ := json.Marshal(told)
	if err := m.q.SetDocumentNotified(ctx, db.SetDocumentNotifiedParams{NotifiedFor: row.ExpiresOn, NotifiedJson: string(raw), ID: row.ID}); err != nil {
		m.d.Log.Warn("document remind state", "id", row.ID, "err", err)
	}
}

func reminderTitle(v api.Document, left int) string {
	label := kindLabels[v.Kind]
	switch {
	case left > 0:
		return fmt.Sprintf("%s「%s」还有 %d 天到期", label, v.Name, left)
	case left == 0:
		return fmt.Sprintf("%s「%s」今天到期", label, v.Name)
	default:
		return fmt.Sprintf("%s「%s」已过期 %d 天", label, v.Name, -left)
	}
}

// reminderBody never contains the number.
func reminderBody(v api.Document, left int) string {
	body := "到期日 " + v.ExpiresOn
	if v.Holder != "" {
		body += "，持有人 " + v.Holder
	}
	if left <= 30 && left >= 0 && (v.Kind == api.Passport || v.Kind == api.Visa) {
		body += "。补办要几周，尽快处理"
	}
	return body
}

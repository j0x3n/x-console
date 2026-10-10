package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// staleRepeat is how many days pass between two reminders about a rotation
// that is still overdue.
const staleRepeat = 30

// kindLabels are the names used in notifications.
var kindLabels = map[api.CredentialKind]string{
	api.ApiKey: "API 密钥", api.AccessToken: "访问令牌", api.SshKey: "SSH 密钥", api.SigningKey: "签名密钥", api.Other: "密钥",
}

// remindAll is the hourly job.
func (m *Module) remindAll(ctx context.Context) error {
	rows, err := m.q.ListCredentials(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		m.remindNow(ctx, row)
	}
	return nil
}

func (m *Module) remindNow(ctx context.Context, row db.Credential) {
	if row.ArchivedAt != nil {
		return
	}
	v := m.view(row, m.today())
	m.remindExpiry(ctx, row, v)
	m.remindStale(ctx, row, v)
}

// remindExpiry works like the one for documents: a reminder day that has come
// and was not told yet sends one notification, several at once count as one.
// Day 0 is always a reminder day: the expiry day and after it.
func (m *Module) remindExpiry(ctx context.Context, row db.Credential, v api.Credential) {
	if v.ExpiresIn == nil {
		return
	}
	left := *v.ExpiresIn
	told := []int{}
	if row.NotifiedFor == row.ExpiresOn {
		_ = json.Unmarshal([]byte(row.NotifiedJson), &told)
	}
	due := []int{}
	for _, d := range append(parseRemind(row.RemindDays), 0) {
		if left <= d && !slices.Contains(told, d) {
			due = append(due, d)
		}
	}
	if len(due) == 0 {
		return
	}
	n := notify.Notification{
		Kind: "credentials.expiring", Source: "credentials", Link: "/credentials",
		Title:    expiryTitle(v, left),
		Body:     expiryBody(v),
		Priority: notify.PriorityNormal,
		Scope:    "credentials:" + strconv.FormatInt(row.ID, 10),
	}
	if left <= 7 {
		n.Priority = notify.PriorityHigh
	}
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("credential remind", "id", row.ID, "err", err)
		return
	}
	told = append(told, due...)
	raw, _ := json.Marshal(told)
	if err := m.q.SetCredentialNotified(ctx, db.SetCredentialNotifiedParams{NotifiedFor: row.ExpiresOn, NotifiedJson: string(raw), ID: row.ID}); err != nil {
		m.d.Log.Warn("credential remind state", "id", row.ID, "err", err)
	}
}

// remindStale tells once when the rotation is overdue and again every
// staleRepeat days until it is rotated. A new rotation date starts over.
func (m *Module) remindStale(ctx context.Context, row db.Credential, v api.Credential) {
	due := rotateDueDate(row)
	if due == "" || v.RotateDueIn == nil || *v.RotateDueIn > 0 {
		return
	}
	today := m.today()
	if row.StaleFor == due {
		if last, err := time.Parse(dateLayout, row.StaleNotifiedOn); err == nil && int(today.Sub(last).Hours()/24) < staleRepeat {
			return
		}
	}
	n := notify.Notification{
		Kind: "credentials.stale", Source: "credentials", Link: "/credentials",
		Title:    fmt.Sprintf("%s「%s」超过 %d 天没更换", label(v), v.Name, row.RotateEveryDays),
		Body:     staleBody(v),
		Priority: notify.PriorityNormal,
		Scope:    "credentials:" + strconv.FormatInt(row.ID, 10) + ":stale",
	}
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("credential stale remind", "id", row.ID, "err", err)
		return
	}
	if err := m.q.SetCredentialStale(ctx, db.SetCredentialStaleParams{StaleFor: due, StaleNotifiedOn: today.Format(dateLayout), ID: row.ID}); err != nil {
		m.d.Log.Warn("credential stale state", "id", row.ID, "err", err)
	}
}

func label(v api.Credential) string {
	l := kindLabels[v.Kind]
	if v.Platform != "" {
		l += " " + v.Platform
	}
	return l
}

func expiryTitle(v api.Credential, left int) string {
	switch {
	case left > 0:
		return fmt.Sprintf("%s「%s」还有 %d 天到期", label(v), v.Name, left)
	case left == 0:
		return fmt.Sprintf("%s「%s」今天到期", label(v), v.Name)
	default:
		return fmt.Sprintf("%s「%s」已过期 %d 天", label(v), v.Name, -left)
	}
}

// usedByText names the first places, for the notification body.
func usedByText(v api.Credential) string {
	if len(v.UsedBy) == 0 {
		return ""
	}
	shown := v.UsedBy
	if len(shown) > 3 {
		shown = shown[:3]
	}
	text := "用在 " + strings.Join(shown, "、")
	if len(v.UsedBy) > len(shown) {
		text += fmt.Sprintf(" 等 %d 处", len(v.UsedBy))
	}
	return text
}

// The bodies never contain the hint.
func expiryBody(v api.Credential) string {
	body := "到期日 " + v.ExpiresOn
	if u := usedByText(v); u != "" {
		body += "，" + u
	}
	return body + "。换完以后在台账里点“已更换”"
}

func staleBody(v api.Credential) string {
	base := v.RotatedOn
	if base == "" {
		base = v.CreatedOn
	}
	body := fmt.Sprintf("上次更换 %s，周期 %d 天", base, v.RotateEveryDays)
	if u := usedByText(v); u != "" {
		body += "，" + u
	}
	return body
}

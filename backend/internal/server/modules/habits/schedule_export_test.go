package habits

import "context"

func SendHostReminder(m *Module, ctx context.Context, ids []string, title, body string) {
	m.sendHostReminder(ctx, ids, title, body)
}

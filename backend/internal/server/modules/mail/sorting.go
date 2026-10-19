package mail

import (
	"context"
	"errors"
	"net/http"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B256: two settings that use the AI sorting result. They apply to every
// mailbox that has sorting on.
//   - MarkPromoRead: mail sorted as marketing when it arrives is marked read on
//     the mailbox server.
//   - ImportantOnly: a notice is pushed only when the sorting says it is
//     important. Action mail, verification codes and mail that was not sorted
//     are pushed as before.
const sortingKey = "mail.sorting"

type sortingSetting struct {
	MarkPromoRead bool `json:"markPromoRead"`
	ImportantOnly bool `json:"importantOnly"`
}

func (m *Module) sorting(ctx context.Context) sortingSetting {
	var s sortingSetting
	if err := m.d.Settings.Get(ctx, sortingKey, &s); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("mail: load sorting setting", "err", err)
	}
	return s
}

// GetMailSorting returns the two sorting settings.
func (m *Module) GetMailSorting(w http.ResponseWriter, r *http.Request) {
	s := m.sorting(r.Context())
	httpx.JSON(w, http.StatusOK, api.MailSorting{MarkPromoRead: s.MarkPromoRead, ImportantOnly: s.ImportantOnly})
}

// PutMailSorting saves the two sorting settings.
func (m *Module) PutMailSorting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in api.MailSorting
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s := sortingSetting{MarkPromoRead: in.MarkPromoRead, ImportantOnly: in.ImportantOnly}
	if err := m.d.Settings.Set(ctx, sortingKey, s); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "mail.sorting", "", map[string]any{"markPromoRead": s.MarkPromoRead, "importantOnly": s.ImportantOnly}, nil)
	httpx.JSON(w, http.StatusOK, in)
}

// markPromoRead marks the new marketing mail read on the server, on the
// worker's own connection. A failure is logged and the mail stays unread.
func (m *Module) markPromoRead(ctx context.Context, c *imapclient.Client, acc db.MailAccount, msgs []db.MailMessage, sorted map[int64]api.MailTriage) {
	if len(sorted) == 0 || !m.sorting(ctx).MarkPromoRead {
		return
	}
	var uids, ids []int64
	for _, x := range msgs {
		if t, ok := sorted[x.ID]; ok && t.Category == api.Promo {
			uids = append(uids, x.Uid)
			ids = append(ids, x.ID)
		}
	}
	if len(uids) == 0 {
		return
	}
	if err := m.execOp(ctx, c, acc.ID, opSeen, uids); err != nil {
		m.d.Log.Warn("mail: mark promo read", "account", acc.ID, "err", err)
		return
	}
	for _, id := range ids {
		row, err := m.getMessage(ctx, id)
		if err != nil {
			continue
		}
		if err := m.q.SetMessageFlags(ctx, db.SetMessageFlagsParams{Unread: 0, Flagged: row.Flagged, ID: id}); err != nil {
			m.d.Log.Warn("mail: mark promo read", "account", acc.ID, "err", err)
		}
	}
	m.d.Bus.Publish("mail.updated", map[string]any{"accountId": acc.ID})
}

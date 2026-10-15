package mail

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

// B219: 批量操作。每个账号对邮箱服务器发一次请求，成功的账号再改本地。

const (
	maxBatchIDs = 500
	localChunk  = 500 // 一条 SQL 里最多放这么多个 uid
)

var batchKinds = map[api.MailBatchAction]opKind{
	api.Read: opSeen, api.Unread: opUnseen, api.Star: opFlag, api.Unstar: opUnflag, api.Delete: opDelete,
}

func (m *Module) BatchMailMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.MailBatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	kind, ok := batchKinds[body.Action]
	if !ok {
		httpx.Fail(w, r, httpx.Invalid("不认识这个操作"))
		return
	}
	ids := slices.Clone(body.Ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 || len(ids) > maxBatchIDs {
		httpx.Fail(w, r, httpx.Invalid(fmt.Sprintf("一次选 1 到 %d 封邮件", maxBatchIDs)))
		return
	}
	rows, err := m.q.ListMessagesByIDs(ctx, ids)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	byAccount := map[int64][]int64{} // 账号 → uid
	for _, row := range rows {
		byAccount[row.AccountID] = append(byAccount[row.AccountID], row.Uid)
	}
	out, err := m.applyByAccount(ctx, byAccount, kind)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) MarkAllMailRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.MailReadAll
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var accountIDs []int64
	if body.AccountId != nil {
		if _, err := m.getAccount(ctx, *body.AccountId); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		accountIDs = []int64{*body.AccountId}
	} else {
		accounts, err := m.q.ListAccounts(ctx)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		for _, a := range accounts {
			accountIDs = append(accountIDs, a.ID)
		}
	}
	byAccount := map[int64][]int64{}
	for _, id := range accountIDs {
		uids, err := m.q.UnreadUIDs(ctx, id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(uids) > 0 {
			byAccount[id] = uids
		}
	}
	out, err := m.applyByAccount(ctx, byAccount, opSeen)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// applyByAccount changes the messages on each account's server, then in the
// local copy for the accounts that worked. A failed account only adds a line
// to errors. The error returned is a local database failure.
func (m *Module) applyByAccount(ctx context.Context, byAccount map[int64][]int64, kind opKind) (api.MailBatchResult, error) {
	out := api.MailBatchResult{Errors: []string{}}
	accountIDs := make([]int64, 0, len(byAccount))
	for id := range byAccount {
		accountIDs = append(accountIDs, id)
	}
	slices.Sort(accountIDs)
	for _, id := range accountIDs {
		uids := byAccount[id]
		if err := m.apply(ctx, id, kind, uids); err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			out.Errors = append(out.Errors, m.accountName(ctx, id)+"："+err.Error())
			continue
		}
		if err := m.applyLocal(ctx, id, kind, uids); err != nil {
			return out, err
		}
		out.Done += len(uids)
		m.d.Bus.Publish("mail.updated", map[string]any{"accountId": id})
	}
	return out, nil
}

// applyLocal repeats the change in mail_messages.
func (m *Module) applyLocal(ctx context.Context, accountID int64, kind opKind, uids []int64) error {
	for chunk := range slices.Chunk(uids, localChunk) {
		var err error
		switch kind {
		case opSeen, opUnseen:
			err = m.q.SetUnreadByUIDs(ctx, db.SetUnreadByUIDsParams{Unread: boolInt(kind == opUnseen), AccountID: accountID, Uids: chunk})
		case opFlag, opUnflag:
			err = m.q.SetFlaggedByUIDs(ctx, db.SetFlaggedByUIDsParams{Flagged: boolInt(kind == opFlag), AccountID: accountID, Uids: chunk})
		case opDelete:
			for _, uid := range chunk {
				if err = m.q.DeleteMessageByUID(ctx, db.DeleteMessageByUIDParams{AccountID: accountID, Uid: uid}); err != nil {
					break
				}
				m.deleteFiles(ctx, fmt.Sprintf("%d/%d", accountID, uid))
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// accountName is the name shown in error lines.
func (m *Module) accountName(ctx context.Context, id int64) string {
	if a, err := m.q.GetAccount(ctx, id); err == nil {
		return a.Name
	}
	return fmt.Sprintf("邮箱 %d", id)
}

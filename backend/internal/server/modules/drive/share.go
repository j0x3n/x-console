package drive

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

const shareAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var errShareNotAllowed = httpx.NewError(400, "share_not_allowed", "这个文件不能分享")

func newShareToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	number := new(big.Int).SetBytes(raw)
	base := big.NewInt(62)
	var digits [22]byte
	for i := 21; i >= 0; i-- {
		var rem big.Int
		number.QuoRem(number, base, &rem)
		digits[i] = shareAlphabet[rem.Int64()]
	}
	return string(digits[:]), nil
}

func validShareCode(code string) bool {
	length := utf8.RuneCountInString(code)
	return utf8.ValidString(code) && length >= 4 && length <= 32
}

func shareExpiry(kind api.DriveShareInputExpiresIn) (*time.Time, bool) {
	days := 0
	switch kind {
	case api.N1d:
		days = 1
	case api.N7d:
		days = 7
	case api.N30d:
		days = 30
	case api.Never:
		return nil, true
	default:
		return nil, false
	}
	value := time.Now().UTC().AddDate(0, 0, days)
	return &value, true
}

func (m *Module) shareable(ctx context.Context, item db.DriveItem) bool {
	for depth := 0; depth < 64; depth++ {
		if item.Hidden != 0 || item.TrashedAt != nil {
			return false
		}
		if item.ParentID == nil {
			return true
		}
		var err error
		item, err = m.row(ctx, *item.ParentID)
		if err != nil {
			return false
		}
	}
	return false
}

func (m *Module) shareURL(r *http.Request, token string) string {
	base := strings.TrimRight(m.d.Config.PublicURL, "/")
	if base == "" {
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
			proto = forwarded
		}
		base = proto + "://" + r.Host
	}
	return base + "/s/" + token
}

func (m *Module) shareDTO(ctx context.Context, r *http.Request, share db.DriveShare, item db.DriveItem) (api.DriveShare, error) {
	out := api.DriveShare{Id: share.ID, ItemId: share.ItemID, ItemName: item.Name, IsDir: item.IsDir != 0,
		Token: share.Token, Url: m.shareURL(r, share.Token), ExpiresAt: share.ExpiresAt,
		Visits: int(share.Visits), Downloads: int(share.Downloads), CreatedAt: share.CreatedAt, LastAccessAt: share.LastAccessAt}
	if share.MaxDownloads != nil {
		value := int(*share.MaxDownloads)
		out.MaxDownloads = &value
	}
	if share.CodeSealed != nil {
		code, err := m.d.Secrets.Open(*share.CodeSealed)
		if err != nil {
			return out, err
		}
		out.Code = &code
	}
	out.Active = m.shareable(ctx, item) && (share.ExpiresAt == nil || share.ExpiresAt.After(time.Now().UTC())) && (share.MaxDownloads == nil || share.Downloads < *share.MaxDownloads)
	return out, nil
}

func (m *Module) CreateDriveShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	var body api.DriveShareInput
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	item, err := m.visibleRow(ctx, body.ItemId)
	if fail(w, r, err) {
		return
	}
	if !m.shareable(ctx, item) {
		httpx.Fail(w, r, errShareNotAllowed)
		return
	}
	expires, valid := shareExpiry(body.ExpiresIn)
	if !valid || (body.Code != nil && !validShareCode(*body.Code)) || (body.MaxDownloads != nil && *body.MaxDownloads < 1) {
		httpx.Fail(w, r, httpx.Invalid("分享参数不正确"))
		return
	}
	token, err := newShareToken()
	if fail(w, r, err) {
		return
	}
	var sealed *string
	if body.Code != nil {
		value, err := m.d.Secrets.Seal(*body.Code)
		if fail(w, r, err) {
			return
		}
		sealed = &value
	}
	var maxDownloads *int64
	if body.MaxDownloads != nil {
		value := int64(*body.MaxDownloads)
		maxDownloads = &value
	}
	var id int64
	now := time.Now().UTC()
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO drive_shares(item_id,token,code_sealed,expires_at,max_downloads,created_at) VALUES(?,?,?,?,?,?) RETURNING id", item.ID, token, sealed, expires, maxDownloads, now).Scan(&id)
	if fail(w, r, err) {
		return
	}
	share := db.DriveShare{ID: id, ItemID: item.ID, Token: token, CodeSealed: sealed, ExpiresAt: expires, MaxDownloads: maxDownloads, CreatedAt: now}
	dto, err := m.shareDTO(ctx, r, share, item)
	if fail(w, r, err) {
		return
	}
	m.audit(ctx, "drive.share.create", id, nil)
	m.d.Bus.Publish("drive_share.changed", map[string]any{"id": id, "itemId": item.ID})
	httpx.JSON(w, http.StatusCreated, dto)
}

func (m *Module) ListDriveShares(w http.ResponseWriter, r *http.Request, params api.ListDriveSharesParams) {
	ctx := r.Context()
	query := "SELECT id,item_id,token,code_sealed,expires_at,max_downloads,visits,downloads,created_at,last_access_at FROM drive_shares"
	var args []any
	if params.ItemId != nil {
		query += " WHERE item_id=?"
		args = append(args, *params.ItemId)
	}
	query += " ORDER BY created_at DESC,id DESC"
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	shares := []db.DriveShare{}
	for rows.Next() {
		var share db.DriveShare
		if err = rows.Scan(&share.ID, &share.ItemID, &share.Token, &share.CodeSealed, &share.ExpiresAt, &share.MaxDownloads, &share.Visits, &share.Downloads, &share.CreatedAt, &share.LastAccessAt); err != nil {
			break
		}
		shares = append(shares, share)
	}
	if err == nil {
		err = rows.Err()
	}
	if fail(w, r, err) {
		return
	}
	items := []api.DriveShare{}
	unlocked := auth.VaultUnlocked(ctx)
	for _, share := range shares {
		item, err := m.row(ctx, share.ItemID)
		if fail(w, r, err) {
			return
		}
		// Hiding a shared item keeps the share for a while (pruneShares).
		// Its name stays out of the list while the vault is locked.
		if item.Hidden != 0 && !unlocked {
			continue
		}
		dto, err := m.shareDTO(ctx, r, share, item)
		if fail(w, r, err) {
			return
		}
		items = append(items, dto)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) DeleteDriveShare(w http.ResponseWriter, r *http.Request, shareID int64) {
	ctx := r.Context()
	var itemID int64
	err := m.d.DB.QueryRowContext(ctx, "SELECT item_id FROM drive_shares WHERE id=?", shareID).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	_, err = m.d.DB.ExecContext(ctx, "DELETE FROM drive_shares WHERE id=?", shareID)
	if fail(w, r, err) {
		return
	}
	m.audit(ctx, "drive.share.delete", shareID, nil)
	m.d.Bus.Publish("drive_share.changed", map[string]any{"id": shareID, "itemId": itemID})
	httpx.NoContent(w)
}

func (m *Module) pruneShares(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -7)
	_, err := m.d.DB.ExecContext(ctx, `DELETE FROM drive_shares WHERE (expires_at IS NOT NULL AND expires_at<?) OR (max_downloads IS NOT NULL AND downloads>=max_downloads AND last_access_at<?) OR item_id IN (SELECT id FROM drive_items WHERE trashed_at<? OR (hidden=1 AND updated_at<?))`, cutoff, cutoff, cutoff, cutoff)
	return err
}

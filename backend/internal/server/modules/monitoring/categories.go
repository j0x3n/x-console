package monitoring

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

func toAPICategory(id int64, name string, builtin *string, position int64, count int64) api.SubscriptionCategoryItem {
	out := api.SubscriptionCategoryItem{Id: id, Name: name, Position: int(position), Count: int(count)}
	if builtin != nil {
		b := api.SubscriptionCategory(*builtin)
		out.Builtin = &b
	}
	return out
}

func validCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 30 {
		return "", httpx.Invalid("分类名称要 1 到 30 个字")
	}
	return name, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

var errCategoryExists = httpx.NewError(http.StatusConflict, "conflict", "已经有这个分类了")

// ListSubscriptionCategories is GET /subscription-categories.
func (m *Module) ListSubscriptionCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListSubscriptionCategories(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.SubscriptionCategoryItem, 0, len(rows))
	for _, c := range rows {
		out = append(out, toAPICategory(c.ID, c.Name, c.Builtin, c.Position, c.Count))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateSubscriptionCategory is POST /subscription-categories.
func (m *Module) CreateSubscriptionCategory(w http.ResponseWriter, r *http.Request) {
	var body api.SubscriptionCategoryInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	name, err := validCategoryName(name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	position := int64(0)
	if body.Position != nil {
		position = int64(*body.Position)
	} else if position, err = m.q.NextSubscriptionCategoryPosition(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.q.CreateSubscriptionCategory(ctx, db.CreateSubscriptionCategoryParams{Name: name, Position: position, CreatedAt: m.now()})
	if isUniqueViolation(err) {
		err = errCategoryExists
	}
	m.d.Audit.Record(ctx, "subscription.category.create", "", map[string]any{"name": name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPICategory(c.ID, c.Name, c.Builtin, c.Position, 0)
	m.d.Bus.Publish("subscription.category_changed", out)
	httpx.JSON(w, http.StatusCreated, out)
}

// UpdateSubscriptionCategory is PATCH /subscription-categories/{categoryId}.
func (m *Module) UpdateSubscriptionCategory(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.SubscriptionCategoryInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	cur, err := m.q.GetSubscriptionCategory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, position := cur.Name, cur.Position
	if body.Name != nil {
		if name, err = validCategoryName(*body.Name); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if body.Position != nil {
		position = int64(*body.Position)
	}
	c, err := m.q.UpdateSubscriptionCategory(ctx, db.UpdateSubscriptionCategoryParams{ID: id, Name: name, Position: position})
	if isUniqueViolation(err) {
		err = errCategoryExists
	}
	m.d.Audit.Record(ctx, "subscription.category.update", itoa(id), map[string]any{"name": name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	count := int64(0)
	if rows, err := m.q.ListSubscriptionCategories(ctx); err == nil {
		for _, row := range rows {
			if row.ID == id {
				count = row.Count
			}
		}
	}
	out := toAPICategory(c.ID, c.Name, c.Builtin, c.Position, count)
	m.d.Bus.Publish("subscription.category_changed", out)
	httpx.JSON(w, http.StatusOK, out)
}

// DeleteSubscriptionCategory is DELETE /subscription-categories/{categoryId}.
// Its subscriptions move to the built-in "other" category first.
func (m *Module) DeleteSubscriptionCategory(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	cur, err := m.q.GetSubscriptionCategory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if err == nil && cur.Builtin != nil && *cur.Builtin == categoryOther {
		err = httpx.Invalid("“其他”不能删除")
	}
	if err == nil {
		err = m.deleteCategory(r, id)
	}
	m.d.Audit.Record(ctx, "subscription.category.delete", itoa(id), map[string]any{"name": cur.Name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("subscription.category_changed", map[string]any{"id": id, "deleted": true})
	httpx.NoContent(w)
}

func (m *Module) deleteCategory(r *http.Request, id int64) error {
	ctx := r.Context()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	q := m.q.WithTx(tx)
	other := categoryOther
	fallback, err := q.GetSubscriptionCategoryByBuiltin(ctx, &other)
	if err != nil {
		return err
	}
	if err := q.MoveSubscriptionsToCategory(ctx, db.MoveSubscriptionsToCategoryParams{
		CategoryID: &fallback.ID, Category: categoryOther, UpdatedAt: m.now(), CategoryID_2: &id}); err != nil {
		return err
	}
	if _, err := q.DeleteSubscriptionCategory(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

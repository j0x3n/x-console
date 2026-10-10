package documents

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Built-in kinds. A kind the user adds is stored as customPrefix + name.
const (
	kindPassport = "passport"
	kindVisa     = "visa"

	customPrefix  = "c:"
	keyCustom     = "documents.custom_kinds"
	maxCustomKind = 30
	maxKindName   = 20
)

var builtinKinds = []string{"passport", "id_card", "driver_license", "visa", "contract", "insurance", "item", "other"}

// kindNames are the names used in notifications.
var kindNames = map[string]string{
	"passport": "护照", "id_card": "身份证", "driver_license": "驾照", "visa": "签证",
	"contract": "合同", "insurance": "保险", "item": "物品保修", "other": "证件",
}

// kindLabel is the name of a kind for people: the built-in name, or the name
// the user gave.
func kindLabel(kind string) string {
	if name, ok := strings.CutPrefix(kind, customPrefix); ok {
		return name
	}
	if n, ok := kindNames[kind]; ok {
		return n
	}
	return "证件"
}

func (m *Module) customKinds(ctx context.Context) ([]string, error) {
	var names []string
	if err := m.d.Settings.Get(ctx, keyCustom, &names); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return nil, err
	}
	return names, nil
}

// checkKind accepts a built-in kind or a custom kind that was added.
func (m *Module) checkKind(ctx context.Context, kind string) error {
	if slices.Contains(builtinKinds, kind) {
		return nil
	}
	if name, ok := strings.CutPrefix(kind, customPrefix); ok {
		names, err := m.customKinds(ctx)
		if err != nil {
			return err
		}
		if slices.Contains(names, name) {
			return nil
		}
	}
	return httpx.Invalid("不认识的类型")
}

func (m *Module) kindCounts(ctx context.Context) (map[string]int, error) {
	rows, err := m.q.ListDocuments(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Kind]++
	}
	return counts, nil
}

func (m *Module) customKindViews(ctx context.Context) ([]api.CustomDocumentKind, error) {
	names, err := m.customKinds(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := m.kindCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.CustomDocumentKind, 0, len(names))
	for _, n := range names {
		out = append(out, api.CustomDocumentKind{Key: customPrefix + n, Name: n, Count: counts[customPrefix+n]})
	}
	return out, nil
}

// ListDocumentKinds implements api.ServerInterface.
func (m *Module) ListDocumentKinds(w http.ResponseWriter, r *http.Request) {
	items, err := m.customKindViews(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateDocumentKind implements api.ServerInterface.
func (m *Module) CreateDocumentKind(w http.ResponseWriter, r *http.Request) {
	var body api.CreateDocumentKindJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	name := strings.TrimSpace(body.Name)
	err := m.addKind(ctx, name)
	m.d.Audit.Record(ctx, "document.kind.create", name, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("document.kind.changed", map[string]any{"name": name})
	httpx.JSON(w, http.StatusCreated, api.CustomDocumentKind{Key: customPrefix + name, Name: name})
}

func (m *Module) addKind(ctx context.Context, name string) error {
	if name == "" {
		return httpx.Invalid("类型名称不能为空")
	}
	if utf8.RuneCountInString(name) > maxKindName {
		return httpx.Invalid("类型名称最多 20 个字")
	}
	for _, n := range kindNames {
		if n == name {
			return httpx.Invalid("已经有这个类型了")
		}
	}
	names, err := m.customKinds(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(names, name) {
		return nil
	}
	if len(names) >= maxCustomKind {
		return httpx.Invalid("自己加的类型最多 30 个")
	}
	return m.d.Settings.Set(ctx, keyCustom, append(names, name))
}

// DeleteDocumentKind implements api.ServerInterface.
func (m *Module) DeleteDocumentKind(w http.ResponseWriter, r *http.Request, params api.DeleteDocumentKindParams) {
	ctx := r.Context()
	err := m.removeKind(ctx, params.Name)
	m.d.Audit.Record(ctx, "document.kind.delete", params.Name, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("document.kind.changed", map[string]any{"name": params.Name})
	httpx.NoContent(w)
}

func (m *Module) removeKind(ctx context.Context, name string) error {
	names, err := m.customKinds(ctx)
	if err != nil {
		return err
	}
	i := slices.Index(names, name)
	if i < 0 {
		return httpx.ErrNotFound
	}
	counts, err := m.kindCounts(ctx)
	if err != nil {
		return err
	}
	if n := counts[customPrefix+name]; n > 0 {
		return httpx.NewError(http.StatusConflict, "conflict", "还有档案在用这个类型，先把它们改成别的类型")
	}
	return m.d.Settings.Set(ctx, keyCustom, slices.Delete(slices.Clone(names), i, i+1))
}

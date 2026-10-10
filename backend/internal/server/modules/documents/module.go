// Package documents keeps the user's papers and things that expire: passports,
// IDs, visas, contracts, insurance and warranties (B115). It reminds before
// the expiry date. Scans live in the drive; this module only remembers which
// drive files belong to which entry. See docs/specs/B115.md.
package documents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/db"
)

const (
	dateLayout = "2006-01-02"

	maxName    = 100
	maxText    = 200
	maxNotes   = 20000
	maxFiles   = 20
	maxRemind  = 8
	maxDays    = 3650
	defaultMax = 90
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "documents.module"

// defaultRemind is when an entry reminds if the user did not choose.
var defaultRemind = []int{90, 30, 7}

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "documents" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the expiry check.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("documents.remind", time.Hour, m.remindAll)
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

// today is the civil date in the configured time zone, as a UTC midnight so
// that two dates subtract into whole days.
func (m *Module) today() time.Time {
	loc := m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	n := m.now().In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// ---------- view ----------

func parseRemind(s string) []int {
	out := []int{}
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func formatRemind(days []int) string {
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = strconv.Itoa(d)
	}
	return strings.Join(parts, ",")
}

// daysLeft is nil when the entry has no expiry date.
func daysLeft(expiresOn string, today time.Time) *int {
	if expiresOn == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, expiresOn)
	if err != nil {
		return nil
	}
	n := int(t.Sub(today).Hours() / 24)
	return &n
}

func status(left *int, remind []int) api.DocumentStatus {
	if left == nil {
		return api.None
	}
	if *left < 0 {
		return api.Expired
	}
	horizon := defaultMax
	if len(remind) > 0 {
		horizon = remind[0]
	}
	if *left <= horizon {
		return api.Soon
	}
	return api.Ok
}

func parseFiles(s string) []api.DocumentFile {
	out := []api.DocumentFile{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []api.DocumentFile{}
	}
	return out
}

func (m *Module) view(row db.Document, today time.Time) (api.Document, error) {
	number := ""
	if row.Number != "" {
		var err error
		if number, err = m.d.Secrets.Open(row.Number); err != nil {
			return api.Document{}, err
		}
	}
	remind := parseRemind(row.RemindDays)
	left := daysLeft(row.ExpiresOn, today)
	return api.Document{
		Id: row.ID, Kind: api.DocumentKind(row.Kind), Name: row.Name, Holder: row.Holder, Number: number,
		IssuedOn: row.IssuedOn, ExpiresOn: row.ExpiresOn, Price: row.Price, Currency: row.Currency, Serial: row.Serial,
		RemindDays: remind, Notes: row.Notes, Files: parseFiles(row.FilesJson),
		Status: status(left, remind), DaysLeft: left, Archived: row.ArchivedAt != nil,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// sortKey puts expired entries first, then the nearest expiry, then the ones
// with no date.
func sortKey(d api.Document) (int, int) {
	if d.DaysLeft == nil {
		return 1, 0
	}
	return 0, *d.DaysLeft
}

func (m *Module) list(ctx context.Context, kind *api.DocumentKind, q string, archived bool) ([]api.Document, api.DocumentSummary, error) {
	rows, err := m.q.ListDocuments(ctx)
	if err != nil {
		return nil, api.DocumentSummary{}, err
	}
	today := m.today()
	q = strings.ToLower(strings.TrimSpace(q))
	items := []api.Document{}
	var sum api.DocumentSummary
	for _, row := range rows {
		if row.ArchivedAt != nil && !archived {
			continue
		}
		v, err := m.view(row, today)
		if err != nil {
			return nil, sum, err
		}
		sum.Total++
		switch v.Status {
		case api.Expired:
			sum.Expired++
		case api.Soon:
			sum.Soon++
		case api.None:
			sum.None++
		}
		if kind != nil && v.Kind != *kind {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Name+"\n"+v.Holder+"\n"+v.Serial+"\n"+v.Notes), q) {
			continue
		}
		items = append(items, v)
	}
	sort.SliceStable(items, func(i, j int) bool {
		ai, bi := sortKey(items[i])
		aj, bj := sortKey(items[j])
		if ai != aj {
			return ai < aj
		}
		return bi < bj
	})
	return items, sum, nil
}

// ---------- validation ----------

type fields struct {
	kind                                    string
	name, holder, number, issuedOn, expires string
	price                                   *float64
	currency, serial, notes                 string
	remind                                  []int
}

func fieldsOf(row db.Document, number string) fields {
	return fields{
		kind: row.Kind, name: row.Name, holder: row.Holder, number: number, issuedOn: row.IssuedOn, expires: row.ExpiresOn,
		price: row.Price, currency: row.Currency, serial: row.Serial, notes: row.Notes, remind: parseRemind(row.RemindDays),
	}
}

func checkDate(label, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	t, err := time.Parse(dateLayout, v)
	if err != nil || t.Year() < 1900 || t.Year() > 2200 {
		return "", httpx.Invalid(label + "要写成 2026-10-31 这样的日期")
	}
	return v, nil
}

func checkText(label, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", httpx.Invalid(label + "太长了")
	}
	return v, nil
}

func checkRemind(days []int) ([]int, error) {
	out := []int{}
	for _, d := range days {
		if d < 1 || d > maxDays {
			return nil, httpx.Invalid("提醒天数要在 1 到 3650 之间")
		}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	if len(out) > maxRemind {
		return nil, httpx.Invalid("最多设 8 个提醒")
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}

// validate checks every field and returns them cleaned.
func validate(f fields) (fields, error) {
	var err error
	if f.name, err = checkText("名称", f.name, maxName); err != nil {
		return f, err
	}
	if f.name == "" {
		return f, httpx.Invalid("名称不能为空")
	}
	if f.holder, err = checkText("持有人", f.holder, maxText); err != nil {
		return f, err
	}
	if f.number, err = checkText("编号", f.number, maxText); err != nil {
		return f, err
	}
	if f.serial, err = checkText("序列号", f.serial, maxText); err != nil {
		return f, err
	}
	if f.currency, err = checkText("币种", f.currency, 10); err != nil {
		return f, err
	}
	if f.notes, err = checkText("备注", f.notes, maxNotes); err != nil {
		return f, err
	}
	if f.issuedOn, err = checkDate("签发或购买日期", f.issuedOn); err != nil {
		return f, err
	}
	if f.expires, err = checkDate("到期日", f.expires); err != nil {
		return f, err
	}
	if f.issuedOn != "" && f.expires != "" && f.expires < f.issuedOn {
		return f, httpx.Invalid("到期日不能早于签发或购买日期")
	}
	if f.price != nil && (*f.price < 0 || *f.price > 1e12) {
		return f, httpx.Invalid("价格不对")
	}
	if f.remind, err = checkRemind(f.remind); err != nil {
		return f, err
	}
	return f, nil
}

func pick(p *string, cur string) string {
	if p == nil {
		return cur
	}
	return *p
}

// publish sends an event without the number, which is not for event streams.
func (m *Module) publish(topic string, v api.Document) {
	v.Number = ""
	m.d.Bus.Publish(topic, v)
}

// ---------- handlers ----------

// ListDocuments implements api.ServerInterface.
func (m *Module) ListDocuments(w http.ResponseWriter, r *http.Request, params api.ListDocumentsParams) {
	if params.Kind != nil && m.checkKind(r.Context(), *params.Kind) != nil {
		httpx.Fail(w, r, httpx.Invalid("不认识的类型"))
		return
	}
	q := ""
	if params.Q != nil {
		q = *params.Q
	}
	items, sum, err := m.list(r.Context(), params.Kind, q, params.Archived != nil && *params.Archived)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "summary": sum})
}

// GetDocument implements api.ServerInterface.
func (m *Module) GetDocument(w http.ResponseWriter, r *http.Request, id api.DocumentId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.view(row, m.today())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) get(ctx context.Context, id int64) (db.Document, error) {
	row, err := m.q.GetDocument(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// CreateDocument implements api.ServerInterface.
func (m *Module) CreateDocument(w http.ResponseWriter, r *http.Request) {
	var body api.CreateDocumentJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.create(ctx, body)
	target := ""
	if err == nil {
		target = strconv.FormatInt(v.Id, 10)
	}
	m.d.Audit.Record(ctx, "document.create", target, map[string]any{"kind": string(body.Kind), "name": body.Name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.publish("document.created", v)
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) create(ctx context.Context, in api.DocumentInput) (api.Document, error) {
	f := fields{kind: string(in.Kind), name: in.Name, price: in.Price, remind: defaultRemind}
	f.holder, f.number, f.issuedOn, f.expires = pick(in.Holder, ""), pick(in.Number, ""), pick(in.IssuedOn, ""), pick(in.ExpiresOn, "")
	f.currency, f.serial, f.notes = pick(in.Currency, ""), pick(in.Serial, ""), pick(in.Notes, "")
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
	}
	f, err := validate(f)
	if err == nil {
		err = m.checkKind(ctx, f.kind)
	}
	if err != nil {
		return api.Document{}, err
	}
	sealed, err := m.seal(f.number)
	if err != nil {
		return api.Document{}, err
	}
	now := m.now().UTC()
	row, err := m.q.InsertDocument(ctx, db.InsertDocumentParams{
		Kind: f.kind, Name: f.name, Holder: f.holder, Number: sealed, IssuedOn: f.issuedOn, ExpiresOn: f.expires,
		Price: f.price, Currency: f.currency, Serial: f.serial, RemindDays: formatRemind(f.remind), Notes: f.notes,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return api.Document{}, err
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today())
}

func (m *Module) seal(number string) (string, error) {
	if number == "" {
		return "", nil
	}
	return m.d.Secrets.Seal(number)
}

// UpdateDocument implements api.ServerInterface.
func (m *Module) UpdateDocument(w http.ResponseWriter, r *http.Request, id api.DocumentId) {
	var body api.UpdateDocumentJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.update(ctx, id, body)
	m.d.Audit.Record(ctx, "document.update", strconv.FormatInt(id, 10), map[string]any{"name": pick(body.Name, "")}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.publish("document.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) update(ctx context.Context, id int64, in api.DocumentPatch) (api.Document, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Document{}, err
	}
	number := ""
	if cur.Number != "" {
		if number, err = m.d.Secrets.Open(cur.Number); err != nil {
			return api.Document{}, err
		}
	}
	f := fieldsOf(cur, number)
	if in.Kind != nil {
		f.kind = string(*in.Kind)
	}
	f.name, f.holder, f.number = pick(in.Name, f.name), pick(in.Holder, f.holder), pick(in.Number, f.number)
	f.issuedOn, f.expires = pick(in.IssuedOn, f.issuedOn), pick(in.ExpiresOn, f.expires)
	f.currency, f.serial, f.notes = pick(in.Currency, f.currency), pick(in.Serial, f.serial), pick(in.Notes, f.notes)
	if in.Price != nil {
		f.price = in.Price
	}
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
	}
	f, err = validate(f)
	if err == nil {
		err = m.checkKind(ctx, f.kind)
	}
	if err != nil {
		return api.Document{}, err
	}
	sealed := cur.Number
	if f.number != number {
		if sealed, err = m.seal(f.number); err != nil {
			return api.Document{}, err
		}
	}
	archivedAt := cur.ArchivedAt
	if in.Archived != nil {
		switch {
		case *in.Archived && archivedAt == nil:
			t := m.now().UTC()
			archivedAt = &t
		case !*in.Archived:
			archivedAt = nil
		}
	}
	row, err := m.q.UpdateDocument(ctx, db.UpdateDocumentParams{
		ID: id, Kind: f.kind, Name: f.name, Holder: f.holder, Number: sealed, IssuedOn: f.issuedOn, ExpiresOn: f.expires,
		Price: f.price, Currency: f.currency, Serial: f.serial, RemindDays: formatRemind(f.remind), Notes: f.notes,
		ArchivedAt: archivedAt, UpdatedAt: m.now().UTC(),
	})
	if err != nil {
		return api.Document{}, err
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today())
}

// DeleteDocument implements api.ServerInterface.
func (m *Module) DeleteDocument(w http.ResponseWriter, r *http.Request, id api.DocumentId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := m.q.DeleteDocument(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "document.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("document.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// AddDocumentFile implements api.ServerInterface.
func (m *Module) AddDocumentFile(w http.ResponseWriter, r *http.Request, id api.DocumentId) {
	var body api.AddDocumentFileJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.changeFiles(r.Context(), id, func(files []api.DocumentFile) ([]api.DocumentFile, error) {
		name, err := checkText("文件名", body.Name, maxText)
		if err != nil {
			return nil, err
		}
		if body.DriveId <= 0 || name == "" {
			return nil, httpx.Invalid("文件不对")
		}
		for i := range files {
			if files[i].DriveId == body.DriveId {
				files[i].Name = name
				return files, nil
			}
		}
		if len(files) >= maxFiles {
			return nil, httpx.Invalid("一条档案最多放 20 个文件")
		}
		return append(files, api.DocumentFile{DriveId: body.DriveId, Name: name}), nil
	})
	m.d.Audit.Record(r.Context(), "document.file.add", strconv.FormatInt(id, 10), map[string]any{"driveId": body.DriveId}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.publish("document.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

// RemoveDocumentFile implements api.ServerInterface.
func (m *Module) RemoveDocumentFile(w http.ResponseWriter, r *http.Request, id api.DocumentId, driveID int64) {
	v, err := m.changeFiles(r.Context(), id, func(files []api.DocumentFile) ([]api.DocumentFile, error) {
		out := files[:0]
		for _, f := range files {
			if f.DriveId != driveID {
				out = append(out, f)
			}
		}
		return out, nil
	})
	m.d.Audit.Record(r.Context(), "document.file.remove", strconv.FormatInt(id, 10), map[string]any{"driveId": driveID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.publish("document.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) changeFiles(ctx context.Context, id int64, fn func([]api.DocumentFile) ([]api.DocumentFile, error)) (api.Document, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Document{}, err
	}
	files, err := fn(parseFiles(cur.FilesJson))
	if err != nil {
		return api.Document{}, err
	}
	raw, err := json.Marshal(files)
	if err != nil {
		return api.Document{}, err
	}
	row, err := m.q.SetDocumentFiles(ctx, db.SetDocumentFilesParams{FilesJson: string(raw), UpdatedAt: m.now().UTC(), ID: id})
	if err != nil {
		return api.Document{}, err
	}
	return m.view(row, m.today())
}

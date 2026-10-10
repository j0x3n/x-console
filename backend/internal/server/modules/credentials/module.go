// Package credentials is the ledger of the user's API keys, access tokens and
// SSH keys (B120). It keeps facts about them: which platform, where each one
// is used, when it expires and when it was last rotated. It reminds before the
// expiry date and when a rotation is overdue. It never stores a secret and
// refuses text that looks like one. See docs/specs/B120.md.
package credentials

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
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/db"
)

const (
	dateLayout = "2006-01-02"

	maxName     = 100
	maxLine     = 200
	maxHint     = 16
	maxNotes    = 20000
	maxUsedBy   = 20
	maxUsedItem = 100
	maxRemind   = 8
	maxDays     = 3650
	// soonDefault is how far ahead an entry counts as "soon" when it has no
	// reminder days of its own.
	soonDefault = 30
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "credentials.module"

// defaultRemind is when an entry reminds if the user did not choose.
var defaultRemind = []int{30, 7}

var errSecret = httpx.Invalid("这看起来是密钥本身，台账里只记信息，不要填密钥")

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
func (m *Module) Name() string { return "credentials" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the reminder check.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("credentials.remind", time.Hour, m.remindAll)
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

func parseUsedBy(s string) []string {
	out := []string{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// daysUntil is nil when date is empty or not a date.
func daysUntil(date string, today time.Time) *int {
	if date == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return nil
	}
	n := int(t.Sub(today).Hours() / 24)
	return &n
}

// rotateDueDate is the day the next rotation is due: the last rotation (or the
// creation date) plus the period. Empty when there is no period or no base.
func rotateDueDate(row db.Credential) string {
	if row.RotateEveryDays <= 0 {
		return ""
	}
	base := row.RotatedOn
	if base == "" {
		base = row.CreatedOn
	}
	t, err := time.Parse(dateLayout, base)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, int(row.RotateEveryDays)).Format(dateLayout)
}

func status(expiresIn, rotateDueIn *int, remind []int) api.CredentialStatus {
	if expiresIn != nil {
		if *expiresIn < 0 {
			return api.Expired
		}
		horizon := soonDefault
		if len(remind) > 0 {
			horizon = remind[0]
		}
		if *expiresIn <= horizon {
			return api.Soon
		}
	}
	if rotateDueIn != nil && *rotateDueIn <= 0 {
		return api.Stale
	}
	if expiresIn == nil && rotateDueIn == nil {
		return api.None
	}
	return api.Ok
}

func (m *Module) view(row db.Credential, today time.Time) api.Credential {
	remind := parseRemind(row.RemindDays)
	expiresIn := daysUntil(row.ExpiresOn, today)
	rotateDueIn := daysUntil(rotateDueDate(row), today)
	return api.Credential{
		Id: row.ID, Kind: api.CredentialKind(row.Kind), Name: row.Name, Platform: row.Platform, Account: row.Account,
		UsedBy: parseUsedBy(row.UsedBy), Scopes: row.Scopes, Hint: row.Hint,
		CreatedOn: row.CreatedOn, RotatedOn: row.RotatedOn, ExpiresOn: row.ExpiresOn,
		RotateEveryDays: int(row.RotateEveryDays), RemindDays: remind, Notes: row.Notes,
		Status: status(expiresIn, rotateDueIn, remind), ExpiresIn: expiresIn, RotateDueIn: rotateDueIn,
		Archived: row.ArchivedAt != nil, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// sortKey puts what is overdue first, then the nearest date. The date is the
// closer one of the expiry and the rotation. Entries with neither come last.
func sortKey(c api.Credential) (int, int) {
	switch {
	case c.ExpiresIn != nil && c.RotateDueIn != nil:
		return 0, min(*c.ExpiresIn, *c.RotateDueIn)
	case c.ExpiresIn != nil:
		return 0, *c.ExpiresIn
	case c.RotateDueIn != nil:
		return 0, *c.RotateDueIn
	}
	return 1, 0
}

func (m *Module) list(ctx context.Context, kind *api.CredentialKind, q string, archived bool) ([]api.Credential, api.CredentialSummary, error) {
	rows, err := m.q.ListCredentials(ctx)
	if err != nil {
		return nil, api.CredentialSummary{}, err
	}
	today := m.today()
	q = strings.ToLower(strings.TrimSpace(q))
	items := []api.Credential{}
	var sum api.CredentialSummary
	for _, row := range rows {
		if row.ArchivedAt != nil && !archived {
			continue
		}
		v := m.view(row, today)
		sum.Total++
		switch v.Status {
		case api.Expired:
			sum.Expired++
		case api.Soon:
			sum.Soon++
		case api.Stale:
			sum.Stale++
		case api.None:
			sum.None++
		}
		if kind != nil && v.Kind != *kind {
			continue
		}
		hay := strings.ToLower(v.Name + "\n" + v.Platform + "\n" + v.Account + "\n" + strings.Join(v.UsedBy, "\n") + "\n" + v.Scopes + "\n" + v.Notes)
		if q != "" && !strings.Contains(hay, q) {
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
	kind                                         string
	name, platform, account, scopes, hint, notes string
	usedBy                                       []string
	createdOn, rotatedOn, expires                string
	rotateEvery                                  int
	remind                                       []int
}

func fieldsOf(row db.Credential) fields {
	return fields{
		kind: row.Kind, name: row.Name, platform: row.Platform, account: row.Account, scopes: row.Scopes, hint: row.Hint,
		notes: row.Notes, usedBy: parseUsedBy(row.UsedBy), createdOn: row.CreatedOn, rotatedOn: row.RotatedOn,
		expires: row.ExpiresOn, rotateEvery: int(row.RotateEveryDays), remind: parseRemind(row.RemindDays),
	}
}

func checkDate(label, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	t, err := time.Parse(dateLayout, v)
	if err != nil || t.Year() < 1990 || t.Year() > 2200 {
		return "", httpx.Invalid(label + "要写成 2026-10-31 这样的日期")
	}
	return v, nil
}

func checkText(label, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", httpx.Invalid(label + "太长了")
	}
	if looksLikeSecret(v) {
		return "", errSecret
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

func checkUsedBy(items []string) ([]string, error) {
	out := []string{}
	for _, item := range items {
		item, err := checkText("用在哪里的每一项", item, maxUsedItem)
		if err != nil {
			return nil, err
		}
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	if len(out) > maxUsedBy {
		return nil, httpx.Invalid("用在哪里最多 20 项")
	}
	return out, nil
}

// validate checks every field and returns them cleaned.
func validate(f fields) (fields, error) {
	var err error
	if !api.CredentialKind(f.kind).Valid() {
		return f, httpx.Invalid("不认识的类型")
	}
	if f.name, err = checkText("名称", f.name, maxName); err != nil {
		return f, err
	}
	if f.name == "" {
		return f, httpx.Invalid("名称不能为空")
	}
	if f.platform, err = checkText("平台", f.platform, maxName); err != nil {
		return f, err
	}
	if f.account, err = checkText("账号", f.account, maxLine); err != nil {
		return f, err
	}
	if f.scopes, err = checkText("权限范围", f.scopes, maxLine); err != nil {
		return f, err
	}
	if f.hint, err = checkText("识别尾号", f.hint, maxHint); err != nil {
		return f, err
	}
	if f.notes, err = checkText("备注", f.notes, maxNotes); err != nil {
		return f, err
	}
	if f.usedBy, err = checkUsedBy(f.usedBy); err != nil {
		return f, err
	}
	if f.createdOn, err = checkDate("创建日期", f.createdOn); err != nil {
		return f, err
	}
	if f.rotatedOn, err = checkDate("上次更换日期", f.rotatedOn); err != nil {
		return f, err
	}
	if f.expires, err = checkDate("到期日", f.expires); err != nil {
		return f, err
	}
	if f.createdOn != "" && f.expires != "" && f.expires < f.createdOn {
		return f, httpx.Invalid("到期日不能早于创建日期")
	}
	if f.createdOn != "" && f.rotatedOn != "" && f.rotatedOn < f.createdOn {
		return f, httpx.Invalid("上次更换日期不能早于创建日期")
	}
	if f.rotateEvery < 0 || f.rotateEvery > maxDays {
		return f, httpx.Invalid("更换周期要在 0 到 3650 天之间")
	}
	if f.rotateEvery > 0 && f.createdOn == "" && f.rotatedOn == "" {
		return f, httpx.Invalid("设了更换周期，就要填创建日期或上次更换日期")
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

// ---------- handlers ----------

// ListCredentials implements api.ServerInterface.
func (m *Module) ListCredentials(w http.ResponseWriter, r *http.Request, params api.ListCredentialsParams) {
	if params.Kind != nil && !params.Kind.Valid() {
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

func (m *Module) get(ctx context.Context, id int64) (db.Credential, error) {
	row, err := m.q.GetCredential(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// GetCredential implements api.ServerInterface.
func (m *Module) GetCredential(w http.ResponseWriter, r *http.Request, id api.CredentialId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.view(row, m.today()))
}

// CreateCredential implements api.ServerInterface.
func (m *Module) CreateCredential(w http.ResponseWriter, r *http.Request) {
	var body api.CreateCredentialJSONRequestBody
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
	m.d.Audit.Record(ctx, "credential.create", target, map[string]any{"kind": string(body.Kind), "name": body.Name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("credential.created", v)
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) create(ctx context.Context, in api.CredentialInput) (api.Credential, error) {
	f := fields{kind: string(in.Kind), name: in.Name, remind: defaultRemind}
	f.platform, f.account, f.scopes, f.hint, f.notes = pick(in.Platform, ""), pick(in.Account, ""), pick(in.Scopes, ""), pick(in.Hint, ""), pick(in.Notes, "")
	f.createdOn, f.rotatedOn, f.expires = pick(in.CreatedOn, ""), pick(in.RotatedOn, ""), pick(in.ExpiresOn, "")
	if in.UsedBy != nil {
		f.usedBy = *in.UsedBy
	}
	if in.RotateEveryDays != nil {
		f.rotateEvery = *in.RotateEveryDays
	}
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
	}
	f, err := validate(f)
	if err != nil {
		return api.Credential{}, err
	}
	usedBy, _ := json.Marshal(f.usedBy)
	now := m.now().UTC()
	row, err := m.q.InsertCredential(ctx, db.InsertCredentialParams{
		Kind: f.kind, Name: f.name, Platform: f.platform, Account: f.account, UsedBy: string(usedBy), Scopes: f.scopes, Hint: f.hint,
		CreatedOn: f.createdOn, RotatedOn: f.rotatedOn, ExpiresOn: f.expires, RotateEveryDays: int64(f.rotateEvery),
		RemindDays: formatRemind(f.remind), Notes: f.notes, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return api.Credential{}, err
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today()), nil
}

// UpdateCredential implements api.ServerInterface.
func (m *Module) UpdateCredential(w http.ResponseWriter, r *http.Request, id api.CredentialId) {
	var body api.UpdateCredentialJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.update(ctx, id, body)
	m.d.Audit.Record(ctx, "credential.update", strconv.FormatInt(id, 10), map[string]any{"name": pick(body.Name, "")}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("credential.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) update(ctx context.Context, id int64, in api.CredentialPatch) (api.Credential, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Credential{}, err
	}
	f := fieldsOf(cur)
	if in.Kind != nil {
		f.kind = string(*in.Kind)
	}
	f.name, f.platform, f.account = pick(in.Name, f.name), pick(in.Platform, f.platform), pick(in.Account, f.account)
	f.scopes, f.hint, f.notes = pick(in.Scopes, f.scopes), pick(in.Hint, f.hint), pick(in.Notes, f.notes)
	f.createdOn, f.rotatedOn, f.expires = pick(in.CreatedOn, f.createdOn), pick(in.RotatedOn, f.rotatedOn), pick(in.ExpiresOn, f.expires)
	if in.UsedBy != nil {
		f.usedBy = *in.UsedBy
	}
	if in.RotateEveryDays != nil {
		f.rotateEvery = *in.RotateEveryDays
	}
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
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
	return m.save(ctx, cur, f, archivedAt)
}

func (m *Module) save(ctx context.Context, cur db.Credential, f fields, archivedAt *time.Time) (api.Credential, error) {
	f, err := validate(f)
	if err != nil {
		return api.Credential{}, err
	}
	usedBy, _ := json.Marshal(f.usedBy)
	row, err := m.q.UpdateCredential(ctx, db.UpdateCredentialParams{
		ID: cur.ID, Kind: f.kind, Name: f.name, Platform: f.platform, Account: f.account, UsedBy: string(usedBy), Scopes: f.scopes, Hint: f.hint,
		CreatedOn: f.createdOn, RotatedOn: f.rotatedOn, ExpiresOn: f.expires, RotateEveryDays: int64(f.rotateEvery),
		RemindDays: formatRemind(f.remind), Notes: f.notes, ArchivedAt: archivedAt, UpdatedAt: m.now().UTC(),
	})
	if err != nil {
		return api.Credential{}, err
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today()), nil
}

// RotateCredential implements api.ServerInterface.
func (m *Module) RotateCredential(w http.ResponseWriter, r *http.Request, id api.CredentialId) {
	var body api.RotateCredentialJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.rotate(ctx, id, body)
	m.d.Audit.Record(ctx, "credential.rotate", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("credential.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) rotate(ctx context.Context, id int64, in api.CredentialRotate) (api.Credential, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Credential{}, err
	}
	f := fieldsOf(cur)
	today := m.today().Format(dateLayout)
	f.rotatedOn = today
	if in.RotatedOn != nil && strings.TrimSpace(*in.RotatedOn) != "" {
		f.rotatedOn = strings.TrimSpace(*in.RotatedOn)
		if _, err := checkDate("更换日期", f.rotatedOn); err != nil {
			return api.Credential{}, err
		}
		if f.rotatedOn > today {
			return api.Credential{}, httpx.Invalid("更换日期不能晚于今天")
		}
	}
	f.expires = pick(in.ExpiresOn, f.expires)
	f.hint = pick(in.Hint, f.hint)
	return m.save(ctx, cur, f, cur.ArchivedAt)
}

// DeleteCredential implements api.ServerInterface.
func (m *Module) DeleteCredential(w http.ResponseWriter, r *http.Request, id api.CredentialId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := m.q.DeleteCredential(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "credential.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("credential.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

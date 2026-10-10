// Package contacts keeps the people the user wants to stay in touch with
// (B122): their birthdays and other dates that come back every year, and when
// they were last in touch. It reminds before a date and when a contact has not
// been in touch for longer than the user chose. Besides the name it keeps
// phone numbers and e-mail addresses, which come from a vCard file or from an
// iCloud address book (B140). See docs/specs/B122.md and docs/specs/B140.md.
package contacts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/db"
)

const (
	dateLayout = "2006-01-02"

	maxName    = 100
	maxLabel   = 50
	maxNotes   = 20000
	maxEvents  = 20
	maxRemind  = 8
	maxDays    = 3650
	maxRemindD = 365
	maxPhones  = 10
	maxEmails  = 10
	maxContact = 100
	// soonDefault is how far ahead a date counts as "soon" when the contact
	// has no reminder days of its own.
	soonDefault = 7
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "contacts.module"

// defaultRemind is when a date reminds if the user did not choose. The day
// itself always reminds.
var defaultRemind = []int{7, 1}

var kindLabels = map[api.ContactEventKind]string{
	api.ContactEventKindBirthday:    "生日",
	api.ContactEventKindAnniversary: "纪念日",
	api.ContactEventKindOther:       "日子",
}

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time
	// httpClient talks to the address book server; nil means the default.
	httpClient *http.Client
	syncing    atomic.Bool
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
func (m *Module) Name() string { return "contacts" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the reminder check.
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("contacts.remind", time.Hour, m.remindAll)
	m.d.Scheduler.Every("contacts.sync", time.Hour, m.syncJob) // 每小时看一眼，到 6 小时才真的同步
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

// ---------- dates ----------

// event is a date as it is stored.
type event struct {
	ID    string               `json:"id"`
	Kind  api.ContactEventKind `json:"kind"`
	Label string               `json:"label"`
	Date  string               `json:"date"`
}

func parseEvents(s string) []event {
	out := []event{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []event{}
	}
	return out
}

// splitDate reads "MM-DD" or "YYYY-MM-DD". year is 0 when it was not given.
func splitDate(v string) (year, month, day int, ok bool) {
	parts := strings.Split(v, "-")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || len(p) < 2 && i > 0 || len(p) > 4 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	switch len(nums) {
	case 2:
		month, day = nums[0], nums[1]
	case 3:
		if len(parts[0]) != 4 {
			return 0, 0, 0, false
		}
		year, month, day = nums[0], nums[1], nums[2]
	default:
		return 0, 0, 0, false
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, 0, 0, false
	}
	// 2000 is a leap year, so it accepts 02-29 for the dates without a year
	check := year
	if check == 0 {
		check = 2000
	}
	t := time.Date(check, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Month() != time.Month(month) || t.Day() != day {
		return 0, 0, 0, false
	}
	if year != 0 && (year < 1900 || year > 2200) {
		return 0, 0, 0, false
	}
	return year, month, day, true
}

// occurrence is the date the event falls on in the given year. 02-29 falls on
// 02-28 in a year that has no 29th.
func occurrence(month, day, year int) time.Time {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Month() != time.Month(month) {
		t = time.Date(year, time.Month(month), 28, 0, 0, 0, 0, time.UTC)
	}
	return t
}

// next is the first date on or after today the event falls on.
func next(e event, today time.Time) (on time.Time, in int, years *int) {
	year, month, day, _ := splitDate(e.Date)
	on = occurrence(month, day, today.Year())
	if on.Before(today) {
		on = occurrence(month, day, today.Year()+1)
	}
	in = int(on.Sub(today).Hours() / 24)
	if year != 0 {
		y := on.Year() - year
		years = &y
	}
	return on, in, years
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

// parseList reads a JSON array of strings.
func parseList(s string) []string {
	out := []string{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func formatList(list []string) string {
	if list == nil {
		list = []string{}
	}
	raw, _ := json.Marshal(list)
	return string(raw)
}

// contactBase is the day the "not in touch" count starts from.
func contactBase(row db.Contact, loc *time.Location) string {
	if row.LastContactOn != "" {
		return row.LastContactOn
	}
	return row.CreatedAt.In(loc).Format(dateLayout)
}

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

func (m *Module) view(row db.Contact, today time.Time) api.Contact {
	remind := parseRemind(row.RemindDays)
	v := api.Contact{
		Id: row.ID, Name: row.Name, Group: api.ContactGroup(row.GroupKind), Events: []api.ContactEvent{},
		LastContactOn: row.LastContactOn, ContactEveryDays: int(row.ContactEveryDays), RemindDays: remind, Notes: row.Notes,
		Phones: parseList(row.Phones), Emails: parseList(row.Emails), Source: row.Source,
		Archived: row.ArchivedAt != nil, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	var nextIn *int
	for _, e := range parseEvents(row.Events) {
		on, in, years := next(e, today)
		v.Events = append(v.Events, api.ContactEvent{
			Id: e.ID, Kind: e.Kind, Label: e.Label, Date: e.Date, NextOn: on.Format(dateLayout), NextIn: in, Years: years,
		})
		if nextIn == nil || in < *nextIn {
			n, label := in, e.Label
			nextIn, v.NextEventLabel = &n, &label
		}
	}
	v.NextEventIn = nextIn
	if t, err := time.Parse(dateLayout, row.LastContactOn); err == nil {
		n := int(today.Sub(t).Hours() / 24)
		v.SinceContact = &n
	}
	if row.ContactEveryDays > 0 {
		if base, err := time.Parse(dateLayout, contactBase(row, m.loc())); err == nil {
			due := int(base.AddDate(0, 0, int(row.ContactEveryDays)).Sub(today).Hours() / 24)
			v.ContactDueIn = &due
		}
	}
	horizon := soonDefault
	if len(remind) > 0 {
		horizon = remind[0]
	}
	switch {
	case nextIn != nil && *nextIn <= horizon:
		v.Status = api.Soon
	case v.ContactDueIn != nil && *v.ContactDueIn <= 0:
		v.Status = api.Overdue
	case nextIn == nil && v.ContactDueIn == nil:
		v.Status = api.None
	default:
		v.Status = api.Ok
	}
	return v
}

// rank orders the list: near dates, then people to get in touch with, then the
// rest, then people with nothing to track.
func rank(c api.Contact) (group, key int) {
	switch c.Status {
	case api.Soon:
		return 0, *c.NextEventIn
	case api.Overdue:
		return 1, *c.ContactDueIn
	case api.Ok:
		if c.NextEventIn != nil {
			return 2, *c.NextEventIn
		}
		return 2, 100000
	}
	return 3, 0
}

func (m *Module) list(ctx context.Context, group *api.ContactGroup, q string, archived bool, within *int) ([]api.Contact, api.ContactSummary, error) {
	rows, err := m.q.ListContacts(ctx)
	if err != nil {
		return nil, api.ContactSummary{}, err
	}
	today := m.today()
	q = strings.ToLower(strings.TrimSpace(q))
	items := []api.Contact{}
	var sum api.ContactSummary
	for _, row := range rows {
		if row.ArchivedAt != nil && !archived {
			continue
		}
		v := m.view(row, today)
		sum.Total++
		switch v.Status {
		case api.Soon:
			sum.Soon++
		case api.Overdue:
			sum.Overdue++
		}
		if group != nil && v.Group != *group {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Name+"\n"+v.Notes), q) {
			continue
		}
		if within != nil && (v.NextEventIn == nil || *v.NextEventIn > *within) {
			continue
		}
		items = append(items, v)
	}
	sort.SliceStable(items, func(i, j int) bool {
		gi, ki := rank(items[i])
		gj, kj := rank(items[j])
		if gi != gj {
			return gi < gj
		}
		if ki != kj {
			return ki < kj
		}
		return items[i].Name < items[j].Name
	})
	return items, sum, nil
}

// ---------- validation ----------

type fields struct {
	name, group, notes, lastContact string
	events                          []event
	every                           int
	remind                          []int
	phones, emails                  []string
	source, externalID              string
}

func fieldsOf(row db.Contact) fields {
	return fields{
		name: row.Name, group: row.GroupKind, notes: row.Notes, lastContact: row.LastContactOn,
		events: parseEvents(row.Events), every: int(row.ContactEveryDays), remind: parseRemind(row.RemindDays),
		phones: parseList(row.Phones), emails: parseList(row.Emails), source: row.Source, externalID: row.ExternalID,
	}
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func checkText(label, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", httpx.Invalid(label + "太长了")
	}
	return v, nil
}

// validate checks every field and returns them cleaned.
func (m *Module) validate(f fields) (fields, error) {
	var err error
	if f.name, err = checkText("名称", f.name, maxName); err != nil {
		return f, err
	}
	if f.name == "" {
		return f, httpx.Invalid("名称不能为空")
	}
	if !api.ContactGroup(f.group).Valid() {
		return f, httpx.Invalid("不认识的分组")
	}
	if f.notes, err = checkText("备注", f.notes, maxNotes); err != nil {
		return f, err
	}
	f.lastContact = strings.TrimSpace(f.lastContact)
	if f.lastContact != "" {
		t, err := time.Parse(dateLayout, f.lastContact)
		if err != nil || t.Year() < 1990 {
			return f, httpx.Invalid("上次联系要写成 2026-10-31 这样的日期")
		}
		if t.After(m.today()) {
			return f, httpx.Invalid("上次联系不能晚于今天")
		}
	}
	if f.every < 0 || f.every > maxDays {
		return f, httpx.Invalid("联系周期要在 0 到 3650 天之间")
	}
	if len(f.events) > maxEvents {
		return f, httpx.Invalid("重要日期最多 20 个")
	}
	births, seen := 0, map[string]bool{}
	for i := range f.events {
		e := &f.events[i]
		if !(api.ContactEventKind(e.Kind)).Valid() {
			return f, httpx.Invalid("不认识的日期类型")
		}
		if e.Label, err = checkText("日期的名称", e.Label, maxLabel); err != nil {
			return f, err
		}
		if e.Label == "" {
			e.Label = kindLabels[e.Kind]
		}
		e.Date = strings.TrimSpace(e.Date)
		if _, _, _, ok := splitDate(e.Date); !ok {
			return f, httpx.Invalid("日期要写成 08-15 或 1990-08-15 这样")
		}
		if e.Kind == api.ContactEventKindBirthday {
			births++
		}
		if e.ID == "" || seen[e.ID] {
			e.ID = newID()
		}
		seen[e.ID] = true
	}
	if births > 1 {
		return f, httpx.Invalid("一个联系人只能有一个生日")
	}
	if f.phones, err = cleanList("电话", f.phones, maxPhones); err != nil {
		return f, err
	}
	if f.emails, err = cleanList("邮箱", f.emails, maxEmails); err != nil {
		return f, err
	}
	days := []int{}
	for _, d := range f.remind {
		if d < 1 || d > maxRemindD {
			return f, httpx.Invalid("提醒天数要在 1 到 365 之间")
		}
		if !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	if len(days) > maxRemind {
		return f, httpx.Invalid("最多设 8 个提醒")
	}
	sort.Sort(sort.Reverse(sort.IntSlice(days)))
	f.remind = days
	return f, nil
}

// cleanList trims, drops empty and repeated items and checks the count and length.
func cleanList(label string, list []string, maxCount int) ([]string, error) {
	out := []string{}
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(out, v) {
			continue
		}
		if utf8.RuneCountInString(v) > maxContact {
			return nil, httpx.Invalid(label + "太长了")
		}
		out = append(out, v)
	}
	if len(out) > maxCount {
		return nil, httpx.Invalid(label + "最多 " + strconv.Itoa(maxCount) + " 个")
	}
	return out, nil
}

func pick(p *string, cur string) string {
	if p == nil {
		return cur
	}
	return *p
}

func eventsOf(in *[]api.ContactEventInput, cur []event) []event {
	if in == nil {
		return cur
	}
	out := make([]event, 0, len(*in))
	for _, e := range *in {
		out = append(out, event{ID: pick(e.Id, ""), Kind: e.Kind, Label: pick(e.Label, ""), Date: e.Date})
	}
	return out
}

// ---------- handlers ----------

// ListContacts implements api.ServerInterface.
func (m *Module) ListContacts(w http.ResponseWriter, r *http.Request, params api.ListContactsParams) {
	if params.Group != nil && !params.Group.Valid() {
		httpx.Fail(w, r, httpx.Invalid("不认识的分组"))
		return
	}
	q := ""
	if params.Q != nil {
		q = *params.Q
	}
	items, sum, err := m.list(r.Context(), params.Group, q, params.Archived != nil && *params.Archived, nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "summary": sum})
}

func (m *Module) get(ctx context.Context, id int64) (db.Contact, error) {
	row, err := m.q.GetContact(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// GetContact implements api.ServerInterface.
func (m *Module) GetContact(w http.ResponseWriter, r *http.Request, id api.ContactId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.view(row, m.today()))
}

// CreateContact implements api.ServerInterface.
func (m *Module) CreateContact(w http.ResponseWriter, r *http.Request) {
	var body api.CreateContactJSONRequestBody
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
	m.d.Audit.Record(ctx, "contact.create", target, map[string]any{"name": body.Name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("contact.created", v)
	httpx.JSON(w, http.StatusCreated, v)
}

func (m *Module) create(ctx context.Context, in api.ContactInput) (api.Contact, error) {
	f := fields{name: in.Name, group: string(api.ContactGroupOther), remind: defaultRemind}
	if in.Group != nil {
		f.group = string(*in.Group)
	}
	f.notes, f.lastContact = pick(in.Notes, ""), pick(in.LastContactOn, "")
	f.events = eventsOf(in.Events, nil)
	if in.ContactEveryDays != nil {
		f.every = *in.ContactEveryDays
	}
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
	}
	if in.Phones != nil {
		f.phones = *in.Phones
	}
	if in.Emails != nil {
		f.emails = *in.Emails
	}
	return m.insert(ctx, f)
}

// insert validates f and stores it as a new contact.
func (m *Module) insert(ctx context.Context, f fields) (api.Contact, error) {
	f, err := m.validate(f)
	if err != nil {
		return api.Contact{}, err
	}
	events, _ := json.Marshal(f.events)
	now := m.now().UTC()
	row, err := m.q.InsertContact(ctx, db.InsertContactParams{
		Name: f.name, GroupKind: f.group, Events: string(events), LastContactOn: f.lastContact,
		ContactEveryDays: int64(f.every), RemindDays: formatRemind(f.remind), Notes: f.notes,
		Phones: formatList(f.phones), Emails: formatList(f.emails), Source: f.source, ExternalID: f.externalID,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return api.Contact{}, err
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today()), nil
}

// UpdateContact implements api.ServerInterface.
func (m *Module) UpdateContact(w http.ResponseWriter, r *http.Request, id api.ContactId) {
	var body api.UpdateContactJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	v, err := m.update(ctx, id, body)
	m.d.Audit.Record(ctx, "contact.update", strconv.FormatInt(id, 10), map[string]any{"name": pick(body.Name, "")}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("contact.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) update(ctx context.Context, id int64, in api.ContactPatch) (api.Contact, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Contact{}, err
	}
	f := fieldsOf(cur)
	f.name, f.notes, f.lastContact = pick(in.Name, f.name), pick(in.Notes, f.notes), pick(in.LastContactOn, f.lastContact)
	if in.Group != nil {
		f.group = string(*in.Group)
	}
	f.events = eventsOf(in.Events, f.events)
	if in.ContactEveryDays != nil {
		f.every = *in.ContactEveryDays
	}
	if in.RemindDays != nil {
		f.remind = *in.RemindDays
	}
	if in.Phones != nil {
		f.phones = *in.Phones
	}
	if in.Emails != nil {
		f.emails = *in.Emails
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

func (m *Module) save(ctx context.Context, cur db.Contact, f fields, archivedAt *time.Time) (api.Contact, error) {
	f, err := m.validate(f)
	if err != nil {
		return api.Contact{}, err
	}
	events, _ := json.Marshal(f.events)
	notified := forgetChanged(cur, f.events)
	row, err := m.q.UpdateContact(ctx, db.UpdateContactParams{
		ID: cur.ID, Name: f.name, GroupKind: f.group, Events: string(events), LastContactOn: f.lastContact,
		ContactEveryDays: int64(f.every), RemindDays: formatRemind(f.remind), Notes: f.notes, ArchivedAt: archivedAt, UpdatedAt: m.now().UTC(),
		Phones: formatList(f.phones), Emails: formatList(f.emails), Source: f.source, ExternalID: f.externalID,
	})
	if err != nil {
		return api.Contact{}, err
	}
	if notified != cur.NotifiedJson {
		if err := m.q.SetContactNotified(ctx, db.SetContactNotifiedParams{NotifiedJson: notified, ID: row.ID}); err != nil {
			return api.Contact{}, err
		}
		row.NotifiedJson = notified
	}
	m.remindNow(ctx, row)
	return m.view(row, m.today()), nil
}

// forgetChanged drops the reminders already sent for a date whose date was
// changed, so the new date reminds again.
func forgetChanged(cur db.Contact, events []event) string {
	old := map[string]string{}
	for _, e := range parseEvents(cur.Events) {
		old[e.ID] = e.Date
	}
	changed := map[string]bool{}
	for _, e := range events {
		if d, ok := old[e.ID]; ok && d != e.Date {
			changed[e.ID] = true
		}
	}
	if len(changed) == 0 {
		return cur.NotifiedJson
	}
	var record, kept []string
	_ = json.Unmarshal([]byte(cur.NotifiedJson), &record)
	for _, k := range record {
		if !changed[strings.SplitN(k, ":", 2)[0]] {
			kept = append(kept, k)
		}
	}
	raw, _ := json.Marshal(kept)
	if kept == nil {
		return "[]"
	}
	return string(raw)
}

// TouchContact implements api.ServerInterface.
func (m *Module) TouchContact(w http.ResponseWriter, r *http.Request, id api.ContactId) {
	var body api.ContactTouch
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<12))
	if err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		err = dec.Decode(&body)
	}
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("请求体格式不正确"))
		return
	}
	ctx := r.Context()
	v, err := m.touch(ctx, id, pick(body.Date, ""))
	m.d.Audit.Record(ctx, "contact.touch", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("contact.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) touch(ctx context.Context, id int64, date string) (api.Contact, error) {
	cur, err := m.get(ctx, id)
	if err != nil {
		return api.Contact{}, err
	}
	f := fieldsOf(cur)
	f.lastContact = strings.TrimSpace(date)
	if f.lastContact == "" {
		f.lastContact = m.today().Format(dateLayout)
	}
	return m.save(ctx, cur, f, cur.ArchivedAt)
}

// DeleteContact implements api.ServerInterface.
func (m *Module) DeleteContact(w http.ResponseWriter, r *http.Request, id api.ContactId) {
	ctx := r.Context()
	n, err := m.q.DeleteContact(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "contact.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("contact.deleted", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

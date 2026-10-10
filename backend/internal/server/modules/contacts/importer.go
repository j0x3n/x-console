package contacts

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/db"
)

const (
	sourceVCard  = "vcard"
	sourceICloud = "icloud"
	maxImport    = 20 << 20
)

type applyResult struct{ total, created, updated, skipped int }

// apply puts people into the contacts. A person is matched to a contact by
// the vCard UID, or by name when the contact was made by hand or has no UID.
// A match keeps everything the user set here (group, contact period, reminder
// days, last contact, notes) and only takes the name, phones, e-mails and the
// dates that came from a vCard.
func (m *Module) apply(ctx context.Context, people []person, source string, onlyWithDates bool) (applyResult, error) {
	res := applyResult{total: len(people)}
	rows, err := m.q.ListContacts(ctx)
	if err != nil {
		return res, err
	}
	byUID := map[string]db.Contact{}
	byName := map[string]db.Contact{}
	for _, r := range rows {
		if r.ExternalID != "" {
			byUID[r.ExternalID] = r
		} else {
			byName[strings.ToLower(r.Name)] = r
		}
	}
	for _, p := range people {
		p = tidy(p)
		if p.name == "" || onlyWithDates && len(p.events) == 0 {
			res.skipped++
			continue
		}
		cur, found := db.Contact{}, false
		if p.uid != "" {
			cur, found = byUID[p.uid]
		}
		if !found {
			cur, found = byName[strings.ToLower(p.name)]
		}
		if !found {
			f := fields{name: p.name, group: string(api.ContactGroupOther), remind: defaultRemind, notes: p.note,
				phones: p.phones, emails: p.emails, events: p.events, source: source, externalID: p.uid}
			f.events = dropSecondBirthday(f.events)
			created, err := m.insert(ctx, f)
			if err != nil {
				res.skipped++
				continue
			}
			res.created++
			// two people with the same name in one file must not match each other's new row
			if p.uid != "" {
				byUID[p.uid] = db.Contact{ID: created.Id, Name: created.Name, ExternalID: p.uid}
			} else {
				byName[strings.ToLower(p.name)] = db.Contact{ID: created.Id, Name: created.Name}
			}
			continue
		}
		changed, err := m.merge(ctx, cur, p, source)
		if err != nil {
			res.skipped++
			continue
		}
		if changed {
			res.updated++
		}
	}
	return res, nil
}

// merge updates one contact from a person. It reports whether anything changed.
func (m *Module) merge(ctx context.Context, cur db.Contact, p person, source string) (bool, error) {
	f := fieldsOf(cur)
	before := fieldsOf(cur)
	f.name = p.name
	if cur.Source == "" {
		// a contact made by hand keeps its own numbers, the imported ones are added
		f.phones = union(f.phones, p.phones)
		f.emails = union(f.emails, p.emails)
	} else {
		f.phones, f.emails = p.phones, p.emails
	}
	kept := []event{}
	for _, e := range f.events {
		if !strings.HasPrefix(e.ID, importedPrefix) {
			kept = append(kept, e)
		}
	}
	f.events = dropSecondBirthday(append(kept, p.events...))
	if f.source == "" {
		f.source = source
	}
	if f.externalID == "" {
		f.externalID = p.uid
	}
	if f.notes == "" {
		f.notes = p.note
	}
	if same(f, before) {
		return false, nil
	}
	_, err := m.save(ctx, cur, f, cur.ArchivedAt)
	return err == nil, err
}

func same(a, b fields) bool {
	return a.name == b.name && a.notes == b.notes && a.source == b.source && a.externalID == b.externalID &&
		slices.Equal(a.phones, b.phones) && slices.Equal(a.emails, b.emails) && slices.Equal(a.events, b.events)
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, v := range b {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// dropSecondBirthday keeps the first birthday: a contact can only have one.
func dropSecondBirthday(events []event) []event {
	out := make([]event, 0, len(events))
	seen := false
	for _, e := range events {
		if e.Kind == api.ContactEventKindBirthday {
			if seen {
				continue
			}
			seen = true
		}
		out = append(out, e)
	}
	return out
}

// tidy cuts values to the lengths the module accepts, so one long note or a
// card with many numbers does not make the whole card fail.
func tidy(p person) person {
	p.name = cut(strings.TrimSpace(p.name), maxName)
	p.note = cut(p.note, maxNotes)
	p.phones = cutList(p.phones, maxPhones)
	p.emails = cutList(p.emails, maxEmails)
	if len(p.events) > maxEvents {
		p.events = p.events[:maxEvents]
	}
	for i := range p.events {
		p.events[i].Label = cut(p.events[i].Label, maxLabel)
	}
	return p
}

func cut(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func cutList(list []string, max int) []string {
	out := []string{}
	for _, v := range list {
		v = cut(strings.TrimSpace(v), maxContact)
		if v != "" && !slices.Contains(out, v) && len(out) < max {
			out = append(out, v)
		}
	}
	return out
}

// ImportContacts implements api.ServerInterface.
func (m *Module) ImportContacts(w http.ResponseWriter, r *http.Request, params api.ImportContactsParams) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxImport+1<<20)
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("请选择一个 .vcf 文件（文件不能超过 20 MB）"))
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxImport+1))
	if err != nil || len(raw) > maxImport {
		httpx.Fail(w, r, httpx.Invalid("文件太大了，不能超过 20 MB"))
		return
	}
	people, bad := parseVCards(strings.NewReader(string(raw)))
	if len(people) == 0 {
		httpx.Fail(w, r, httpx.Invalid("文件里没有找到联系人，要选 vCard（.vcf）文件"))
		return
	}
	res, err := m.apply(ctx, people, sourceVCard, params.OnlyWithDates != nil && *params.OnlyWithDates)
	res.total += bad
	res.skipped += bad
	m.d.Audit.Record(ctx, "contact.import", "", map[string]any{"total": res.total, "created": res.created, "updated": res.updated}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("contact.updated", map[string]any{"import": true})
	httpx.JSON(w, http.StatusOK, api.ContactImportResult{Total: res.total, Created: res.created, Updated: res.updated, Skipped: res.skipped})
}

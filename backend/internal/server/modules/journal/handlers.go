package journal

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/db"
)

func toItem(r db.JournalItem) api.JournalItem {
	return api.JournalItem{
		Id: r.ID, At: r.At, Module: r.Module, Kind: api.JournalKind(r.Kind),
		Title: r.Title, Detail: r.Detail, Link: r.Link, Minutes: int(r.Minutes),
	}
}

func summarize(items []api.JournalItem) []api.JournalCount {
	byKind := map[api.JournalKind]*api.JournalCount{}
	for _, it := range items {
		c := byKind[it.Kind]
		if c == nil {
			c = &api.JournalCount{Kind: it.Kind}
			byKind[it.Kind] = c
		}
		c.Count++
		c.Minutes += it.Minutes
	}
	out := []api.JournalCount{}
	for _, k := range kindOrder {
		if c := byKind[k]; c != nil {
			out = append(out, *c)
			delete(byKind, k)
		}
	}
	// kinds a newer source brings that this list does not know yet
	for _, c := range byKind {
		out = append(out, *c)
	}
	return out
}

// day returns the visible part of one day.
func (m *Module) day(ctx context.Context, day string) (api.JournalDay, error) {
	if _, err := m.parseDay(day); err != nil {
		return api.JournalDay{}, err
	}
	// pages for the last few days refresh the stored items first
	if t, _ := m.parseDay(day); !t.Before(m.startOfDay(m.now()).AddDate(0, 0, -(recentDays - 1))) {
		m.ensureFresh(ctx)
	}
	rows, err := m.q.ItemsOfDay(ctx, day)
	if err != nil {
		return api.JournalDay{}, err
	}
	visible := m.visibility(ctx)
	items := []api.JournalItem{}
	for _, r := range rows {
		if visible(r.Module) {
			items = append(items, toItem(r))
		}
	}
	diary, err := m.diary(ctx, day)
	if err != nil {
		return api.JournalDay{}, err
	}
	return api.JournalDay{Day: day, Items: items, Counts: summarize(items), Diary: diary}, nil
}

func (m *Module) diary(ctx context.Context, day string) (api.JournalDiary, error) {
	row, err := m.q.GetDiary(ctx, day)
	if errors.Is(err, sql.ErrNoRows) {
		return api.JournalDiary{Day: day, Body: ""}, nil
	}
	if err != nil {
		return api.JournalDiary{}, err
	}
	t := row.UpdatedAt
	return api.JournalDiary{Day: day, Body: row.Body, UpdatedAt: &t}, nil
}

// GetJournalDay implements api.ServerInterface.
func (m *Module) GetJournalDay(w http.ResponseWriter, r *http.Request, day string) {
	out, err := m.day(r.Context(), day)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// PutJournalDiary implements api.ServerInterface.
func (m *Module) PutJournalDiary(w http.ResponseWriter, r *http.Request, day string) {
	// a missing body is an error, an empty one clears the day
	var body struct {
		Body *string `json:"body"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Body == nil {
		httpx.Fail(w, r, httpx.Invalid("缺少 body"))
		return
	}
	ctx := r.Context()
	err := m.saveDiary(ctx, day, *body.Body)
	m.d.Audit.Record(ctx, "journal.diary", day, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.diary(ctx, day)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("journal.updated", map[string]any{"day": day})
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) saveDiary(ctx context.Context, day, body string) error {
	if _, err := m.parseDay(day); err != nil {
		return err
	}
	if utf8.RuneCountInString(body) > maxDiary {
		return httpx.Invalid("日记最多 20000 个字")
	}
	if strings.TrimSpace(body) == "" {
		return m.q.DeleteDiary(ctx, day)
	}
	return m.q.SaveDiary(ctx, db.SaveDiaryParams{Day: day, Body: body, UpdatedAt: m.now().UTC()})
}

// ListJournalRecent implements api.ServerInterface.
func (m *Module) ListJournalRecent(w http.ResponseWriter, r *http.Request, params api.ListJournalRecentParams) {
	days := 14
	if params.Days != nil {
		days = *params.Days
	}
	if days < 1 || days > 366 {
		httpx.Fail(w, r, httpx.Invalid("天数要在 1 到 366 之间"))
		return
	}
	out, err := m.recent(r.Context(), days)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) recent(ctx context.Context, days int) (api.JournalRecent, error) {
	today := m.startOfDay(m.now())
	since := today.AddDate(0, 0, -(days - 1)).Format(dayLayout)
	counts, err := m.q.DayModuleCounts(ctx, since)
	if err != nil {
		return api.JournalRecent{}, err
	}
	diaries, err := m.q.DiaryDaysSince(ctx, since)
	if err != nil {
		return api.JournalRecent{}, err
	}
	visible := m.visibility(ctx)
	n := map[string]int{}
	for _, c := range counts {
		if visible(c.Module) {
			n[c.Day] += int(c.N)
		}
	}
	hasDiary := map[string]bool{}
	for _, d := range diaries {
		hasDiary[d] = true
	}
	out := api.JournalRecent{Days: []api.JournalRecentDay{}}
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -i).Format(dayLayout)
		if i == 0 || n[d] > 0 || hasDiary[d] {
			out.Days = append(out.Days, api.JournalRecentDay{Day: d, Items: n[d], Diary: hasDiary[d]})
		}
	}
	return out, nil
}

// ---------- search ----------

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// snippet returns the text around the first match, about 40 characters each side.
func snippet(text, q string) string {
	runes := []rune(text)
	needle := []rune(strings.ToLower(q))
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	at := -1
	for i := 0; i+len(needle) <= len(lower); i++ {
		match := true
		for j, r := range needle {
			if lower[i+j] != r {
				match = false
				break
			}
		}
		if match {
			at = i
			break
		}
	}
	if at < 0 {
		at = 0
	}
	start, end := max(0, at-40), min(len(runes), at+len(needle)+40)
	out := strings.Join(strings.Fields(string(runes[start:end])), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// SearchJournal implements api.ServerInterface.
func (m *Module) SearchJournal(w http.ResponseWriter, r *http.Request, params api.SearchJournalParams) {
	out, err := m.search(r.Context(), params.Q)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) search(ctx context.Context, q string) (api.JournalSearch, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return api.JournalSearch{}, httpx.Invalid("请输入要找的词")
	}
	if utf8.RuneCountInString(q) > 100 {
		return api.JournalSearch{}, httpx.Invalid("搜索词最多 100 个字")
	}
	p := likePattern(q)
	visible := m.visibility(ctx)
	type hit struct {
		at  time.Time
		day string
		h   api.JournalHit
	}
	var hits []hit

	rows, err := m.d.DB.QueryContext(ctx,
		`SELECT day, at, module, kind, title, detail FROM journal_items WHERE title LIKE ?1 ESCAPE '\' OR detail LIKE ?1 ESCAPE '\' ORDER BY at DESC LIMIT 300`, p)
	if err != nil {
		return api.JournalSearch{}, err
	}
	for rows.Next() {
		var day, mod, kind, title, detail string
		var at time.Time
		if err := rows.Scan(&day, &at, &mod, &kind, &title, &detail); err != nil {
			rows.Close()
			return api.JournalSearch{}, err
		}
		if !visible(mod) {
			continue
		}
		text := detail
		if !strings.Contains(strings.ToLower(detail), strings.ToLower(q)) {
			text = ""
		}
		snip := ""
		if text != "" {
			snip = snippet(text, q)
		}
		hits = append(hits, hit{at, day, api.JournalHit{Day: day, Kind: kind, Title: title, Snippet: snip, Link: "/journal?date=" + day}})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return api.JournalSearch{}, err
	}
	rows.Close()

	drows, err := m.d.DB.QueryContext(ctx, `SELECT day, body FROM journal_diary WHERE body LIKE ?1 ESCAPE '\' ORDER BY day DESC LIMIT 100`, p)
	if err != nil {
		return api.JournalSearch{}, err
	}
	defer drows.Close()
	for drows.Next() {
		var day, body string
		if err := drows.Scan(&day, &body); err != nil {
			return api.JournalSearch{}, err
		}
		t, _ := time.ParseInLocation(dayLayout, day, m.loc())
		// a diary sorts just before the items of its day
		hits = append(hits, hit{t.Add(24*time.Hour - time.Second), day, api.JournalHit{Day: day, Kind: "diary", Title: "日记", Snippet: snippet(body, q), Link: "/journal?date=" + day}})
	}
	if err := drows.Err(); err != nil {
		return api.JournalSearch{}, err
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at.After(hits[j].at) })
	out := api.JournalSearch{Items: []api.JournalHit{}}
	for _, h := range hits {
		if len(out.Items) == maxSearchHits {
			break
		}
		out.Items = append(out.Items, h.h)
	}
	return out, nil
}

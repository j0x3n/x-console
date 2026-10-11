package cards

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	// OptionsKey holds the settings as one JSON value.
	OptionsKey = "cards.options"
	// lastPushKey holds the date (YYYY-MM-DD) of the last daily push.
	lastPushKey = "cards.last_push"

	defaultDeck   = "默认"
	maxText       = 4000
	maxDeckName   = 60
	maxSourceText = 300
	maxQueue      = 500
)

// options are the settings with their defaults.
type options struct {
	NewPerDay    int     `json:"newPerDay"`
	ReviewPerDay int     `json:"reviewPerDay"`
	Retention    float64 `json:"retention"`
	PushEnabled  bool    `json:"pushEnabled"`
	PushTime     string  `json:"pushTime"`
	AIEnabled    bool    `json:"aiEnabled"`
}

func defaultOptions() options {
	return options{NewPerDay: 10, ReviewPerDay: 100, Retention: 0.9, PushEnabled: true, PushTime: "20:00", AIEnabled: true}
}

func (m *Module) options(ctx context.Context) options {
	o := defaultOptions()
	if err := m.d.Settings.Get(ctx, OptionsKey, &o); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("cards: read settings", "err", err)
	}
	if o.Retention < 0.7 || o.Retention > 0.97 {
		o.Retention = 0.9
	}
	return o
}

// dayBounds is the local day of t as [start, end), in UTC for the database.
func (m *Module) dayBounds(t time.Time) (time.Time, time.Time) {
	l := t.In(m.location())
	start := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, m.location())
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

// ---- cards ----

func toAPI(c db.Card) api.Card {
	out := api.Card{
		Id: c.ID, Deck: c.Deck, Question: c.Question, Answer: c.Answer,
		SourceTitle: c.SourceTitle, SourceLink: c.SourceLink, Suspended: c.Suspended != 0,
		State: api.New, Due: c.Due, LastReview: c.LastReview, Stability: c.Stability, Difficulty: c.Difficulty,
		Reps: int(c.Reps), Lapses: int(c.Lapses), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if name, ok := stateNames[fsrsCard(c).State]; ok {
		out.State = name
	}
	return out
}

func cleanText(field, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", httpx.Invalid(field + "不能为空")
	}
	if utf8.RuneCountInString(s) > maxText {
		return "", httpx.Invalid(field + "太长，最多 4000 字")
	}
	return s, nil
}

func cleanDeck(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultDeck, nil
	}
	if utf8.RuneCountInString(s) > maxDeckName {
		return "", httpx.Invalid("牌组名太长，最多 60 字")
	}
	return s, nil
}

func cleanSource(title, link string) (string, string, error) {
	title, link = strings.TrimSpace(title), strings.TrimSpace(link)
	if utf8.RuneCountInString(title) > maxSourceText || utf8.RuneCountInString(link) > 1000 {
		return "", "", httpx.Invalid("来源太长")
	}
	// Only in-app paths and web addresses: the link is shown as a link.
	if link != "" && !strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return "", "", httpx.Invalid("来源链接要是站内路径或网址")
	}
	if strings.HasPrefix(link, "//") {
		return "", "", httpx.Invalid("来源链接要是站内路径或网址")
	}
	return title, link, nil
}

// createCard validates and stores a new card.
func (m *Module) createCard(ctx context.Context, in api.CardInput) (db.Card, error) {
	question, err := cleanText("问题", in.Question)
	if err != nil {
		return db.Card{}, err
	}
	answer, err := cleanText("答案", in.Answer)
	if err != nil {
		return db.Card{}, err
	}
	deck, err := cleanDeck(deref(in.Deck))
	if err != nil {
		return db.Card{}, err
	}
	title, link, err := cleanSource(deref(in.SourceTitle), deref(in.SourceLink))
	if err != nil {
		return db.Card{}, err
	}
	now := m.now().UTC()
	card, err := m.q.InsertCard(ctx, db.InsertCardParams{
		Deck: deck, Question: question, Answer: answer, SourceTitle: title, SourceLink: link,
		Due: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return db.Card{}, err
	}
	m.d.Audit.Record(ctx, "card.create", idString(card.ID), map[string]any{"deck": deck}, nil)
	m.d.Bus.Publish("card.created", toAPI(card))
	return card, nil
}

func (m *Module) getCard(ctx context.Context, id int64) (db.Card, error) {
	card, err := m.q.GetCard(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Card{}, httpx.ErrNotFound
	}
	return card, err
}

func (m *Module) patchCard(ctx context.Context, id int64, in api.CardPatch) (db.Card, error) {
	card, err := m.getCard(ctx, id)
	if err != nil {
		return db.Card{}, err
	}
	p := db.UpdateCardContentParams{
		Deck: card.Deck, Question: card.Question, Answer: card.Answer, SourceTitle: card.SourceTitle,
		SourceLink: card.SourceLink, Suspended: card.Suspended, UpdatedAt: m.now().UTC(), ID: id,
	}
	if in.Question != nil {
		if p.Question, err = cleanText("问题", *in.Question); err != nil {
			return db.Card{}, err
		}
	}
	if in.Answer != nil {
		if p.Answer, err = cleanText("答案", *in.Answer); err != nil {
			return db.Card{}, err
		}
	}
	if in.Deck != nil {
		if p.Deck, err = cleanDeck(*in.Deck); err != nil {
			return db.Card{}, err
		}
	}
	if in.SourceTitle != nil || in.SourceLink != nil {
		title, link := card.SourceTitle, card.SourceLink
		if in.SourceTitle != nil {
			title = *in.SourceTitle
		}
		if in.SourceLink != nil {
			link = *in.SourceLink
		}
		if p.SourceTitle, p.SourceLink, err = cleanSource(title, link); err != nil {
			return db.Card{}, err
		}
	}
	if in.Suspended != nil {
		p.Suspended = 0
		if *in.Suspended {
			p.Suspended = 1
		}
	}
	card, err = m.q.UpdateCardContent(ctx, p)
	if err != nil {
		return db.Card{}, err
	}
	m.d.Audit.Record(ctx, "card.update", idString(id), nil, nil)
	m.d.Bus.Publish("card.updated", toAPI(card))
	return card, nil
}

func (m *Module) deleteCard(ctx context.Context, id int64) error {
	n, err := m.q.DeleteCard(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "card.delete", idString(id), nil, nil)
	m.d.Bus.Publish("card.deleted", map[string]any{"id": id})
	return nil
}

// ---- review ----

// summary counts what is left for today. Daily limits are global: they do not
// depend on the deck filter, only the counted cards do.
func (m *Module) summary(ctx context.Context, deck string) (api.CardSummary, error) {
	o := m.options(ctx)
	now := m.now()
	start, end := m.dayBounds(now)
	reviewed, err := m.q.CountReviewsBetween(ctx, db.CountReviewsBetweenParams{FromAt: start, Until: end, OnlyNew: 0})
	if err != nil {
		return api.CardSummary{}, err
	}
	learned, err := m.q.CountReviewsBetween(ctx, db.CountReviewsBetweenParams{FromAt: start, Until: end, OnlyNew: 1})
	if err != nil {
		return api.CardSummary{}, err
	}
	rawDue, err := m.q.CountDueCards(ctx, db.CountDueCardsParams{Until: end, Deck: deck})
	if err != nil {
		return api.CardSummary{}, err
	}
	rawNew, err := m.q.CountNewCards(ctx, deck)
	if err != nil {
		return api.CardSummary{}, err
	}
	total, err := m.q.CountAllCards(ctx, deck)
	if err != nil {
		return api.CardSummary{}, err
	}
	suspended, err := m.q.CountSuspendedCards(ctx, deck)
	if err != nil {
		return api.CardSummary{}, err
	}
	out := api.CardSummary{
		DueReview:     int(min(rawDue, max(int64(o.ReviewPerDay)-reviewed, 0))),
		DueNew:        int(min(rawNew, max(int64(o.NewPerDay)-learned, 0))),
		ReviewedToday: int(reviewed), LearnedToday: int(learned),
		Total: int(total), NewCards: int(rawNew), Suspended: int(suspended),
	}
	out.Due = out.DueReview + out.DueNew
	if next, err := m.q.NextDueCard(ctx, db.NextDueCardParams{Since: end, Deck: deck}); err == nil {
		out.NextDue = &next
	} else if !errors.Is(err, sql.ErrNoRows) {
		return api.CardSummary{}, err
	}
	since := start.AddDate(0, 0, -29)
	if all, err := m.q.CountRecallReviews(ctx, since); err != nil {
		return api.CardSummary{}, err
	} else if all > 0 {
		passed, err := m.q.CountPassedReviews(ctx, since)
		if err != nil {
			return api.CardSummary{}, err
		}
		v := float64(passed) / float64(all)
		out.Recall30 = &v
	}
	return out, nil
}

// DueCount implements contracts.CardsDue.
func (m *Module) DueCount(ctx context.Context) (int, error) {
	s, err := m.summary(ctx, "")
	return s.Due, err
}

// queue lists today's cards: due old cards first, then new ones.
func (m *Module) queue(ctx context.Context, deck string, limit int) (api.CardQueue, error) {
	sum, err := m.summary(ctx, deck)
	if err != nil {
		return api.CardQueue{}, err
	}
	limit = min(max(limit, 1), maxQueue)
	_, end := m.dayBounds(m.now())
	out := api.CardQueue{Items: []api.CardQueueItem{}, Summary: sum}
	var cards []db.Card
	if n := min(sum.DueReview, limit); n > 0 {
		list, err := m.q.ListDueCards(ctx, db.ListDueCardsParams{Until: end, Deck: deck, MaxRows: int64(n)})
		if err != nil {
			return api.CardQueue{}, err
		}
		cards = append(cards, list...)
	}
	if n := min(sum.DueNew, limit-len(cards)); n > 0 {
		list, err := m.q.ListNewCards(ctx, db.ListNewCardsParams{Deck: deck, MaxRows: int64(n)})
		if err != nil {
			return api.CardQueue{}, err
		}
		cards = append(cards, list...)
	}
	f := newScheduler(m.options(ctx).Retention)
	now := m.now()
	for _, c := range cards {
		iv, err := intervals(f, c, now)
		if err != nil {
			return api.CardQueue{}, err
		}
		out.Items = append(out.Items, api.CardQueueItem{Card: toAPI(c), Intervals: iv})
	}
	return out, nil
}

var errStale = httpx.NewError(http.StatusConflict, "card_stale", "这张卡已经答过了")

// reviewCard applies a rating. When reps is set it must match the card, so a
// button pressed twice (or answered somewhere else already) does not count
// twice.
func (m *Module) reviewCard(ctx context.Context, id int64, rating int, reps *int) (api.CardReviewResult, error) {
	if rating < 1 || rating > 4 {
		return api.CardReviewResult{}, httpx.Invalid("评分要在 1 到 4 之间")
	}
	f := newScheduler(m.options(ctx).Retention)
	now := m.now().UTC()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.CardReviewResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	card, err := q.GetCard(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.CardReviewResult{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.CardReviewResult{}, err
	}
	if card.Suspended != 0 {
		return api.CardReviewResult{}, httpx.NewError(http.StatusConflict, "card_suspended", "这张卡已暂停")
	}
	if reps != nil && int64(*reps) != card.Reps {
		return api.CardReviewResult{}, errStale
	}
	info, err := schedule(f, card, rating, now)
	if err != nil {
		return api.CardReviewResult{}, err
	}
	next := info.Card
	last := next.LastReview.UTC()
	updated, err := q.UpdateCardSchedule(ctx, db.UpdateCardScheduleParams{
		State: int64(next.State), Stability: next.Stability, Difficulty: next.Difficulty, Due: next.Due.UTC(),
		LastReview: &last, ScheduledDays: int64(next.ScheduledDays), Reps: int64(next.Reps), Lapses: int64(next.Lapses),
		UpdatedAt: now, ID: id,
	})
	if err != nil {
		return api.CardReviewResult{}, err
	}
	err = q.InsertCardReview(ctx, db.InsertCardReviewParams{
		CardID: id, Rating: int64(rating), StateBefore: card.State, ReviewedAt: now,
		ScheduledDays: int64(next.ScheduledDays), Stability: next.Stability, Difficulty: next.Difficulty,
	})
	if err != nil {
		return api.CardReviewResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return api.CardReviewResult{}, err
	}
	sum, err := m.summary(ctx, "")
	if err != nil {
		return api.CardReviewResult{}, err
	}
	m.d.Bus.Publish("card.reviewed", map[string]any{"id": id, "rating": rating, "due": sum.Due})
	return api.CardReviewResult{Card: toAPI(updated), IntervalDays: int(next.ScheduledDays), Summary: sum}, nil
}

// ---- settings ----

func (m *Module) settingsDTO(ctx context.Context) api.CardSettings {
	o := m.options(ctx)
	return api.CardSettings{
		NewPerDay: o.NewPerDay, ReviewPerDay: o.ReviewPerDay, Retention: o.Retention,
		PushEnabled: o.PushEnabled, PushTime: o.PushTime, AiEnabled: o.AIEnabled, AiAvailable: m.aiAvailable(ctx),
	}
}

// parseClock reads "HH:MM".
func parseClock(s string) (int, int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func (m *Module) putSettings(ctx context.Context, in api.CardSettingsInput) error {
	o := m.options(ctx)
	if in.NewPerDay != nil {
		if *in.NewPerDay < 0 || *in.NewPerDay > 500 {
			return httpx.Invalid("每天新卡上限要在 0 到 500 之间")
		}
		o.NewPerDay = *in.NewPerDay
	}
	if in.ReviewPerDay != nil {
		if *in.ReviewPerDay < 0 || *in.ReviewPerDay > 5000 {
			return httpx.Invalid("每天复习上限要在 0 到 5000 之间")
		}
		o.ReviewPerDay = *in.ReviewPerDay
	}
	if in.Retention != nil {
		if *in.Retention < 0.7 || *in.Retention > 0.97 {
			return httpx.Invalid("目标记忆保持率要在 70% 到 97% 之间")
		}
		o.Retention = *in.Retention
	}
	if in.PushEnabled != nil {
		o.PushEnabled = *in.PushEnabled
	}
	if in.PushTime != nil {
		if _, _, ok := parseClock(*in.PushTime); !ok {
			return httpx.Invalid("推送时间要写成 20:00 这样")
		}
		o.PushTime = *in.PushTime
	}
	if in.AiEnabled != nil {
		o.AIEnabled = *in.AiEnabled
	}
	err := m.d.Settings.Set(ctx, OptionsKey, o)
	m.d.Audit.Record(ctx, "card.settings.update", "", nil, err)
	return err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

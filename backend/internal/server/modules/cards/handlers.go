package cards

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/db"
)

func idString(id int64) string { return strconv.FormatInt(id, 10) }

// ListCards is GET /cards.
func (m *Module) ListCards(w http.ResponseWriter, r *http.Request, p api.ListCardsParams) {
	ctx := r.Context()
	before, err := httpx.DecodeIDCursor(p.Cursor)
	if fail(w, r, err) {
		return
	}
	limit := httpx.Limit(p.Limit)
	state, suspended := int64(-1), int64(-1)
	if p.State != nil {
		v, ok := stateValue(*p.State)
		if !ok {
			httpx.Fail(w, r, httpx.Invalid("状态不对"))
			return
		}
		state = v
	}
	if p.Suspended != nil {
		suspended = 0
		if *p.Suspended {
			suspended = 1
		}
	}
	query := strings.TrimSpace(deref(p.Q))
	deck := deref(p.Deck)
	rows, err := m.q.ListCards(ctx, db.ListCardsParams{
		Before: before, Q: query, Deck: deck, State: state, Suspended: suspended, MaxRows: limit + 1,
	})
	if fail(w, r, err) {
		return
	}
	total, err := m.q.CountCards(ctx, db.CountCardsParams{Q: query, Deck: deck, State: state, Suspended: suspended})
	if fail(w, r, err) {
		return
	}
	out := api.CardPage{Items: []api.Card{}, Total: int(total)}
	for i, c := range rows {
		if int64(i) == limit {
			next := httpx.EncodeIDCursor(rows[i-1].ID)
			out.NextCursor = &next
			break
		}
		out.Items = append(out.Items, toAPI(c))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateCard is POST /cards.
func (m *Module) CreateCard(w http.ResponseWriter, r *http.Request) {
	var in api.CardInput
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	card, err := m.createCard(r.Context(), in)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, toAPI(card))
}

// GetCard is GET /cards/{id}.
func (m *Module) GetCard(w http.ResponseWriter, r *http.Request, id api.CardId) {
	card, err := m.getCard(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(card))
}

// UpdateCard is PATCH /cards/{id}.
func (m *Module) UpdateCard(w http.ResponseWriter, r *http.Request, id api.CardId) {
	var in api.CardPatch
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	card, err := m.patchCard(r.Context(), id, in)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toAPI(card))
}

// DeleteCard is DELETE /cards/{id}.
func (m *Module) DeleteCard(w http.ResponseWriter, r *http.Request, id api.CardId) {
	if fail(w, r, m.deleteCard(r.Context(), id)) {
		return
	}
	httpx.NoContent(w)
}

// ReviewCard is POST /cards/{id}/review.
func (m *Module) ReviewCard(w http.ResponseWriter, r *http.Request, id api.CardId) {
	var in api.CardReviewInput
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	out, err := m.reviewCard(r.Context(), id, in.Rating, in.Reps)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetCardQueue is GET /cards/queue.
func (m *Module) GetCardQueue(w http.ResponseWriter, r *http.Request, p api.GetCardQueueParams) {
	limit := 200
	if p.Limit != nil {
		limit = *p.Limit
	}
	out, err := m.queue(r.Context(), deref(p.Deck), limit)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetCardSummary is GET /cards/summary.
func (m *Module) GetCardSummary(w http.ResponseWriter, r *http.Request, p api.GetCardSummaryParams) {
	out, err := m.summary(r.Context(), deref(p.Deck))
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ListCardDecks is GET /cards/decks.
func (m *Module) ListCardDecks(w http.ResponseWriter, r *http.Request) {
	_, end := m.dayBounds(m.now())
	rows, err := m.q.ListDecks(r.Context(), end)
	if fail(w, r, err) {
		return
	}
	out := make([]api.CardDeck, 0, len(rows))
	for _, d := range rows {
		out = append(out, api.CardDeck{Name: d.Name, Total: int(d.Total), Due: int(d.Due), NewCards: int(d.NewCards)})
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetCardSettings is GET /cards/settings.
func (m *Module) GetCardSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.settingsDTO(r.Context()))
}

// PutCardSettings is PUT /cards/settings.
func (m *Module) PutCardSettings(w http.ResponseWriter, r *http.Request) {
	var in api.CardSettingsInput
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	if fail(w, r, m.putSettings(r.Context(), in)) {
		return
	}
	httpx.JSON(w, http.StatusOK, m.settingsDTO(r.Context()))
}

// GenerateCard is POST /cards/generate.
func (m *Module) GenerateCard(w http.ResponseWriter, r *http.Request) {
	var in api.CardGenerateInput
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	out, err := m.generate(r.Context(), in)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/clubmatch/clubmatch/internal/visibility"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const deckLimit = 10

type DeckHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewDeckHandler(pool *pgxpool.Pool, st *store.Store) *DeckHandler {
	return &DeckHandler{pool: pool, st: st}
}

func loadCard(ctx context.Context, st *store.Store, userID pgtype.UUID) (visibility.Card, error) {
	card := visibility.Card{
		ID:                    uuidString(userID),
		Skills:                []string{},
		Projects:              []string{},
		ProfessionalInterests: []string{},
		PersonalInterests:     []string{},
	}
	kinds := []struct {
		kind store.ProfileTagKind
		dst  *[]string
	}{
		{store.ProfileTagKindSkill, &card.Skills},
		{store.ProfileTagKindProfessionalInterest, &card.ProfessionalInterests},
		{store.ProfileTagKindPersonalInterest, &card.PersonalInterests},
	}
	for _, k := range kinds {
		list, err := st.ListProfileTagCanonicals(ctx, store.ListProfileTagCanonicalsParams{
			ProfileID: userID,
			Kind:      k.kind,
		})
		if err != nil {
			return card, err
		}
		if list != nil {
			*k.dst = list
		}
	}
	projects, err := st.ListProjectTexts(ctx, userID)
	if err != nil {
		return card, err
	}
	if projects != nil {
		card.Projects = projects
	}
	return card, nil
}

func (h *DeckHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := h.st.ListDeckCards(ctx, store.ListDeckCardsParams{
		ProfileID: u.ID,
		Limit:     deckLimit,
	})
	if err != nil {
		slog.Error("deck: list cards", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		card, err := loadCard(ctx, h.st, row.UserID)
		if err != nil {
			slog.Error("deck: load card", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		card.Location = strVal(row.Location)
		card.MeetingFormat = string(row.MeetingFormat)
		out = append(out, visibility.Serialize(card, visibility.StatePreMatch))
	}
	writeJSON(w, http.StatusOK, out)
}

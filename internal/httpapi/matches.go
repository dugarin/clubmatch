package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/clubmatch/clubmatch/internal/visibility"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MatchesHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewMatchesHandler(pool *pgxpool.Pool, st *store.Store) *MatchesHandler {
	return &MatchesHandler{pool: pool, st: st}
}

type matchItemDTO struct {
	MatchID   string         `json:"match_id"`
	State     string         `json:"state"`
	Card      map[string]any `json:"card"`
	CreatedAt string         `json:"created_at"`
}

func (h *MatchesHandler) List(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := h.st.ListMatchesByUser(ctx, u.ID)
	if err != nil {
		slog.Error("matches: list", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]matchItemDTO, 0, len(rows))
	for _, row := range rows {
		item := matchItemDTO{
			MatchID:   uuidString(row.ID),
			State:     string(row.State),
			CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339),
		}
		if row.State != store.MatchStateActive {
			out = append(out, item)
			continue
		}
		card, err := loadCard(ctx, h.st, row.OtherID)
		if err != nil {
			slog.Error("matches: load card", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		card.FirstName = strVal(row.OtherFirstName)
		card.LastName = strVal(row.OtherLastName)
		card.Location = strVal(row.OtherLocation)
		if row.OtherMeetingFormat != nil {
			card.MeetingFormat = string(*row.OtherMeetingFormat)
		}
		card.TelegramUsername = row.OtherTelegramUsername
		tgID := row.OtherTelegramUserID
		card.TelegramUserID = &tgID
		item.Card = visibility.Serialize(card, visibility.StateMatched)
		if err := h.st.InsertContactDisclosure(ctx, store.InsertContactDisclosureParams{
			MatchID:                 row.ID,
			SubjectID:               row.OtherID,
			RecipientID:             u.ID,
			SubjectTelegramUserID:   row.OtherTelegramUserID,
			RecipientTelegramUserID: u.TelegramUserID,
		}); err != nil {
			slog.Error("matches: disclosure journal", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

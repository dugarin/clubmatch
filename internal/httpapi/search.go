package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/clubmatch/clubmatch/internal/profile"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/clubmatch/clubmatch/internal/visibility"
	"github.com/jackc/pgx/v5/pgxpool"
)

const searchPageSize = 10

type SearchHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewSearchHandler(pool *pgxpool.Pool, st *store.Store) *SearchHandler {
	return &SearchHandler{pool: pool, st: st}
}

type searchResponse struct {
	Items          []map[string]any `json:"items"`
	Page           int              `json:"page"`
	HasMore        bool             `json:"has_more"`
	FiltersApplied bool             `json:"filters_applied"`
}

func (h *SearchHandler) Get(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < 1 || n > 256 {
		writeErr(w, http.StatusBadRequest, "q must be 1..256 characters")
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 1 {
			writeErr(w, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = p
	}
	var locationNorm *string
	if loc := strings.TrimSpace(r.URL.Query().Get("location")); loc != "" {
		n := profile.Normalize(loc)
		locationNorm = &n
	}
	var meetingFormat *store.MeetingFormat
	if mf := r.URL.Query().Get("meeting_format"); mf != "" {
		if mf != "online" && mf != "offline" {
			writeErr(w, http.StatusBadRequest, "meeting_format must be online or offline")
			return
		}
		v := store.MeetingFormat(mf)
		meetingFormat = &v
	}

	ctx := r.Context()
	lexemes, err := h.st.LexemesOfText(ctx, profile.Normalize(q))
	if err != nil {
		slog.Error("search: lexemes", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	tagIDs, err := h.st.ResolveTagIDsByLexemes(ctx, lexemes)
	if err != nil {
		slog.Error("search: resolve tags", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(tagIDs) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "unknown tag", "q")
		return
	}
	rows, err := h.st.SearchProfilesByTags(ctx, store.SearchProfilesByTagsParams{
		EvaluatorID:   u.ID,
		LocationNorm:  locationNorm,
		MeetingFormat: meetingFormat,
		OffsetCount:   int32((page - 1) * searchPageSize),
		LimitCount:    searchPageSize + 1,
		TagIds:        tagIDs,
	})
	if err != nil {
		slog.Error("search: query", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	hasMore := len(rows) > searchPageSize
	if hasMore {
		rows = rows[:searchPageSize]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		card, err := loadCard(ctx, h.st, row.UserID)
		if err != nil {
			slog.Error("search: load card", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		card.Location = strVal(row.Location)
		card.MeetingFormat = string(row.MeetingFormat)
		out := visibility.Serialize(card, visibility.StatePreMatch)
		if ev, ok := row.MyEvaluation.(string); ok && ev != "" {
			out["my_evaluation"] = ev
		}
		items = append(items, out)
	}
	writeJSON(w, http.StatusOK, searchResponse{
		Items:          items,
		Page:           page,
		HasMore:        hasMore,
		FiltersApplied: locationNorm != nil || meetingFormat != nil,
	})
}

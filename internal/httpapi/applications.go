package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewApplicationHandler(pool *pgxpool.Pool, st *store.Store) *ApplicationHandler {
	return &ApplicationHandler{pool: pool, st: st}
}

type applicationDTO struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	DecidedAt string `json:"decided_at,omitempty"`
}

type applicationStatusDTO struct {
	AccessState string          `json:"access_state"`
	Application *applicationDTO `json:"application"`
}

func appDTO(a store.Application) applicationDTO {
	out := applicationDTO{
		ID:        uuidString(a.ID),
		State:     string(a.State),
		CreatedAt: a.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
	if a.DecidedAt.Valid {
		out.DecidedAt = a.DecidedAt.Time.UTC().Format(time.RFC3339)
	}
	return out
}

func (h *ApplicationHandler) Submit(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	switch u.AccessState {
	case store.UserAccessStateNone, store.UserAccessStateRejected:
	default:
		writeErr(w, http.StatusForbidden, "access already granted")
		return
	}
	ctx := r.Context()
	if _, err := h.st.GetPendingApplicationByCandidate(ctx, u.ID); err == nil {
		writeErr(w, http.StatusConflict, "active application already exists")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("applications: check pending", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a, err := h.st.CreateApplication(ctx, u.ID)
	if err != nil {
		slog.Error("applications: create", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, appDTO(a))
}

// GetMe returns the status of the caller's latest application for the
// Mini App status screen (ADR-002 F1). NOTE: this endpoint is not in
// docs/architecture/04-openapi.yaml yet — spec addition pending.
func (h *ApplicationHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	out := applicationStatusDTO{AccessState: string(u.AccessState)}
	a, err := h.st.GetLatestApplicationByCandidate(r.Context(), u.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("applications: load latest", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err == nil {
		dto := appDTO(a)
		out.Application = &dto
	}
	writeJSON(w, http.StatusOK, out)
}

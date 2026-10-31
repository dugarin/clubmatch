package httpapi

import (
	"log/slog"
	"net/http"
)

func (h *ProfileHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("profiles: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	steps := []func() error{
		func() error { return qtx.MarkMatchesUnavailableByUser(ctx, u.ID) },
		func() error { return qtx.CancelPendingNotificationsByUser(ctx, u.ID) },
		func() error { return qtx.DeleteEvaluationsByUser(ctx, u.ID) },
		func() error { return qtx.DeleteEvaluationHistoryByUser(ctx, u.ID) },
		func() error { return qtx.DeleteProfileTagsByUser(ctx, u.ID) },
		func() error { return qtx.DeleteProjectsByUser(ctx, u.ID) },
		func() error { return qtx.DeleteConsentByUser(ctx, u.ID) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			slog.Error("profiles: delete step", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	rows, err := qtx.DeleteProfileByUser(ctx, u.ID)
	if err != nil {
		slog.Error("profiles: delete profile", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rows == 0 {
		writeErr(w, http.StatusNotFound, "profile not found")
		return
	}
	if err := qtx.SoftDeleteUser(ctx, u.ID); err != nil {
		slog.Error("profiles: soft delete user", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("profiles: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

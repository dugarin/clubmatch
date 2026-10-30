package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/clubmatch/clubmatch/internal/auth"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5"
)

func currentUser(st *store.Store, w http.ResponseWriter, r *http.Request) (store.User, bool) {
	id, ok := auth.FromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return store.User{}, false
	}
	u, err := st.GetUserByTelegramID(r.Context(), id.TelegramUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return store.User{}, false
		}
		slog.Error("httpapi: get user", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return store.User{}, false
	}
	return u, true
}

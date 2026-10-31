package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/clubmatch/clubmatch/internal/auth"
	"github.com/clubmatch/clubmatch/internal/config"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool, cfg config.Config) http.Handler {
	st := store.NewStore(db)
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Post("/telegram/webhook", NewTelegramWebhookHandler(st, cfg.WebhookSecret))
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Middleware(st, cfg.TelegramBotToken, cfg.InitDataTTL))
		ph := NewProfileHandler(db, st)
		r.Post("/profiles", ph.Create)
		r.Get("/profiles/me", ph.GetMe)
		r.Patch("/profiles/me", ph.PatchMe)
		r.Delete("/profiles/me", ph.DeleteMe)
		r.Get("/tags/autocomplete", ph.Autocomplete)
		dh := NewDeckHandler(db, st)
		r.Get("/deck", dh.Get)
		eh := NewEvaluationHandler(db, st)
		r.Post("/evaluations", eh.Create)
		sh := NewSearchHandler(db, st)
		r.Get("/search", sh.Get)
		mth := NewMatchesHandler(db, st)
		r.Get("/matches", mth.List)
		ah := NewApplicationHandler(db, st)
		r.Post("/applications", ah.Submit)
		r.Get("/applications/me", ah.GetMe)
		mh := NewModerationHandler(db, st, cfg.BotUsername, cfg.MiniAppName)
		r.Route("/moderation", func(r chi.Router) {
			r.Use(RequireModerator(st))
			r.Get("/applications", mh.ListApplications)
			r.Post("/applications/{id}/approve", mh.Approve)
			r.Post("/applications/{id}/reject", mh.Reject)
			r.Post("/invites", mh.CreateInvite)
			r.Post("/tags/{id}/revoke", mh.RevokeTag)
		})
	})
	return r
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func PingContext(ctx context.Context, db *pgxpool.Pool) error {
	return db.Ping(ctx)
}

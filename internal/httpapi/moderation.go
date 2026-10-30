package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type moderatorCtxKey struct{}

// RequireModerator gates moderation endpoints: the caller must be an
// authenticated user with role=moderator in the DB (FR-15, NFR-9).
func RequireModerator(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := currentUser(st, w, r)
			if !ok {
				return
			}
			if u.Role != store.UserRoleModerator {
				writeErr(w, http.StatusForbidden, "moderator role required")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), moderatorCtxKey{}, u)))
		})
	}
}

type ModerationHandler struct {
	pool        *pgxpool.Pool
	st          *store.Store
	botUsername string
	miniAppName string
}

func NewModerationHandler(pool *pgxpool.Pool, st *store.Store, botUsername, miniAppName string) *ModerationHandler {
	return &ModerationHandler{pool: pool, st: st, botUsername: botUsername, miniAppName: miniAppName}
}

func (h *ModerationHandler) moderator(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(moderatorCtxKey{}).(store.User)
	if !ok {
		writeErr(w, http.StatusForbidden, "moderator role required")
		return store.User{}, false
	}
	return u, true
}

type moderationApplicationDTO struct {
	applicationDTO
	TelegramUserID   int64  `json:"telegram_user_id"`
	TelegramUsername string `json:"telegram_username,omitempty"`
}

func (h *ModerationHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.moderator(w, r); !ok {
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = string(store.ApplicationStatePending)
	}
	switch store.ApplicationState(state) {
	case store.ApplicationStatePending, store.ApplicationStateApproved, store.ApplicationStateRejected:
	default:
		writeErr(w, http.StatusBadRequest, "invalid state")
		return
	}
	rows, err := h.st.ListApplicationsByState(r.Context(), store.ApplicationState(state))
	if err != nil {
		slog.Error("moderation: list applications", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]moderationApplicationDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, moderationApplicationDTO{
			applicationDTO: appDTO(store.Application{
				ID:          row.ID,
				CandidateID: row.CandidateID,
				State:       row.State,
				DecidedBy:   row.DecidedBy,
				DecidedAt:   row.DecidedAt,
				CreatedAt:   row.CreatedAt,
			}),
			TelegramUserID:   row.TelegramUserID,
			TelegramUsername: strVal(row.TelegramUsername),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ModerationHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

func (h *ModerationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h *ModerationHandler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	mod, ok := h.moderator(w, r)
	if !ok {
		return
	}
	var appID pgtype.UUID
	if err := appID.Scan(chi.URLParam(r, "id")); err != nil || !appID.Valid {
		writeErr(w, http.StatusBadRequest, "invalid application id")
		return
	}
	newState := store.ApplicationStateRejected
	action := store.ModerationActionApplicationRejected
	ntype := store.NotificationTypeApplicationRejected
	if approve {
		newState = store.ApplicationStateApproved
		action = store.ModerationActionApplicationApproved
		ntype = store.NotificationTypeApplicationApproved
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("moderation: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	app, err := qtx.GetApplicationByID(ctx, appID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		slog.Error("moderation: load application", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	rows, err := qtx.DecideApplication(ctx, store.DecideApplicationParams{
		ID:        appID,
		State:     newState,
		DecidedBy: mod.ID,
	})
	if err != nil {
		slog.Error("moderation: decide application", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rows == 0 {
		writeErr(w, http.StatusConflict, "application already decided")
		return
	}
	if approve {
		err = qtx.ApproveUserAccess(ctx, app.CandidateID)
	} else {
		err = qtx.RejectUserAccess(ctx, app.CandidateID)
	}
	if err != nil {
		slog.Error("moderation: update access state", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	payload, err := json.Marshal(map[string]string{"application_id": uuidString(appID)})
	if err != nil {
		slog.Error("moderation: marshal payload", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertNotification(ctx, store.InsertNotificationParams{
		Type:    ntype,
		UserID:  app.CandidateID,
		Payload: payload,
	}); err != nil {
		slog.Error("moderation: insert notification", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertModerationJournal(ctx, store.InsertModerationJournalParams{
		ModeratorID: mod.ID,
		Action:      action,
		TargetType:  store.ModerationTargetTypeApplication,
		TargetID:    appID,
	}); err != nil {
		slog.Error("moderation: insert journal", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("moderation: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, applicationDTO{
		ID:        uuidString(appID),
		State:     string(newState),
		CreatedAt: app.CreatedAt.Time.UTC().Format(time.RFC3339),
		DecidedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ModerationHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	mod, ok := h.moderator(w, r)
	if !ok {
		return
	}
	if h.botUsername == "" || h.miniAppName == "" {
		writeErr(w, http.StatusInternalServerError, "invite links not configured")
		return
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		slog.Error("moderation: invite token rand", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("moderation: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	inv, err := qtx.CreateInvite(ctx, store.CreateInviteParams{Token: token, CreatedBy: mod.ID})
	if err != nil {
		slog.Error("moderation: create invite", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertModerationJournal(ctx, store.InsertModerationJournalParams{
		ModeratorID: mod.ID,
		Action:      store.ModerationActionInviteCreated,
		TargetType:  store.ModerationTargetTypeInvite,
		TargetID:    inv.ID,
	}); err != nil {
		slog.Error("moderation: insert journal", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("moderation: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"url": fmt.Sprintf("https://t.me/%s/%s?startapp=%s", h.botUsername, h.miniAppName, token),
	})
}

func (h *ModerationHandler) RevokeTag(w http.ResponseWriter, r *http.Request) {
	mod, ok := h.moderator(w, r)
	if !ok {
		return
	}
	var tagID pgtype.UUID
	if err := tagID.Scan(chi.URLParam(r, "id")); err != nil || !tagID.Valid {
		writeErr(w, http.StatusBadRequest, "invalid tag id")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("moderation: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	tag, err := qtx.GetTagByID(ctx, tagID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "tag not found")
		return
	}
	if err != nil {
		slog.Error("moderation: load tag", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	rows, err := qtx.RevokeTag(ctx, tagID)
	if err != nil {
		slog.Error("moderation: revoke tag", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rows == 0 {
		writeErr(w, http.StatusConflict, "tag already revoked")
		return
	}
	// Collect affected profiles before deleting the tag from them.
	lostLastSkill, err := qtx.ListProfilesWithLastSkillTag(ctx, tagID)
	if err != nil {
		slog.Error("moderation: list last-skill profiles", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	affected, err := qtx.ListProfileIDsByTag(ctx, tagID)
	if err != nil {
		slog.Error("moderation: list affected profiles", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.DeleteProfileTagsByTagID(ctx, tagID); err != nil {
		slog.Error("moderation: delete profile tags", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	payload, err := json.Marshal(map[string]string{"tag": tag.Canonical})
	if err != nil {
		slog.Error("moderation: marshal payload", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertNotification(ctx, store.InsertNotificationParams{
		Type:    store.NotificationTypeTagRevoked,
		UserID:  tag.CreatedBy,
		Payload: payload,
	}); err != nil {
		slog.Error("moderation: notify tag author", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, profileID := range lostLastSkill {
		if err := qtx.SetProfileSearchable(ctx, store.SetProfileSearchableParams{
			UserID:       profileID,
			IsSearchable: false,
		}); err != nil {
			slog.Error("moderation: unset searchable", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := qtx.InsertNotification(ctx, store.InsertNotificationParams{
			Type:    store.NotificationTypeLastSkillLost,
			UserID:  profileID,
			Payload: payload,
		}); err != nil {
			slog.Error("moderation: notify last skill lost", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	for _, profileID := range affected {
		if err := qtx.RebuildProfileFTS(ctx, profileID); err != nil {
			slog.Error("moderation: rebuild fts", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if err := qtx.InsertModerationJournal(ctx, store.InsertModerationJournalParams{
		ModeratorID: mod.ID,
		Action:      store.ModerationActionTagRevoked,
		TargetType:  store.ModerationTargetTypeTag,
		TargetID:    tagID,
	}); err != nil {
		slog.Error("moderation: insert journal", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("moderation: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     uuidString(tagID),
		"status": string(store.TagStatusRevoked),
	})
}

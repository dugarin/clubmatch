package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/clubmatch/clubmatch/internal/visibility"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EvaluationHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewEvaluationHandler(pool *pgxpool.Pool, st *store.Store) *EvaluationHandler {
	return &EvaluationHandler{pool: pool, st: st}
}

type evaluationDTO struct {
	TargetID string `json:"target_id"`
	Value    string `json:"value"`
	Source   string `json:"source"`
}

func orderedPair(a, b pgtype.UUID) (pgtype.UUID, pgtype.UUID) {
	if uuidString(a) < uuidString(b) {
		return a, b
	}
	return b, a
}

func (h *EvaluationHandler) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := currentUser(h.st, w, r)
	if !ok {
		return
	}
	var dto evaluationDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var targetID pgtype.UUID
	if err := targetID.Scan(dto.TargetID); err != nil || !targetID.Valid {
		writeErr(w, http.StatusUnprocessableEntity, "validation failed", "target_id")
		return
	}
	value := store.EvaluationValue(dto.Value)
	if value != store.EvaluationValueInterested && value != store.EvaluationValueNotInterested {
		writeErr(w, http.StatusUnprocessableEntity, "validation failed", "value")
		return
	}
	source := store.EvaluationSource(dto.Source)
	if dto.Source == "" {
		source = store.EvaluationSourceDeck
	}
	if source != store.EvaluationSourceDeck && source != store.EvaluationSourceSearch {
		writeErr(w, http.StatusUnprocessableEntity, "validation failed", "source")
		return
	}
	if targetID.Bytes == u.ID.Bytes {
		writeErr(w, http.StatusUnprocessableEntity, "cannot evaluate self", "target_id")
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("evaluations: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	low, high := orderedPair(u.ID, targetID)
	if err := qtx.LockEvaluationPair(ctx, store.LockEvaluationPairParams{
		UserLow:  uuidString(low),
		UserHigh: uuidString(high),
	}); err != nil {
		slog.Error("evaluations: lock pair", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	target, err := qtx.GetCardContact(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "target not found")
		return
	}
	if err != nil {
		slog.Error("evaluations: load target", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.UpsertEvaluation(ctx, store.UpsertEvaluationParams{
		EvaluatorID: u.ID,
		TargetID:    targetID,
		Value:       value,
		Source:      source,
	}); err != nil {
		slog.Error("evaluations: upsert", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertEvaluationHistory(ctx, store.InsertEvaluationHistoryParams{
		EvaluatorID: u.ID,
		TargetID:    targetID,
		Value:       value,
	}); err != nil {
		slog.Error("evaluations: insert history", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	matched := false
	if value == store.EvaluationValueInterested {
		reciprocal, err := qtx.GetEvaluationValue(ctx, store.GetEvaluationValueParams{
			EvaluatorID: targetID,
			TargetID:    u.ID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("evaluations: reciprocal", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		if reciprocal == store.EvaluationValueInterested {
			matched = true
			matchID, err := qtx.CreateMatch(ctx, store.CreateMatchParams{UserLow: low, UserHigh: high})
			switch {
			case err == nil:
				// Match created by this transaction: disclose contacts and
				// enqueue notifications for both sides (S1).
				if !h.discloseMatch(ctx, qtx, w, matchID, u, target) {
					return
				}
			case errors.Is(err, pgx.ErrNoRows):
				// Unique conflict: the match already exists (concurrent
				// transaction won the race or it was created earlier) —
				// journal and outbox were written by its creator.
			default:
				slog.Error("evaluations: create match", "err", err)
				writeErr(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("evaluations: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := map[string]any{"match": matched}
	if matched {
		card, err := loadCard(ctx, h.st, targetID)
		if err != nil {
			slog.Error("evaluations: load card", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		card.FirstName = target.FirstName
		card.LastName = strVal(target.LastName)
		card.Location = strVal(target.Location)
		card.MeetingFormat = string(target.MeetingFormat)
		card.TelegramUsername = target.TelegramUsername
		tgID := target.TelegramUserID
		card.TelegramUserID = &tgID
		resp["card"] = visibility.Serialize(card, visibility.StateMatched)
	}
	writeJSON(w, http.StatusCreated, resp)
}

// discloseMatch writes the contact-disclosure journal entries and the
// match_created outbox notifications for both sides. Returns false if an
// error response was already written.
func (h *EvaluationHandler) discloseMatch(ctx context.Context, qtx *store.Queries, w http.ResponseWriter, matchID pgtype.UUID, viewer store.User, target store.GetCardContactRow) bool {
	for _, d := range []store.InsertContactDisclosureParams{
		{
			MatchID:                 matchID,
			SubjectID:               viewer.ID,
			RecipientID:             target.ID,
			SubjectTelegramUserID:   viewer.TelegramUserID,
			RecipientTelegramUserID: target.TelegramUserID,
		},
		{
			MatchID:                 matchID,
			SubjectID:               target.ID,
			RecipientID:             viewer.ID,
			SubjectTelegramUserID:   target.TelegramUserID,
			RecipientTelegramUserID: viewer.TelegramUserID,
		},
	} {
		if err := qtx.InsertContactDisclosure(ctx, d); err != nil {
			slog.Error("evaluations: insert disclosure", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}
	payload, err := json.Marshal(map[string]string{"match_id": uuidString(matchID)})
	if err != nil {
		slog.Error("evaluations: marshal payload", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return false
	}
	for _, userID := range []pgtype.UUID{viewer.ID, target.ID} {
		if err := qtx.InsertNotification(ctx, store.InsertNotificationParams{
			Type:    store.NotificationTypeMatchCreated,
			UserID:  userID,
			Payload: payload,
		}); err != nil {
			slog.Error("evaluations: insert notification", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}
	return true
}

package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

const webhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

type WebhookStore interface {
	UpsertUserByTelegramID(ctx context.Context, telegramUserID int64, username *string) (store.User, error)
	SetUserChatID(ctx context.Context, telegramUserID, chatID int64) error
	ConsumeBotLinkNonce(ctx context.Context, nonce string, telegramUserID int64, ttl time.Duration) (bool, error)
	ReactivateBlockedNotifications(ctx context.Context, userID pgtype.UUID) error
}

type tgUpdate struct {
	Message *struct {
		Text string `json:"text"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func NewTelegramWebhookHandler(st WebhookStore, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(webhookSecretHeader)), []byte(secret)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid secret token"})
			return
		}
		var upd tgUpdate
		if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		msg := upd.Message
		if msg == nil || msg.From == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		if cmd, _, _ := strings.Cut(msg.Text, " "); cmd != "/start" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}

		var username *string
		if msg.From.Username != "" {
			username = &msg.From.Username
		}
		usr, err := st.UpsertUserByTelegramID(r.Context(), msg.From.ID, username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if err := st.SetUserChatID(r.Context(), msg.From.ID, msg.Chat.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if err := st.ReactivateBlockedNotifications(r.Context(), usr.ID); err != nil {
			slog.Error("webhook: reactivate notifications", "err", err)
		}
		if payload := startPayload(msg.Text); payload != "" {
			_, _ = st.ConsumeBotLinkNonce(r.Context(), payload, msg.From.ID, store.BotLinkNonceTTL)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func startPayload(text string) string {
	_, payload, found := strings.Cut(text, " ")
	if !found {
		return ""
	}
	return strings.TrimSpace(payload)
}

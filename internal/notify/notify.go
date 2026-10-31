// Package notify implements the notification outbox worker (NFR-12, ADR-002):
// due notifications are claimed from Postgres and sent via Telegram Bot API
// with configurable backoff; 403 (bot blocked) marks the notification blocked.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5"
)

type BotClient interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

var ErrBlocked = fmt.Errorf("recipient blocked the bot")

type TelegramBot struct {
	token  string
	client *http.Client
}

func NewTelegramBot(token string) *TelegramBot {
	return &TelegramBot{token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

type tgResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func (b *TelegramBot) SendMessage(ctx context.Context, chatID int64, text string) error {
	body, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+b.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var tr tgResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return fmt.Errorf("bad telegram response: %w", err)
	}
	if tr.OK {
		return nil
	}
	if tr.ErrorCode == 403 {
		return ErrBlocked
	}
	return fmt.Errorf("telegram error %d: %s", tr.ErrorCode, tr.Description)
}

var messageTexts = map[string]string{
	"match_created":        "You have a new match! Open ClubMatch to see the contact.",
	"application_approved": "Your application has been approved! Open ClubMatch to complete onboarding.",
	"application_rejected": "Your application was not approved this time. You can apply again in ClubMatch.",
	"tag_revoked":          "A tag you created was removed by moderation.",
	"last_skill_lost":      "Your profile lost its last skill due to tag moderation. Please add a skill — your card is hidden from search until then.",
}

func messageText(n store.ClaimDueNotificationsRow) string {
	text := messageTexts[n.Type]
	if n.Type == "tag_revoked" {
		var p struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(n.Payload, &p); err == nil && p.Tag != "" {
			text = fmt.Sprintf("A tag you created (%q) was removed by moderation.", p.Tag)
		}
	}
	return text
}

type Worker struct {
	st      *store.Store
	bot     BotClient
	poll    time.Duration
	backoff []time.Duration
	log     *slog.Logger
}

func NewWorker(st *store.Store, bot BotClient, poll time.Duration, backoff []time.Duration, log *slog.Logger) *Worker {
	return &Worker{st: st, bot: bot, poll: poll, backoff: backoff, log: log}
}

func (w *Worker) backoffDelay(attempts int32) time.Duration {
	i := int(attempts) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(w.backoff) {
		i = len(w.backoff) - 1
	}
	if i < 0 {
		return time.Hour
	}
	return w.backoff[i]
}

func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	rows, err := w.st.ClaimDueNotifications(ctx)
	if err != nil {
		return 0, err
	}
	for _, n := range rows {
		w.process(ctx, n)
	}
	return len(rows), nil
}

func (w *Worker) process(ctx context.Context, n store.ClaimDueNotificationsRow) {
	chatID, err := w.st.GetUserChatID(ctx, n.UserID)
	if err != nil && !isNoRows(err) {
		w.log.Error("notify: chat id", "err", err)
		w.reschedule(ctx, n)
		return
	}
	if chatID == nil {
		if err := w.st.MarkNotificationBlocked(ctx, n.ID); err != nil {
			w.log.Error("notify: mark blocked", "err", err)
		}
		return
	}
	err = w.bot.SendMessage(ctx, *chatID, messageText(n))
	switch err {
	case nil:
		if err := w.st.MarkNotificationSent(ctx, n.ID); err != nil {
			w.log.Error("notify: mark sent", "err", err)
		}
	case ErrBlocked:
		if err := w.st.MarkNotificationBlocked(ctx, n.ID); err != nil {
			w.log.Error("notify: mark blocked", "err", err)
		}
	default:
		w.log.Warn("notify: send failed", "err", err, "attempts", n.Attempts)
		w.reschedule(ctx, n)
	}
}

func (w *Worker) reschedule(ctx context.Context, n store.ClaimDueNotificationsRow) {
	delay := w.backoffDelay(n.Attempts)
	if err := w.st.RescheduleNotification(ctx, store.RescheduleNotificationParams{
		ID:   n.ID,
		Secs: delay.Seconds(),
	}); err != nil {
		w.log.Error("notify: reschedule", "err", err)
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil {
			w.log.Error("notify: process", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func isNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

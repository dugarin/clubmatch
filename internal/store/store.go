package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = pgx.ErrNoRows

const BotLinkNonceTTL = time.Hour

type Store struct {
	*Queries
}

func NewStore(db DBTX) *Store {
	return &Store{Queries: New(db)}
}

func (s *Store) UpsertUserByTelegramID(ctx context.Context, telegramUserID int64, username *string) (User, error) {
	return s.Queries.UpsertUserByTelegramID(ctx, UpsertUserByTelegramIDParams{
		TelegramUserID:   telegramUserID,
		TelegramUsername: username,
	})
}

func (s *Store) SetUserChatID(ctx context.Context, telegramUserID, chatID int64) error {
	return s.Queries.SetUserChatID(ctx, SetUserChatIDParams{
		TelegramUserID: telegramUserID,
		ChatID:         &chatID,
	})
}

func (s *Store) ConsumeBotLinkNonce(ctx context.Context, nonce string, telegramUserID int64, ttl time.Duration) (bool, error) {
	rows, err := s.Queries.ConsumeBotLinkNonce(ctx, ConsumeBotLinkNonceParams{
		Nonce:          nonce,
		TelegramUserID: telegramUserID,
		Secs:           ttl.Seconds(),
	})
	return rows == 1, err
}

package notify

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeBot struct {
	calls int
	err   error
}

func (f *fakeBot) SendMessage(_ context.Context, _ int64, _ string) error {
	f.calls++
	return f.err
}

func testWorker(t *testing.T, bot BotClient) (*Worker, *store.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE notification, users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	st := store.NewStore(pool)
	w := NewWorker(st, bot, time.Second, []time.Duration{time.Minute, 5 * time.Minute}, slog.Default())
	return w, st, pool
}

func enqueue(t *testing.T, pool *pgxpool.Pool, userID pgtype.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO notification (type, user_id, payload) VALUES ('match_created', $1, '{}')", userID); err != nil {
		t.Fatal(err)
	}
}

func notificationState(t *testing.T, pool *pgxpool.Pool, userID pgtype.UUID) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(context.Background(),
		"SELECT state::text FROM notification WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1", userID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestWorkerSendsAndMarksSent(t *testing.T) {
	w, st, pool := testWorker(t, &fakeBot{})
	ctx := context.Background()
	u, err := st.UpsertUserByTelegramID(ctx, 7_000_000_001, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserChatID(ctx, 7_000_000_001, 42); err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, u.ID)
	if _, err := w.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := notificationState(t, pool, u.ID); got != "sent" {
		t.Errorf("state = %s, want sent", got)
	}
}

func TestWorkerBlockedWithoutChatID(t *testing.T) {
	w, st, pool := testWorker(t, &fakeBot{})
	ctx := context.Background()
	u, err := st.UpsertUserByTelegramID(ctx, 7_000_000_002, nil)
	if err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, u.ID)
	if _, err := w.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := notificationState(t, pool, u.ID); got != "blocked" {
		t.Errorf("state = %s, want blocked (no chat link)", got)
	}
}

func TestWorkerBlockedOn403(t *testing.T) {
	w, st, pool := testWorker(t, &fakeBot{err: ErrBlocked})
	ctx := context.Background()
	u, err := st.UpsertUserByTelegramID(ctx, 7_000_000_003, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserChatID(ctx, 7_000_000_003, 43); err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, u.ID)
	if _, err := w.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := notificationState(t, pool, u.ID); got != "blocked" {
		t.Errorf("state = %s, want blocked (403)", got)
	}
}

func TestWorkerRetryOnError(t *testing.T) {
	w, st, pool := testWorker(t, &fakeBot{err: errors.New("network down")})
	ctx := context.Background()
	u, err := st.UpsertUserByTelegramID(ctx, 7_000_000_004, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserChatID(ctx, 7_000_000_004, 44); err != nil {
		t.Fatal(err)
	}
	enqueue(t, pool, u.ID)
	if _, err := w.ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var next time.Time
	if err := pool.QueryRow(ctx,
		"SELECT attempts, next_attempt_at FROM notification WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1",
		u.ID).Scan(&attempts, &next); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if time.Until(next) < 30*time.Second {
		t.Errorf("next_attempt_at not rescheduled with backoff: %v", next)
	}
	if got := notificationState(t, pool, u.ID); got != "pending" {
		t.Errorf("state = %s, want pending (retry)", got)
	}
}

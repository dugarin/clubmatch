package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testStore(t *testing.T) *Store {
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
	return NewStore(pool)
}

var tgSeq = time.Now().UnixNano() % 1_000_000_000

func nextTG() int64 {
	tgSeq++
	return 5_000_000_000 + tgSeq
}

func TestUpsertUserByTelegramID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()

	u1, err := s.UpsertUserByTelegramID(ctx, tg, nil)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if u1.TelegramUserID != tg || u1.TelegramUsername != nil {
		t.Errorf("u1 = %+v", u1)
	}
	if u1.Role != "candidate" || u1.AccessState != "none" {
		t.Errorf("defaults: role=%s state=%s", u1.Role, u1.AccessState)
	}

	name := "jdoe"
	u2, err := s.UpsertUserByTelegramID(ctx, tg, &name)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if u2.ID != u1.ID {
		t.Errorf("upsert created new row: %s != %s", u2.ID, u1.ID)
	}
	if u2.TelegramUsername == nil || *u2.TelegramUsername != "jdoe" {
		t.Errorf("username snapshot = %v", u2.TelegramUsername)
	}
}

func TestSetUserChatID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()
	if _, err := s.UpsertUserByTelegramID(ctx, tg, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserChatID(ctx, tg, 4242); err != nil {
		t.Fatalf("SetUserChatID: %v", err)
	}
	u, err := s.UpsertUserByTelegramID(ctx, tg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.ChatID == nil || *u.ChatID != 4242 {
		t.Errorf("chat_id = %v", u.ChatID)
	}
}

func TestProfileRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()
	u, err := s.UpsertUserByTelegramID(ctx, tg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProfileByUserID(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	loc := "Berlin"
	p := UpsertProfileParams{UserID: u.ID, FirstName: "Ann", Location: &loc, LocationNorm: &loc, MeetingFormat: "online"}
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	got, err := s.GetProfileByUserID(ctx, u.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.FirstName != "Ann" || got.MeetingFormat != "online" || !got.IsSearchable {
		t.Errorf("profile = %+v", got)
	}
	p.FirstName = "Anna"
	p.MeetingFormat = "any"
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got, err = s.GetProfileByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName != "Anna" || got.MeetingFormat != "any" {
		t.Errorf("updated profile = %+v", got)
	}
}

func TestApplicationCreateAndGetPending(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()
	u, err := s.UpsertUserByTelegramID(ctx, tg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPendingApplicationByCandidate(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	a, err := s.CreateApplication(ctx, u.ID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.State != "pending" {
		t.Errorf("state = %s", a.State)
	}
	got, err := s.GetPendingApplicationByCandidate(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID {
		t.Errorf("got %s, want %s", got.ID, a.ID)
	}
}

func TestInviteLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()
	u, err := s.UpsertUserByTelegramID(ctx, tg, nil)
	if err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("tok-%d", tg)
	v, err := s.CreateInvite(ctx, CreateInviteParams{Token: token, CreatedBy: u.ID})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	got, err := s.GetInviteByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v.ID || got.UsedBy.Valid {
		t.Errorf("invite = %+v", got)
	}
	rows, err := s.MarkInviteUsed(ctx, MarkInviteUsedParams{Token: token, UsedBy: u.ID})
	if err != nil || rows != 1 {
		t.Fatalf("mark used: rows=%d err=%v", rows, err)
	}
	rows, err = s.MarkInviteUsed(ctx, MarkInviteUsedParams{Token: token, UsedBy: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Error("second use must fail (one-shot invite)")
	}
}

func TestBotLinkNonce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tg := nextTG()
	if _, err := s.UpsertUserByTelegramID(ctx, tg, nil); err != nil {
		t.Fatal(err)
	}
	nonce := fmt.Sprintf("nonce-%d", tg)
	if err := s.CreateBotLinkNonce(ctx, CreateBotLinkNonceParams{Nonce: nonce, TelegramUserID: tg}); err != nil {
		t.Fatalf("create nonce: %v", err)
	}
	ok, err := s.ConsumeBotLinkNonce(ctx, nonce, tg, time.Hour)
	if err != nil || !ok {
		t.Fatalf("consume: ok=%v err=%v", ok, err)
	}
	ok, err = s.ConsumeBotLinkNonce(ctx, nonce, tg, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("nonce reuse must fail")
	}
	ok, err = s.ConsumeBotLinkNonce(ctx, nonce, nextTG(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("nonce of another telegram user must not be consumed")
	}
	ok, err = s.ConsumeBotLinkNonce(ctx, "missing-nonce", tg, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("unknown nonce must not be consumed")
	}
}

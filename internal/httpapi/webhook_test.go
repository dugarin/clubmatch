package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
)

const testSecret = "test-webhook-secret"

type fakeWebhookStore struct {
	upsertCalls  int
	upsertID     int64
	upsertUN     *string
	chatCalls    int
	chatUserID   int64
	chatID       int64
	nonceCalls   int
	nonceValue   string
	nonceUserID  int64
	nonceTTL     time.Duration
	nonceConsume bool
}

func (f *fakeWebhookStore) UpsertUserByTelegramID(_ context.Context, id int64, username *string) (store.User, error) {
	f.upsertCalls++
	f.upsertID = id
	f.upsertUN = username
	return store.User{}, nil
}

func (f *fakeWebhookStore) SetUserChatID(_ context.Context, telegramUserID, chatID int64) error {
	f.chatCalls++
	f.chatUserID = telegramUserID
	f.chatID = chatID
	return nil
}

func (f *fakeWebhookStore) ConsumeBotLinkNonce(_ context.Context, nonce string, telegramUserID int64, ttl time.Duration) (bool, error) {
	f.nonceCalls++
	f.nonceValue = nonce
	f.nonceUserID = telegramUserID
	f.nonceTTL = ttl
	return f.nonceConsume, nil
}

func runWebhook(t *testing.T, st *fakeWebhookStore, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	req.Header.Set(webhookSecretHeader, secret)
	rec := httptest.NewRecorder()
	NewTelegramWebhookHandler(st, testSecret).ServeHTTP(rec, req)
	return rec
}

func TestWebhookValidSecretStartWithNonce(t *testing.T) {
	st := &fakeWebhookStore{nonceConsume: true}
	body := `{"message":{"text":"/start abc123nonce","from":{"id":111,"username":"alice"},"chat":{"id":111}}}`
	rec := runWebhook(t, st, testSecret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}
	if st.upsertCalls != 1 || st.upsertID != 111 {
		t.Errorf("upsert calls=%d id=%d", st.upsertCalls, st.upsertID)
	}
	if st.upsertUN == nil || *st.upsertUN != "alice" {
		t.Errorf("username = %v", st.upsertUN)
	}
	if st.chatCalls != 1 || st.chatID != 111 {
		t.Errorf("chat link calls=%d chatID=%d", st.chatCalls, st.chatID)
	}
	if st.nonceCalls != 1 || st.nonceValue != "abc123nonce" || st.nonceUserID != 111 {
		t.Errorf("nonce calls=%d value=%q user=%d", st.nonceCalls, st.nonceValue, st.nonceUserID)
	}
	if st.nonceTTL != store.BotLinkNonceTTL {
		t.Errorf("nonce ttl = %v, want %v", st.nonceTTL, store.BotLinkNonceTTL)
	}
}

func TestWebhookStartWithoutPayload(t *testing.T) {
	st := &fakeWebhookStore{}
	body := `{"message":{"text":"/start","from":{"id":222},"chat":{"id":222}}}`
	rec := runWebhook(t, st, testSecret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if st.upsertCalls != 1 || st.chatCalls != 1 {
		t.Errorf("upsert=%d chat=%d", st.upsertCalls, st.chatCalls)
	}
	if st.upsertUN != nil {
		t.Errorf("username must be nil, got %v", *st.upsertUN)
	}
	if st.nonceCalls != 0 {
		t.Error("nonce must not be consumed without payload")
	}
}

func TestWebhookInvalidSecret(t *testing.T) {
	st := &fakeWebhookStore{}
	body := `{"message":{"text":"/start","from":{"id":1},"chat":{"id":1}}}`
	rec := runWebhook(t, st, "wrong-secret", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if st.upsertCalls != 0 || st.chatCalls != 0 || st.nonceCalls != 0 {
		t.Error("store must not be touched on invalid secret")
	}
}

func TestWebhookMissingSecret(t *testing.T) {
	st := &fakeWebhookStore{}
	rec := runWebhook(t, st, "", `{"message":{"text":"/start","from":{"id":1},"chat":{"id":1}}}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestWebhookNotStartCommand(t *testing.T) {
	st := &fakeWebhookStore{}
	body := `{"message":{"text":"hello","from":{"id":1},"chat":{"id":1}}}`
	rec := runWebhook(t, st, testSecret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if st.upsertCalls != 0 {
		t.Error("non-/start message must be ignored")
	}
}

func TestWebhookNonMessageUpdate(t *testing.T) {
	st := &fakeWebhookStore{}
	rec := runWebhook(t, st, testSecret, `{"update_id":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if st.upsertCalls != 0 {
		t.Error("update without message must be ignored")
	}
}

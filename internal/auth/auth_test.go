package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
)

const testToken = "123456:ABC-DEF-test-token"

func signInitData(t *testing.T, fields map[string]string) string {
	t.Helper()
	var dcs []string
	for k, v := range fields {
		dcs = append(dcs, k+"="+v)
	}
	sort.Strings(dcs)
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(testToken))
	sigMAC := hmac.New(sha256.New, secretMAC.Sum(nil))
	sigMAC.Write([]byte(strings.Join(dcs, "\n")))
	hash := hex.EncodeToString(sigMAC.Sum(nil))

	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hash)
	return q.Encode()
}

func validFields(authDate time.Time) map[string]string {
	return map[string]string{
		"auth_date": fmt.Sprint(authDate.Unix()),
		"user":      `{"id":123456789,"username":"jdoe","first_name":"John","last_name":"Doe"}`,
	}
}

type fakeUpserter struct {
	calls  int
	lastID int64
	lastUN *string
	err    error
}

func (f *fakeUpserter) UpsertUserByTelegramID(_ context.Context, id int64, username *string) (store.User, error) {
	f.calls++
	f.lastID = id
	f.lastUN = username
	return store.User{}, f.err
}

func TestParseAndValidateValid(t *testing.T) {
	now := time.Now()
	raw := signInitData(t, validFields(now.Add(-time.Hour)))
	id, err := ParseAndValidate(testToken, raw, 24*time.Hour, now)
	if err != nil {
		t.Fatalf("ParseAndValidate: %v", err)
	}
	if id.TelegramUserID != 123456789 {
		t.Errorf("TelegramUserID = %d", id.TelegramUserID)
	}
	if id.Username != "jdoe" || id.FirstName != "John" || id.LastName != "Doe" {
		t.Errorf("unexpected identity: %+v", id)
	}
}

func TestParseAndValidateTampered(t *testing.T) {
	now := time.Now()
	raw := signInitData(t, validFields(now))
	raw = strings.Replace(raw, "jdoe", "evil", 1)
	if _, err := ParseAndValidate(testToken, raw, 24*time.Hour, now); !errors.Is(err, ErrBadSign) {
		t.Fatalf("err = %v, want ErrBadSign", err)
	}
}

func TestParseAndValidateExpired(t *testing.T) {
	now := time.Now()
	raw := signInitData(t, validFields(now.Add(-25*time.Hour)))
	if _, err := ParseAndValidate(testToken, raw, 24*time.Hour, now); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestParseAndValidateNoHash(t *testing.T) {
	if _, err := ParseAndValidate(testToken, "auth_date=1&user=%7B%7D", time.Hour, time.Now()); !errors.Is(err, ErrNoHash) {
		t.Fatalf("err = %v, want ErrNoHash", err)
	}
}

func runMiddleware(t *testing.T, up *fakeUpserter, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	var gotID Identity
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	h := Middleware(up, testToken, 24*time.Hour)(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusNoContent && !gotOK {
		t.Error("handler called but identity missing in context")
	}
	_ = gotID
	return rec
}

func TestMiddlewareValid(t *testing.T) {
	up := &fakeUpserter{}
	raw := signInitData(t, validFields(time.Now()))
	rec := runMiddleware(t, up, "tma "+raw)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, body %s", rec.Code, rec.Body)
	}
	if up.calls != 1 || up.lastID != 123456789 {
		t.Errorf("upsert calls=%d id=%d", up.calls, up.lastID)
	}
	if up.lastUN == nil || *up.lastUN != "jdoe" {
		t.Errorf("username snapshot = %v", up.lastUN)
	}
}

func TestMiddlewareMissingHeader(t *testing.T) {
	up := &fakeUpserter{}
	rec := runMiddleware(t, up, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if up.calls != 0 {
		t.Error("upsert must not be called on 401")
	}
}

func TestMiddlewareWrongScheme(t *testing.T) {
	up := &fakeUpserter{}
	rec := runMiddleware(t, up, "Bearer abc")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestMiddlewareTampered(t *testing.T) {
	up := &fakeUpserter{}
	raw := signInitData(t, validFields(time.Now()))
	raw = strings.Replace(raw, "jdoe", "evil", 1)
	rec := runMiddleware(t, up, "tma "+raw)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestMiddlewareExpired(t *testing.T) {
	up := &fakeUpserter{}
	raw := signInitData(t, validFields(time.Now().Add(-25*time.Hour)))
	rec := runMiddleware(t, up, "tma "+raw)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

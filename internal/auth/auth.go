package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clubmatch/clubmatch/internal/store"
)

var (
	ErrMalformed   = errors.New("malformed initData")
	ErrNoHash      = errors.New("initData without hash")
	ErrBadSign     = errors.New("invalid initData signature")
	ErrExpired     = errors.New("initData auth_date expired")
	ErrNoAuth      = errors.New("missing Authorization header")
	ErrBadAuthType = errors.New("unsupported authorization scheme")
)

type Identity struct {
	TelegramUserID int64
	Username       string
	FirstName      string
	LastName       string
	AuthDate       time.Time
}

type tgUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type UserUpserter interface {
	UpsertUserByTelegramID(ctx context.Context, telegramUserID int64, username *string) (store.User, error)
}

func ParseAndValidate(botToken, initData string, ttl time.Duration, now time.Time) (Identity, error) {
	var hash string
	var pairs []string
	for _, p := range strings.Split(initData, "&") {
		k, v, _ := strings.Cut(p, "=")
		key, err := url.QueryUnescape(k)
		if err != nil {
			return Identity{}, ErrMalformed
		}
		value, err := url.QueryUnescape(v)
		if err != nil {
			return Identity{}, ErrMalformed
		}
		if key == "hash" {
			hash = value
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	if hash == "" {
		return Identity{}, ErrNoHash
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	sigMAC := hmac.New(sha256.New, secretMAC.Sum(nil))
	sigMAC.Write([]byte(dataCheckString))
	expected, err := hex.DecodeString(hash)
	if err != nil || !hmac.Equal(sigMAC.Sum(nil), expected) {
		return Identity{}, ErrBadSign
	}

	q, err := url.ParseQuery(initData)
	if err != nil {
		return Identity{}, ErrMalformed
	}
	authUnix, err := strconv.ParseInt(q.Get("auth_date"), 10, 64)
	if err != nil {
		return Identity{}, ErrMalformed
	}
	authDate := time.Unix(authUnix, 0)
	if now.Sub(authDate) > ttl {
		return Identity{}, ErrExpired
	}

	var u tgUser
	if err := json.Unmarshal([]byte(q.Get("user")), &u); err != nil || u.ID == 0 {
		return Identity{}, ErrMalformed
	}
	return Identity{
		TelegramUserID: u.ID,
		Username:       u.Username,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		AuthDate:       authDate,
	}, nil
}

type ctxKey struct{}

func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

func NewContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

func Middleware(users UserUpserter, botToken string, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, initData, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok {
				writeError(w, http.StatusUnauthorized, ErrNoAuth.Error())
				return
			}
			if scheme != "tma" {
				writeError(w, http.StatusUnauthorized, ErrBadAuthType.Error())
				return
			}
			id, err := ParseAndValidate(botToken, initData, ttl, time.Now())
			if err != nil {
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}
			var username *string
			if id.Username != "" {
				username = &id.Username
			}
			if _, err := users.UpsertUserByTelegramID(r.Context(), id.TelegramUserID, username); err != nil {
				slog.Error("auth: upsert user", "err", err)
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
		})
	}
}

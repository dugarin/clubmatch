package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/clubmatch/clubmatch/internal/config"
	"github.com/clubmatch/clubmatch/internal/devmode"
)

func TestDevAuthPreview(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := devmode.Seed(ctx, e.pool); err != nil {
			t.Fatalf("seed %d: %v", i+1, err)
		}
	}
	var users, matches, projects int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE telegram_user_id BETWEEN 900000000001 AND 900000000004").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM match").Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM project").Scan(&projects); err != nil {
		t.Fatal(err)
	}
	if users != 4 || matches != 1 || projects != 4 {
		t.Fatalf("seed is not idempotent: users=%d matches=%d projects=%d", users, matches, projects)
	}

	h := NewRouter(e.pool, config.Config{Env: "dev", DevAuthEnabled: true})
	for _, tc := range []struct {
		path     string
		contains string
	}{
		{path: "/api/v1/profiles/me", contains: `"first_name":"Alex"`},
		{path: "/api/v1/deck", contains: "Telegram Mini App interface"},
		{path: "/api/v1/matches", contains: `"telegram_username":"demo_maria"`},
		{path: "/api/v1/search?q=Kafka", contains: "Reliable event pipeline with Kafka"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
			}
			body, _ := io.ReadAll(rec.Body)
			if !strings.Contains(string(body), tc.contains) {
				t.Fatalf("body %s does not contain %s", body, tc.contains)
			}
		})
	}
}

func TestDevAuthDoesNotBypassOutsideDev(t *testing.T) {
	e := newTestEnv(t)
	for _, tc := range []struct {
		name string
		cfg  config.Config
	}{
		{name: "disabled", cfg: config.Config{Env: "dev"}},
		{name: "stage", cfg: config.Config{Env: "stage", DevAuthEnabled: true}},
		{name: "prod", cfg: config.Config{Env: "prod", DevAuthEnabled: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(e.pool, tc.cfg)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/profiles/me", nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("code = %d, want 401; body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

package devmode

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clubmatch/clubmatch/internal/auth"
)

func TestMiddlewareSetsDemoIdentity(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok {
			t.Fatal("identity is missing")
		}
		if id.TelegramUserID != DemoTelegramUserID || id.Username != DemoUsername {
			t.Fatalf("unexpected identity: %+v", id)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	Middleware()(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
}

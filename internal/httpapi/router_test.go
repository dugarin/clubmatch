package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clubmatch/clubmatch/internal/config"
)

func TestPublicRouterMetricsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	NewRouter(nil, config.Config{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

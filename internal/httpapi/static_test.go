package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html>app</html>")},
		"assets/app.js":  {Data: []byte("js")},
		"assets/app.css": {Data: []byte("css")},
	}
}

func TestStaticHandlerServesIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	newStaticHandler(testDistFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "<html>app</html>") {
		t.Fatalf("body = %q", body)
	}
}

func TestStaticHandlerServesAssetWithCacheHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	newStaticHandler(testDistFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q", cc)
	}
}

func TestStaticHandlerSPAFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	newStaticHandler(testDistFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/matches/unknown-route", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "<html>app</html>") {
		t.Fatalf("fallback body = %q", body)
	}
}

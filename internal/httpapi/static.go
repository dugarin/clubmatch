package httpapi

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/clubmatch/clubmatch/web"
)

// newStaticHandler отдаёт собранный Mini App из embed.FS (D-6.1).
// SPA-fallback: неизвестный путь без файла в dist → index.html (роутинг на клиенте).
// Хэшированные ассеты vite (/assets/*) кэшируются агрессивно.
func newStaticHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(distFS, p); err != nil {
				r.URL.Path = "/"
			} else if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

// embeddedDist — собранный Mini App из бинаря; паника при сборке без web/dist
// невозможна: .gitkeep гарантирует непустой embed.
func embeddedDist() fs.FS {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}

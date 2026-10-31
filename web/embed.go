// Package web поставляет собранный Mini App (web/dist) как embed.FS.
// Решение D-6.1 (docs/dev-open-questions.md): dist встраивается в Go-бинарь —
// один артефакт, отдельный контейнер/Caddy для статики не нужен.
package web

import "embed"

// Dist — результат `npm run build` в web/. В репозитории dist не хранится
// (см. .gitignore); для сборки без фронтенда в dist лежит .gitkeep-заглушка.
// Реальная сборка образа — стадия web в Dockerfile.
//
//go:embed all:dist
var Dist embed.FS

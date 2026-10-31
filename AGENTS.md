# AGENTS.md — ClubMatch (clubmatch)

A service for professional community member matching: member profiles,
recommendations (card deck), people search on demand, onboarding via
applications/invitations. Interface — Telegram Mini App + Telegram bot
for notifications.

## Language rules

- **Conversation with the user is held in Russian.** All agent replies in chat
  must be in Russian.
- **Requirements and architecture documentation is written in Russian**:
  `README.md`, everything under `docs/` (requirements, ADR, spikes, ops,
  roadmap). Keep this language when editing those files.
- **Code and model reasoning are in English**: identifiers, code comments,
  commit messages, technical artifacts not listed above (this file, configs'
  inline comments). The agent's internal reasoning (plans, analysis) is also
  done in English.

## Stack

- **Backend**: Go 1.26, HTTP router `chi/v5`, `pgx/v5` (connection pool), Prometheus metrics.
- **DB**: PostgreSQL 16 (`pgcrypto`, `pg_trgm` extensions, full-text search
  with the `russian` configuration).
- **DB layer generation**: sqlc (v1.31.1) — generates `internal/store/*.sql.go` from `db/queries/*.sql`.
- **Migrations**: goose (v3.24.1), files `db/migrations/NNNNN_*.sql` with `-- +goose Up/Down` markers.
- **Frontend**: React 18 + TypeScript + Vite 5 (`web/` directory); the built `dist`
  is embedded into the Go binary via `go:embed` (`web/embed.go`).
- **Infrastructure**: Docker (single multi-stage `Dockerfile`, `BINARY` argument),
  docker-compose (dev), `docker-compose.stage.yml` + Caddy (auto-TLS) + Prometheus (stage).
- **CI/CD**: GitHub Actions (`lint` → `go test` → docker build + ops config validation
  + frontend lint/build; SSH deploy — `deploy.yml`, gated by feature flags).

## Architecture and processes

Two binaries from one codebase:

- `cmd/server` — HTTP API (`:8080`), Telegram webhook, serves the embedded SPA.
  On startup assigns moderators from `MODERATOR_TG_IDS`.
- `cmd/worker` — notification worker: polls the `notification` outbox table
  (`WORKER_POLL_SECONDS`), sends messages via Telegram Bot API with a backoff
  policy (1/5/15/60 minutes), exposes metrics on `MetricsAddr` (`:9091`).

API authentication — Telegram Mini App `initData` validation (`internal/auth`,
TTL from `INITDATA_TTL_HOURS`). Moderator access is checked by `RequireModerator`
against the user's role in the DB.

## Directory layout

- `cmd/server`, `cmd/worker` — entry points of the two processes.
- `internal/httpapi` — HTTP layer: router (`router.go` — the single place for routes
  `/api/v1/*`, `/metrics`, `/healthz`, `/readyz`, `/telegram/webhook`), handlers
  by domain (profiles, deck, evaluations, search, matches, applications,
  moderation, delete, webhook, static).
- `internal/store` — sqlc-generated code (`*.sql.go`, `models.go`) + a thin
  hand-written layer (`store.go`, `tags.go`). `ErrNotFound = pgx.ErrNoRows`.
- `internal/auth` — initData validation middleware.
- `internal/config` — configuration from environment variables (see `.env.example`).
- `internal/notify` — notification worker and Telegram client.
- `internal/observ` — Prometheus metrics and latency middleware.
- `internal/logging` — `slog` setup.
- `internal/visibility` — privacy/visibility rules (ADR-004).
- `internal/profile` — profile domain logic.
- `db/migrations` — goose migrations; `db/queries` — SQL for sqlc
  (`-- name: X :one/:many` annotations).
- `web/` — Mini App: `src/` (React components by screen: Onboarding, Profile,
  Deck, Search, Matches, etc.), `embed.go` (`go:embed all:dist`).
- `ops/` — Caddyfile, Prometheus configs, `backup.sh`/`restore.sh` scripts (pg_dump, rotation by `RETENTION_DAYS`).
- `docs/` — specification and architecture: traceability chain
  glossary → FR → NFR → UC → AC (`docs/requirements/`), ADR 001–004 and OpenAPI
  (`docs/architecture/`), spikes (`docs/spikes/`), operations (`docs/ops/`),
  roadmap (`docs/roadmap.md`).

## Build and development commands

Via Makefile:

- `make build` — `go build ./...`
- `make test` — `go test -p 1 ./...` (packages run sequentially)
- `make lint` — `golangci-lint run` (config `.golangci.yml`, v2 format;
  version v2.15.0 is pinned in CI)
- `make sqlc` — regenerate `internal/store` from `db/queries` (run after
  changing SQL queries or schema)
- `make migrate-up` / `make migrate-down` — goose migrations (requires `DATABASE_URL`)
- `make dev` — `docker compose --profile tools up --build` (postgres + migrations + server + worker)

Frontend (in `web/`):

- `npm ci`, `npm run dev` (vite, proxies `/api` → `localhost:8080`)
- `npm run build` — `tsc -b && vite build`
- `npm run lint` — eslint

Full image build (both binaries from one Dockerfile):

```bash
docker build --build-arg BINARY=server -t clubmatch-server .
docker build --build-arg BINARY=worker -t clubmatch-worker .
```

`web/dist` is not stored in git; for Go builds without the frontend it contains
a `.gitkeep` placeholder. The real dist is provided by the `web` stage in the
Dockerfile (Node 20).

## Configuration and secrets

All configuration comes from environment variables (`internal/config/config.go`,
template `.env.example`; `.env` is not committed, `chmod 600` on the server).
Required: `DATABASE_URL`. Secrets: `TELEGRAM_BOT_TOKEN`,
`TELEGRAM_WEBHOOK_SECRET`, `POSTGRES_PASSWORD` — must never end up in logs or commits.
Moderator list: `MODERATOR_TG_IDS` (comma-separated telegram ids).

## Code conventions

- **Code comments are written in English; documentation in `docs/` and
  `README.md` — in Russian** (see Language rules).
- Code and docs use requirement/decision traceability references
  (FR-x, NFR-x, AC-x, Q-x, D-x, ADR-xxx) — keep these links when making changes
  and update the corresponding documents in `docs/`.
- SQL queries live in `db/queries/*.sql` with sqlc annotations; generated code
  (`internal/store/*.sql.go`, `models.go`) must not be edited by hand — edit
  the queries and run `make sqlc`.
- Migrations are created as new files `db/migrations/NNNNN_name.sql` with
  `-- +goose Up` and `-- +goose Down` sections; existing applied migrations
  must not be modified.
- Go linters: `errcheck`, `govet`, `staticcheck`, `unused`.
- HTTP handlers respond with JSON via `writeJSON`; routes are added in
  `internal/httpapi/router.go`.
- Frontend: eslint config — `js.configs.recommended` + `typescript-eslint recommended`.

## Testing

- `make test` / `go test -p 1 ./...`; in CI — with `-race`.
- Unit tests do not require a DB; integration tests (`internal/store`, part of
  `internal/httpapi`, `internal/notify`) are skipped unless `TEST_DATABASE_URL`
  is set. Locally, bring the DB up with `docker compose up db`
  (login/password/db — `clubmatch`), then `make migrate-up` and
  `TEST_DATABASE_URL=postgres://clubmatch:clubmatch@localhost:5432/clubmatch?sslmode=disable go test ./...`.
- Integration tests run `TRUNCATE ... CASCADE` on the test DB themselves — never
  run them against a database with valuable data.
- CI (`.github/workflows/ci.yml`): golangci-lint, go test with a postgres:16
  service, build of both docker images (no push — registry not chosen yet, ADR-001),
  validation of compose/prometheus/caddy configs, frontend lint+build.

## Deployment and operations

- Dev: `docker-compose.yml` (postgres + migrate + server + worker).
- Stage: `docker-compose.stage.yml` — same services + Caddy (automatic TLS via
  `STAGE_DOMAIN`) + `monitoring` profile (Prometheus, node-exporter).
  First-deploy order and server requirements — `docs/ops/stage.md`.
- CD (`.github/workflows/deploy.yml`): SSH deploy from git (build on the server),
  stage — push to `main`, prod — `v*` tags; both jobs are disabled by feature flags
  `DEPLOY_STAGE_ENABLED` / `DEPLOY_PROD_ENABLED` until secrets are configured.
  Procedure and rollback — `docs/ops/deploy.md`.
- Images are not published to a registry yet (open question Q-1.2, ADR-001).
- Backups: `ops/backup.sh` (pg_dump, `RETENTION_DAYS` rotation); restore — `ops/restore.sh`.
- Health/readiness: `GET /healthz`, `GET /readyz`; metrics: `/metrics` (server
  on the main port, worker on `METRICS_ADDR`).

.PHONY: dev build test lint migrate-up migrate-down sqlc

SQLC_VERSION := v1.31.1

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

build:
	go build ./...

test:
	go test -p 1 ./...

lint:
	golangci-lint run

migrate-up:
	go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$${DATABASE_URL}" up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$${DATABASE_URL}" down

dev:
	docker compose --profile tools up --build

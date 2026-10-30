.PHONY: dev build test lint migrate-up migrate-down

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run

migrate-up:
	go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$${DATABASE_URL}" up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$${DATABASE_URL}" down

dev:
	docker compose --profile tools up --build

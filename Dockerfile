# Сборка Mini App (D-6.1): dist встраивается в Go-бинарь (web/embed.go).
# Версия Node — как в CI (.github/workflows/ci.yml, job frontend).
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Реальный dist вместо .gitkeep-заглушки (dist в .gitignore — из контекста его нет)
COPY --from=web /web/dist ./web/dist
ARG BINARY=server
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${BINARY}

FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]

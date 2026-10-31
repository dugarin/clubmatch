FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BINARY=server
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${BINARY}

FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]

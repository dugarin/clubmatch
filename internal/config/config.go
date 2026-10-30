package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	TelegramBotToken  string
	WebhookSecret     string
	InitDataTTL       time.Duration
	WorkerPoll        time.Duration
	BackoffMinutes    []int
	Env               string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() (Config, error) {
	ttl, err := strconv.Atoi(getenv("INITDATA_TTL_HOURS", "24"))
	if err != nil {
		return Config{}, fmt.Errorf("INITDATA_TTL_HOURS: %w", err)
	}
	poll, err := strconv.Atoi(getenv("WORKER_POLL_SECONDS", "30"))
	if err != nil {
		return Config{}, fmt.Errorf("WORKER_POLL_SECONDS: %w", err)
	}
	cfg := Config{
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		WebhookSecret:    os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		InitDataTTL:      time.Duration(ttl) * time.Hour,
		WorkerPoll:       time.Duration(poll) * time.Second,
		BackoffMinutes:   []int{1, 5, 15, 60},
		Env:              getenv("APP_ENV", "dev"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

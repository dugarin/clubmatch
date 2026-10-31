package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	TelegramBotToken string
	WebhookSecret    string
	MetricsAddr      string
	ModeratorTGIDs   []int64
	BotUsername      string
	MiniAppName      string
	InitDataTTL      time.Duration
	WorkerPoll       time.Duration
	BackoffMinutes   []int
	Env              string
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
		MetricsAddr:      getenv("METRICS_ADDR", ":9091"),
		ModeratorTGIDs:   parseIDList(os.Getenv("MODERATOR_TG_IDS")),
		BotUsername:      os.Getenv("TELEGRAM_BOT_USERNAME"),
		MiniAppName:      os.Getenv("TELEGRAM_MINIAPP_NAME"),
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

func parseIDList(raw string) []int64 {
	var out []int64
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

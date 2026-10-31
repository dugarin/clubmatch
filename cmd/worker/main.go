package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clubmatch/clubmatch/internal/config"
	"github.com/clubmatch/clubmatch/internal/logging"
	"github.com/clubmatch/clubmatch/internal/notify"
	"github.com/clubmatch/clubmatch/internal/observ"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	log := logging.New(cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	backoff := make([]time.Duration, 0, len(cfg.BackoffMinutes))
	for _, m := range cfg.BackoffMinutes {
		backoff = append(backoff, time.Duration(m)*time.Minute)
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", observ.MetricsHandler())
		if err := http.ListenAndServe(cfg.MetricsAddr, mux); err != nil {
			log.Error("metrics server", "err", err)
		}
	}()
	w := notify.NewWorker(store.NewStore(db), notify.NewTelegramBot(cfg.TelegramBotToken), cfg.WorkerPoll, backoff, log)
	log.Info("notification worker started", "poll", cfg.WorkerPoll)
	w.Run(ctx)
	log.Info("worker stopped")
}

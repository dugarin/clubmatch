package logging

import (
	"log/slog"
	"os"
)

func New(env string) *slog.Logger {
	var h slog.Handler
	if env == "dev" {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	return slog.New(h)
}

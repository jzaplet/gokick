package main

import (
	"context"
	"gokick/app/infrastructure/di"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Best-effort: load .env before building the logger so APP_LOG_* take
	// effect locally. config.LoadConfig loads .env again later; godotenv.Load
	// never overrides already-set vars, so the double call is harmless.
	_ = godotenv.Load()

	logger := newLogger(os.Getenv("APP_LOG_FORMAT"), parseLogLevel(os.Getenv("APP_LOG_LEVEL")))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := di.CreateApplication(logger)
	if err != nil {
		logger.Error("failed to create application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}

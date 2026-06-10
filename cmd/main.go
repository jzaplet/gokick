package main

import (
	"context"
	"gokick/app/infrastructure/di"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const sentryFlushTimeout = 2 * time.Second

func main() {
	// Best-effort: load .env before building the logger so APP_LOG_* / APP_SENTRY_*
	// take effect locally. config.LoadConfig loads .env again later; godotenv.Load
	// never overrides already-set vars, so the double call is harmless.
	_ = godotenv.Load()

	logger := newLogger(os.Getenv("APP_LOG_FORMAT"), parseLogLevel(os.Getenv("APP_LOG_LEVEL")))
	slog.SetDefault(logger)
	logger.Info("starting gokick", "version", releaseVersion())

	reporter, err := newErrorReporter(
		os.Getenv("APP_SENTRY_DSN"),
		os.Getenv("APP_SENTRY_ENVIRONMENT"),
		releaseVersion(),
	)
	if err != nil {
		logger.Error("failed to initialize error reporter", "error", err)
		os.Exit(1)
	}
	// Flush on normal return and during panic unwinding. os.Exit skips defers,
	// so the error paths below flush explicitly before exiting.
	defer reporter.Flush(sentryFlushTimeout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := di.CreateApplication(logger, reporter)
	if err != nil {
		logger.Error("failed to create application", "error", err)
		reporter.Flush(sentryFlushTimeout)
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil {
		logger.Error("application error", "error", err)
		reporter.Flush(sentryFlushTimeout)
		os.Exit(1)
	}
}

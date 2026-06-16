package main

import (
	"context"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/di"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const sentryFlushTimeout = 2 * time.Second

func main() {
	// The logger and error reporter are built before the full Config loads, so a
	// failure inside LoadConfig itself can still be logged and reported. They read
	// their env through config.LoadStartup (which loads .env) so cmd/ never calls
	// os.Getenv directly.
	startup := config.LoadStartup()
	sentryEnabled := startup.SentryDSN != ""
	version := releaseVersion(startup.SentryRelease)

	logger := newLogger(
		startup.LogFormat,
		parseLogLevel(startup.LogLevel),
		sentryEnabled,
	)
	slog.SetDefault(logger)
	logger.Info("starting gokick", "version", version)

	reporter, err := newErrorReporter(startup.SentryDSN, startup.SentryEnvironment, version)
	if err != nil {
		logger.Error("failed to initialize error reporter", "error", err)
		os.Exit(1)
	}
	// Surface a common footgun: with a DSN but no environment, both backend
	// (default) and frontend (meta-tag fallback) tag events "development" — so
	// production errors would hide under the dev environment unless flagged.
	if sentryEnabled && startup.SentryEnvironment == "" {
		logger.Warn(
			"APP_SENTRY_ENVIRONMENT is empty — Sentry events will be tagged 'development'; set it explicitly in production",
		)
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

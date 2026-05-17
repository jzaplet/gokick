// Package worker dequeues persisted jobs and runs their handlers. One worker
// per goroutine; the pool's concurrency is configurable but bounded by SQLite
// writer serialization in practice (one writer at a time under WAL).
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	jobapp "gokick/app/application/job"
	"gokick/app/domain/job"
	"gokick/app/domain/shared"
)

const (
	defaultPollInterval = 1 * time.Second
	defaultLockFor      = 5 * time.Minute
	defaultBaseBackoff  = 5 * time.Second
)

type Worker struct {
	logger      *slog.Logger
	repo        job.Repository
	registry    *jobapp.HandlerRegistry
	tx          shared.Transactor
	dispatcher  shared.JobDispatcher
	concurrency int
}

func NewWorker(
	logger *slog.Logger,
	repo job.Repository,
	registry *jobapp.HandlerRegistry,
	tx shared.Transactor,
	dispatcher shared.JobDispatcher,
	concurrency int,
) *Worker {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Worker{
		logger:      logger,
		repo:        repo,
		registry:    registry,
		tx:          tx,
		dispatcher:  dispatcher,
		concurrency: concurrency,
	}
}

// Run starts the worker pool and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker: starting",
		"concurrency", w.concurrency,
		"kinds", w.registry.Kinds(),
	)

	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			w.loop(ctx, slot)
		}(i)
	}
	wg.Wait()
	w.logger.Info("worker: stopped")
}

func (w *Worker) loop(ctx context.Context, slot int) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		w.processOne(ctx, slot)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// processOne claims and runs at most one job. Errors are logged; returning
// silently keeps the worker loop healthy.
func (w *Worker) processOne(ctx context.Context, slot int) {
	j, err := w.repo.ClaimDue(ctx, defaultLockFor)
	if err != nil {
		w.logger.Error("worker: claim failed", "slot", slot, "error", err)
		return
	}
	if j == nil {
		return
	}

	log := w.logger.With("slot", slot, "job_id", j.ID, "kind", j.Kind, "attempt", j.Attempts)

	handler, ok := w.registry.Lookup(j.Kind)
	if !ok {
		// Unknown kind = permanent failure; no retry.
		log.Error("worker: unknown job kind, marking failed")
		_ = w.repo.MarkFailed(ctx, j.ID, fmt.Sprintf("unknown kind %q", j.Kind))
		return
	}

	start := time.Now()
	if err := w.runWithinTx(ctx, j, handler); err != nil {
		w.handleFailure(ctx, log, j, err, time.Since(start))
		return
	}
	log.Info("worker: job completed", "duration", time.Since(start))
}

// runWithinTx opens a transaction, injects the dispatcher, invokes the
// handler, and — if the handler returns no error — calls MarkComplete inside
// the same transaction before commit. Handler failure rolls back the entire
// transaction (including any DB writes the handler made), so the job stays
// claimable for retry without partial state lingering.
func (w *Worker) runWithinTx(ctx context.Context, j *job.Job, handler jobapp.HandlerFunc) (err error) {
	txCtx, err := w.tx.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = w.tx.Rollback(txCtx)
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()

	txCtx = shared.ContextWithJobDispatcher(txCtx, w.dispatcher)

	if handlerErr := handler(txCtx, j.Payload); handlerErr != nil {
		_ = w.tx.Rollback(txCtx)
		return handlerErr
	}

	if completeErr := w.repo.MarkComplete(txCtx, j.ID); completeErr != nil {
		_ = w.tx.Rollback(txCtx)
		return fmt.Errorf("mark complete: %w", completeErr)
	}

	if commitErr := w.tx.Commit(txCtx); commitErr != nil {
		return fmt.Errorf("commit: %w", commitErr)
	}
	return nil
}

func (w *Worker) handleFailure(
	ctx context.Context,
	log *slog.Logger,
	j *job.Job,
	jobErr error,
	duration time.Duration,
) {
	if j.Attempts >= j.MaxAttempts {
		log.Error("worker: job exhausted retries, marking failed",
			"duration", duration, "error", jobErr,
		)
		if err := w.repo.MarkFailed(ctx, j.ID, jobErr.Error()); err != nil {
			log.Error("worker: mark failed write errored", "error", err)
		}
		return
	}

	delay := backoff(j.Attempts)
	runAt := time.Now().Add(delay)
	log.Warn("worker: job failed, retry scheduled",
		"duration", duration, "retry_in", delay, "error", jobErr,
	)
	if err := w.repo.Reschedule(ctx, j.ID, runAt, jobErr.Error()); err != nil {
		log.Error("worker: reschedule write errored", "error", err)
	}
}

// backoff returns 2^(attempts-1) * baseBackoff, capped at 1h. attempts is
// already-incremented (claim bumps it before returning), so attempts=1 means
// "first run failed, wait base before next attempt".
func backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	exp := math.Pow(2, float64(attempts-1))
	d := time.Duration(exp) * defaultBaseBackoff
	const cap = time.Hour
	if d > cap || d < 0 {
		return cap
	}
	return d
}

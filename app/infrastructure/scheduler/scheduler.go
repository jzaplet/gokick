// Package scheduler runs periodic in-process jobs (cron-like). Each job runs
// in its own goroutine; cancelling the context passed to Run drains all jobs.
//
// Jobs use a run-once-then-tick semantic — Fn runs immediately on Run, then
// every Interval thereafter. This guarantees maintenance jobs (token cleanup,
// stat collectors) execute at least once per process lifetime, even on a
// process that restarts more often than the interval.
//
// Every replica of `serve` runs a Scheduler, but each job runs on one of them:
// before each run the Scheduler holds the job's lock (shared.Locker, key
// "job:<name>"), and it keeps the lock for as long as it runs — not just for the
// run, which would only stop two replicas from overlapping and still run the job
// once per replica per interval. The other replicas' ticks find the lock taken
// and skip. When the holder stops, it releases the lock; when it dies, the
// database drops it; either way the next replica to tick takes the job over. On
// SQLite, which one process serves, the Locker grants every lock.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"gokick/app/domain/shared"
)

// Scheduler-local structured-log keys (cross-cutting ones live in
// shared.LogKey*). sloglint's no-raw-keys forbids bare string keys.
const (
	logKeyName = "name"
	logKeyJobs = "jobs"
)

// releaseTimeout bounds the release of a job's lock when the Scheduler stops —
// the Scheduler's context is already cancelled by then, and a stuck database
// must not hold up the shutdown.
const releaseTimeout = 5 * time.Second

type JobFunc func(ctx context.Context) error

type Job struct {
	Name     string
	Interval time.Duration
	Fn       JobFunc
}

type Scheduler struct {
	logger *slog.Logger
	locker shared.Locker
	jobs   []Job
}

func NewScheduler(logger *slog.Logger, locker shared.Locker, jobs []Job) (*Scheduler, error) {
	if locker == nil {
		return nil, fmt.Errorf("scheduler: a locker is required")
	}
	seen := make(map[string]struct{}, len(jobs))
	for _, j := range jobs {
		if j.Name == "" {
			return nil, fmt.Errorf("scheduler: job name is required")
		}
		if j.Interval <= 0 {
			return nil, fmt.Errorf(
				"scheduler: job %q has non-positive interval %s",
				j.Name,
				j.Interval,
			)
		}
		if j.Fn == nil {
			return nil, fmt.Errorf("scheduler: job %q has nil Fn", j.Name)
		}
		if _, dup := seen[j.Name]; dup {
			return nil, fmt.Errorf("scheduler: duplicate job name %q", j.Name)
		}
		seen[j.Name] = struct{}{}
	}
	return &Scheduler{logger: logger, locker: locker, jobs: jobs}, nil
}

// Run launches every registered job in its own goroutine and blocks until
// ctx is cancelled and all jobs have drained.
func (s *Scheduler) Run(ctx context.Context) {
	if len(s.jobs) == 0 {
		s.logger.Info("scheduler: no jobs registered")
		return
	}
	s.logger.Info("scheduler: starting", logKeyJobs, len(s.jobs))

	var wg sync.WaitGroup
	for _, j := range s.jobs {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			s.runJob(ctx, j)
		}(j)
	}
	wg.Wait()
	s.logger.Info("scheduler: stopped")
}

func (s *Scheduler) runJob(ctx context.Context, j Job) {
	defer s.release(ctx, j)

	// Run-once-then-tick: maintenance jobs benefit from immediate execution
	// so a frequently-restarted process still cleans up at least once.
	s.tick(ctx, j)

	ticker := time.NewTicker(j.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx, j)
		}
	}
}

// lockKey is the Locker key of a job.
func lockKey(j Job) string { return "job:" + j.Name }

// release gives up the job's lock once its loop ends, so another replica can
// take the job over on its next tick.
func (s *Scheduler) release(ctx context.Context, j Job) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := s.locker.Release(ctx, lockKey(j)); err != nil {
		s.logger.Warn("scheduler: releasing the job lock failed",
			slog.String(logKeyName, j.Name),
			slog.Any(shared.LogKeyError, err),
		)
	}
}

// tick runs one invocation of j.Fn — if this replica holds the job's lock — with
// panic recovery, so a buggy job can't take down the rest of the scheduler.
func (s *Scheduler) tick(ctx context.Context, j Job) {
	defer func() {
		if r := recover(); r != nil {
			// Log-only on purpose — NOT reported to the error tracker. Unlike a
			// terminal job failure, a scheduler job re-ticks every interval
			// forever, so a deterministic panic would emit an unbounded stream
			// of identical events. The Error-level log is the operator signal, so
			// it must carry the stack trace — without it the log names the job but
			// not the faulting line.
			s.logger.LogAttrs(ctx, slog.LevelError, "scheduler: job panicked",
				slog.String(logKeyName, j.Name),
				slog.Any(shared.LogKeyPanic, r),
				slog.String(shared.LogKeyStack, string(debug.Stack())),
			)
		}
	}()

	held, err := s.locker.Hold(ctx, lockKey(j))
	if err != nil {
		s.logger.Error("scheduler: job lock failed",
			slog.String(logKeyName, j.Name),
			slog.Any(shared.LogKeyError, err),
		)
		return
	}
	if !held {
		s.logger.Debug("scheduler: job skipped, another replica runs it",
			slog.String(logKeyName, j.Name))
		return
	}

	start := time.Now()
	err = j.Fn(ctx)
	duration := time.Since(start)
	if err != nil {
		s.logger.Error(
			"scheduler: job failed",
			slog.String(logKeyName, j.Name),
			shared.DurationMsAttr(duration),
			slog.Any(shared.LogKeyError, err),
		)
		return
	}
	s.logger.Info(
		"scheduler: job completed",
		slog.String(logKeyName, j.Name),
		shared.DurationMsAttr(duration),
	)
}

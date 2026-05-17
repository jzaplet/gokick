// Package scheduler runs periodic in-process jobs (cron-like). Each job runs
// in its own goroutine; cancelling the context passed to Run drains all jobs.
//
// Jobs use a run-once-then-tick semantic — Fn runs immediately on Run, then
// every Interval thereafter. This guarantees maintenance jobs (token cleanup,
// stat collectors) execute at least once per process lifetime, even on a
// process that restarts more often than the interval.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type JobFunc func(ctx context.Context) error

type Job struct {
	Name     string
	Interval time.Duration
	Fn       JobFunc
}

type Scheduler struct {
	logger *slog.Logger
	jobs   []Job
}

func NewScheduler(logger *slog.Logger, jobs []Job) (*Scheduler, error) {
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
	return &Scheduler{logger: logger, jobs: jobs}, nil
}

// Run launches every registered job in its own goroutine and blocks until
// ctx is cancelled and all jobs have drained.
func (s *Scheduler) Run(ctx context.Context) {
	if len(s.jobs) == 0 {
		s.logger.Info("scheduler: no jobs registered")
		return
	}
	s.logger.Info("scheduler: starting", "jobs", len(s.jobs))

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

// tick runs one invocation of j.Fn with panic recovery, so a buggy job
// can't take down the rest of the scheduler.
func (s *Scheduler) tick(ctx context.Context, j Job) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("scheduler: job panicked", "name", j.Name, "panic", r)
		}
	}()

	start := time.Now()
	err := j.Fn(ctx)
	duration := time.Since(start)
	if err != nil {
		s.logger.Error("scheduler: job failed", "name", j.Name, "duration", duration, "error", err)
		return
	}
	s.logger.Info("scheduler: job completed", "name", j.Name, "duration", duration)
}

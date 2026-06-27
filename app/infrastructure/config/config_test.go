package config

import (
	"strings"
	"testing"
	"time"
)

// LoadConfig parses the APP_RUN_WORKER_* env vars onto the Config. No test exercised
// this glue, so its parse + wrapped-error branches were dead. These do NOT use
// t.Parallel — t.Setenv forbids it.

func TestLoadConfig_RunWorker_ParsesValues(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_LEASE", "42s")
	t.Setenv("APP_RUN_WORKER_HEARTBEAT", "7s")
	t.Setenv("APP_RUN_WORKER_POLL", "2s")
	t.Setenv("APP_RUN_WORKER_DRAIN_TIMEOUT", "5s")
	t.Setenv("APP_RUN_WORKER_MAX_IN_FLIGHT", "3")
	t.Setenv("APP_RUN_WORKER_MAX_RECLAIMS", "9")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RunWorkerLease != 42*time.Second {
		t.Fatalf("RunWorkerLease: got %s want 42s", cfg.RunWorkerLease)
	}
	if cfg.RunWorkerHeartbeat != 7*time.Second {
		t.Fatalf("RunWorkerHeartbeat: got %s want 7s", cfg.RunWorkerHeartbeat)
	}
	if cfg.RunWorkerPoll != 2*time.Second {
		t.Fatalf("RunWorkerPoll: got %s want 2s", cfg.RunWorkerPoll)
	}
	if cfg.RunWorkerDrainTimeout != 5*time.Second {
		t.Fatalf("RunWorkerDrainTimeout: got %s want 5s", cfg.RunWorkerDrainTimeout)
	}
	if cfg.RunWorkerMaxInFlight != 3 {
		t.Fatalf("RunWorkerMaxInFlight: got %d want 3", cfg.RunWorkerMaxInFlight)
	}
	if cfg.RunWorkerMaxReclaims != 9 {
		t.Fatalf("RunWorkerMaxReclaims: got %d want 9", cfg.RunWorkerMaxReclaims)
	}
}

func TestLoadConfig_RunWorker_InvalidDurationFails(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_LEASE", "banana")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid APP_RUN_WORKER_LEASE") {
		t.Fatalf("expected a wrapped invalid-duration error, got %v", err)
	}
}

func TestLoadConfig_RunWorker_InvalidIntFails(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_MAX_IN_FLIGHT", "foo")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid APP_RUN_WORKER_MAX_IN_FLIGHT") {
		t.Fatalf("expected a wrapped invalid-int error, got %v", err)
	}
}

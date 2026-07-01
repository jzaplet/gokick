# Durable-run E2E (local)

End-to-end checks for the things in-process tests **structurally cannot** prove —
they need a real OS-process boundary / lifecycle (kill -9, SIGTERM) + a persistent
SQLite file, not a simulated one. No Docker, no srv3.

All of it is driven by the `APP_RUN_DEBUG` affordance (off by default, never in
production): the `e2e:*` run kinds + the `/debug/runs` enqueue/observe/cancel
endpoints. See `app/application/run/e2edebug.go` and
`app/presentation/http/handler/debug_run.go`. `make e2e` runs all four; each also
has its own target. Needs `jq` (at-least-once also `sqlite3`).

| Test | `make` | Proves |
|------|--------|--------|
| **Crash recovery** ⭐ | `e2e-crash-recovery` | kill -9 mid-run → a cold process reclaims the orphaned run and **resumes from its last checkpoint** to completion (`reclaims≥1`, final `step`==total). |
| **At-least-once** | `e2e-at-least-once` | A fire-and-forget run's side-effect fires, then kill -9 **before** the separate complete-write. No checkpoint → the reclaim **re-runs the whole handler** → the effect fires **twice**. Handlers must be idempotent. |
| **SIGTERM drain** | `e2e-sigterm-drain` | A graceful stop (redeploy) **abandons** the in-flight run cleanly (`attempts==0`, not failed/rescheduled) → a fresh instance reclaims + resumes. Not lost, not duplicated. |
| **Terminal failure** | `e2e-terminal-failure` | A run that exhausts retries reaches `failed`, firing the worker's `ErrorReporter.Capture` (fingerprint `run:<kind>`). **Local half only** — see note. |

**Recovery latency ≈ lease.** An orphaned run is not reclaimable until the dead
owner's lease expires, so the harness runs with a short `APP_RUN_WORKER_LEASE` (prod
default 5m). A real operational property, not a test artifact.

**Terminal-failure is the local half of a Sentry check.** It proves the terminal
path *fires* in a real process; it does **not** verify the Sentry event or the
`run:<kind>` fingerprint delivery — that needs a real Sentry project on a live
deploy (the throwaway `gokick-e2e` env exists for exactly this). The fingerprint
*logic* is unit-tested: `cmd/sentry_test.go` `TestCapture_WorkerFingerprintByKind`.

## What is NOT here (covered in-process — no deploy adds value)

retry/backoff math · lease/heartbeat renew + lease-loss abandon · owner-token
fencing · checkpoint encode/decode · cancel signal · poison cap · per-attempt
timeout · backpressure · tenant propagation. Those live in the Go suite (testfx,
real SQLite + real worker goroutine).

## Remaining candidate (deferred)

- **Multi-process fencing** — two worker containers sharing one SQLite volume,
  asserting a run is processed exactly once. Low priority: single-node SQLite;
  split-deploy is a Postgres-era concern (see the roadmap).

# Durable-run E2E (local)

End-to-end checks for the things in-process tests **structurally cannot** prove —
they need a real OS-process boundary / lifecycle, not a simulated one.

All of this is driven by the `APP_RUN_DEBUG` affordance (off by default, never in
production): the `e2e:*` run kinds + the `/debug/runs` enqueue/observe/cancel
endpoints. See `app/application/run/e2edebug.go` and
`app/presentation/http/handler/debug_run.go`.

## `run_crash_recovery.sh` — the crown jewel

```
make e2e-crash-recovery
```

Enqueues a checkpointing run, `kill -9`s the `serve` process **mid-run**, starts a
fresh **cold** process on the **same SQLite file**, and asserts the orphaned run is
reclaimed and **resumed from its last checkpoint** to completion — not restarted
from zero (`reclaims ≥ 1`, final `state.step` == total). No Docker, no srv3.

**Recovery latency ≈ lease.** An orphaned run is not reclaimable until the dead
owner's lease expires, so the harness runs with a short `APP_RUN_WORKER_LEASE`
(the production default is 5m). This is a real operational property, not a test
artifact.

Requires `jq`.

## What is NOT here (covered in-process — no deploy adds value)

retry/backoff math · lease/heartbeat renew + lease-loss abandon · owner-token
fencing · checkpoint encode/decode · cancel signal · poison cap · per-attempt
timeout · backpressure · tenant propagation. Those live in the Go suite (testfx,
real SQLite + real worker goroutine).

## Candidate follow-ups (same harness)

- **at-least-once after crash** — a fire-and-forget handler completes, the process
  dies before the complete write → restart re-runs it (idempotency window).
- **SIGTERM drain** — an in-flight run during a graceful stop drains or is cleanly
  reclaimed; never lost, never double-completed.
- **Sentry `run:<kind>` fingerprint** — a terminal failure surfaces one issue per
  kind (observability; reuses the throwaway e2e Sentry project on a live deploy).

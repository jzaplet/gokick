#!/usr/bin/env bash
#
# Local E2E — durable-run CRASH RECOVERY (the one guarantee in-process tests can't prove).
#
# Enqueues a checkpointing run, `kill -9`s the serve process MID-RUN, starts a fresh
# COLD process on the SAME SQLite file, and asserts the orphaned run is reclaimed and
# resumed from its last checkpoint to completion — not restarted from zero.
#
# No Docker, no srv3: a real OS-process boundary + a persistent DB file is all the
# durable guarantee needs. Recovery latency ≈ lease (an orphaned run is not
# reclaimable until the dead owner's lease expires), so this runs with a short
# APP_RUN_WORKER_LEASE — the production default is 5m.
#
# Requires: jq. Run:  make e2e-crash-recovery   (or ./test/e2e/run_crash_recovery.sh)
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP="$ROOT/bin/app"
WORK="$(mktemp -d)"
DB="$WORK/app.db"
SECRET="e2e-crash-recovery-jwt-secret-0123456789"   # ≥ 32 chars
LEASE="3s"
P1=18081
P2=18082
PID1=""
PID2=""

cleanup() {
	[ -n "$PID1" ] && kill -9 "$PID1" 2>/dev/null
	[ -n "$PID2" ] && kill -9 "$PID2" 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT

command -v jq >/dev/null || { echo "need jq"; exit 1; }

echo "building bin/app ..."
(cd "$ROOT" && go build -o bin/app ./cmd) || { echo "build failed"; exit 1; }

start_serve() { # $1 = port -> echoes PID
	APP_HTTP_PORT="$1" APP_DB_PATH="$DB" APP_JWT_SECRET="$SECRET" \
		APP_RUN_DEBUG=true APP_RUN_WORKER_LEASE="$LEASE" APP_RUN_WORKER_POLL=300ms \
		APP_CORS_ORIGIN="http://localhost:$1" \
		"$APP" serve >"$WORK/serve-$1.log" 2>&1 &
	echo $!
}
wait_health() { for _ in $(seq 1 50); do curl -sf "http://localhost:$1/health" >/dev/null 2>&1 && return 0; sleep 0.2; done; return 1; }
state() { curl -sf "http://localhost:$1/debug/runs/$2"; }

PID1="$(start_serve "$P1")"
wait_health "$P1" || { echo "instance1 never became healthy"; cat "$WORK/serve-$P1.log"; exit 1; }

ID="$(curl -sf -XPOST "http://localhost:$P1/debug/runs/e2e:checkpoint" -d '{"steps":10,"sleep_ms":500}' | jq -r .id)"
echo "enqueued run $ID (10 checkpoints × 500ms) on :$P1"

STEP=0
for _ in $(seq 1 40); do
	STEP="$(state "$P1" "$ID" | jq -r '.state.step // 0')"
	[ "$STEP" -ge 2 ] && break
	sleep 0.2
done
echo "pre-crash: step=$STEP status=$(state "$P1" "$ID" | jq -r .status)"

kill -9 "$PID1" 2>/dev/null || true
echo ">>> kill -9 instance1 ($PID1) MID-RUN"
sleep 0.5

PID2="$(start_serve "$P2")"
wait_health "$P2" || { echo "instance2 never became healthy"; cat "$WORK/serve-$P2.log"; exit 1; }
echo "restarted as instance2 ($PID2) on :$P2 — cold process, same DB"

FINAL=""
for _ in $(seq 1 80); do
	FINAL="$(state "$P2" "$ID")"
	ST="$(echo "$FINAL" | jq -r .status)"
	{ [ "$ST" = "completed" ] || [ "$ST" = "failed" ]; } && break
	sleep 0.3
done
echo "final: $FINAL"

STATUS="$(echo "$FINAL" | jq -r .status)"
RECLAIMS="$(echo "$FINAL" | jq -r .reclaims)"
FSTEP="$(echo "$FINAL" | jq -r '.state.step')"

if [ "$STATUS" = "completed" ] && [ "$RECLAIMS" -ge 1 ] && [ "$FSTEP" = "10" ]; then
	echo "✅ PASS — completed after $RECLAIMS reclaim(s), final step $FSTEP: a cold process resumed from the checkpoint after a hard crash"
	exit 0
fi
echo "❌ FAIL — status=$STATUS reclaims=$RECLAIMS step=$FSTEP"
echo "--- instance2 log ---"; tail -25 "$WORK/serve-$P2.log"
exit 1

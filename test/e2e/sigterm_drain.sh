#!/usr/bin/env bash
#
# SIGTERM drain — a graceful stop (like a redeploy) never loses or duplicates work.
#
# On SIGTERM the worker stops claiming and drains; an in-flight run is ABANDONED for
# reclaim (finalize's workerCtx.Err() branch) — NOT marked failed, NOT rescheduled.
# A fresh instance reclaims it and RESUMES from the checkpoint to completion.
#
# Primary assertion: completed exactly once, reclaimed, resumed (final step == total).
# Discriminator: attempts == 0 — proves the graceful shutdown-abandon branch, not the
# failure/reschedule branch. That's why this earns its place next to the kill -9 test.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib.sh
source "$ROOT/test/e2e/lib.sh"
APP="$ROOT/bin/app"; WORK="$(mktemp -d)"; LEASE="3s"
SECRET="e2e-sigterm-drain-jwt-secret-0123456789012"
P1=18085; P2=18086; PID1=""; PID2=""
cleanup() { [ -n "$PID1" ] && kill -9 "$PID1" 2>/dev/null; [ -n "$PID2" ] && kill -9 "$PID2" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT

e2e_require; e2e_build

PID1="$(e2e_start $P1)"; e2e_wait_health $P1 || { echo "instance1 unhealthy"; cat "$WORK/serve-$P1.log"; exit 1; }
ID="$(e2e_enqueue $P1 e2e:checkpoint '{"steps":10,"sleep_ms":500}' | jq -r .id)"
echo "enqueued $ID (10 checkpoints × 500ms) on :$P1"

STEP=0
for _ in $(seq 1 40); do STEP="$(e2e_state $P1 "$ID" | jq -r '.state.step // 0')"; [ "$STEP" -ge 2 ] && break; sleep 0.2; done
echo "pre-shutdown: step=$STEP"

kill -TERM "$PID1"; echo ">>> SIGTERM instance1 MID-RUN (graceful stop, like a redeploy)"
for _ in $(seq 1 60); do kill -0 "$PID1" 2>/dev/null || break; sleep 0.2; done
if kill -0 "$PID1" 2>/dev/null; then echo "instance1 did not exit after SIGTERM (drain hung)"; kill -9 "$PID1"; exit 1; fi
echo "instance1 exited cleanly on SIGTERM"

PID2="$(e2e_start $P2)"; e2e_wait_health $P2 || { echo "instance2 unhealthy"; cat "$WORK/serve-$P2.log"; exit 1; }
echo "restarted as instance2 on :$P2 (same DB)"

FINAL=""
for _ in $(seq 1 80); do
	FINAL="$(e2e_state $P2 "$ID")"; ST="$(echo "$FINAL" | jq -r .status)"
	{ [ "$ST" = completed ] || [ "$ST" = failed ]; } && break; sleep 0.3
done
echo "final: $FINAL"

STATUS="$(echo "$FINAL" | jq -r .status)"; RECLAIMS="$(echo "$FINAL" | jq -r .reclaims)"; ATT="$(echo "$FINAL" | jq -r .attempts)"; FSTEP="$(echo "$FINAL" | jq -r '.state.step')"
if [ "$STATUS" = completed ] && [ "$RECLAIMS" -ge 1 ] && [ "$ATT" -eq 0 ] && [ "$FSTEP" = 10 ]; then
	echo "✅ PASS — SIGTERM abandoned the in-flight run cleanly (attempts=0, not failed/rescheduled); a fresh instance reclaimed ($RECLAIMS) and resumed to step $FSTEP. Not lost, not duplicated."
	exit 0
fi
echo "❌ FAIL — status=$STATUS reclaims=$RECLAIMS attempts=$ATT step=$FSTEP (want completed / ≥1 / 0 / 10)"
echo "--- instance2 log ---"; tail -25 "$WORK/serve-$P2.log"
exit 1

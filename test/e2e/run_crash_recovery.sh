#!/usr/bin/env bash
#
# Crash recovery — the crown jewel. Enqueue a checkpointing run, `kill -9` the serve
# process MID-RUN, restart cold on the SAME SQLite file, assert the orphaned run is
# reclaimed and RESUMED from its last checkpoint to completion (not restarted from
# zero). Recovery latency ≈ lease, so LEASE is short (prod default is 5m).
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib.sh
source "$ROOT/test/e2e/lib.sh"
APP="$ROOT/bin/app"; WORK="$(mktemp -d)"; LEASE="3s"
SECRET="e2e-crash-recovery-jwt-secret-0123456789"
P1=18081; P2=18082; PID1=""; PID2=""
cleanup() { [ -n "$PID1" ] && kill -9 "$PID1" 2>/dev/null; [ -n "$PID2" ] && kill -9 "$PID2" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT

e2e_require; e2e_build

PID1="$(e2e_start $P1)"; e2e_wait_health $P1 || { echo "instance1 unhealthy"; cat "$WORK/serve-$P1.log"; exit 1; }
ID="$(e2e_enqueue $P1 e2e:checkpoint '{"steps":10,"sleep_ms":500}' | jq -r .id)"
echo "enqueued $ID (10 checkpoints × 500ms) on :$P1"

STEP=0
for _ in $(seq 1 40); do STEP="$(e2e_state $P1 "$ID" | jq -r '.state.step // 0')"; [ "$STEP" -ge 2 ] && break; sleep 0.2; done
echo "pre-crash: step=$STEP"

kill -9 "$PID1" 2>/dev/null; echo ">>> kill -9 instance1 MID-RUN"; sleep 0.5
PID2="$(e2e_start $P2)"; e2e_wait_health $P2 || { echo "instance2 unhealthy"; cat "$WORK/serve-$P2.log"; exit 1; }
echo "restarted cold as instance2 on :$P2 (same DB)"

FINAL=""
for _ in $(seq 1 80); do
	FINAL="$(e2e_state $P2 "$ID")"; ST="$(echo "$FINAL" | jq -r .status)"
	{ [ "$ST" = completed ] || [ "$ST" = failed ]; } && break; sleep 0.3
done
echo "final: $FINAL"

STATUS="$(echo "$FINAL" | jq -r .status)"; RECLAIMS="$(echo "$FINAL" | jq -r .reclaims)"; FSTEP="$(echo "$FINAL" | jq -r '.state.step')"
if [ "$STATUS" = completed ] && [ "$RECLAIMS" -ge 1 ] && [ "$FSTEP" = 10 ]; then
	echo "✅ PASS — completed after $RECLAIMS reclaim(s), final step $FSTEP: cold process resumed from the checkpoint after kill -9"
	exit 0
fi
echo "❌ FAIL — status=$STATUS reclaims=$RECLAIMS step=$FSTEP"; echo "--- instance2 log ---"; tail -25 "$WORK/serve-$P2.log"
exit 1

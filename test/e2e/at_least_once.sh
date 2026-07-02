#!/usr/bin/env bash
#
# At-least-once — a fire-and-forget run's side-effect can fire MORE THAN ONCE.
#
# The e2e:effect handler enqueues a marker child run (its observable side-effect —
# stands in for a mail sent / API called), then sleeps. We kill -9 AFTER the effect
# but BEFORE the handler returns (before the SEPARATE MarkComplete write). The run is
# reclaimed and, having NO checkpoint, RE-RUNS the whole handler → the effect fires a
# SECOND time. This is why durable handlers must be idempotent. Marker rows are
# counted directly in the SQLite file.
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib.sh
source "$ROOT/test/e2e/lib.sh"
APP="$ROOT/bin/app"; WORK="$(mktemp -d)"; LEASE="3s"
SECRET="e2e-at-least-once-jwt-secret-012345678901"
P1=18083; P2=18084; PID1=""; PID2=""
cleanup() { [ -n "$PID1" ] && kill -9 "$PID1" 2>/dev/null; [ -n "$PID2" ] && kill -9 "$PID2" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT
command -v sqlite3 >/dev/null || { echo "FATAL: need sqlite3"; exit 1; }
markers() { sqlite3 "$WORK/app.db" "SELECT count(*) FROM runs WHERE kind='e2e:succeed';" 2>/dev/null || echo 0; }

e2e_require; e2e_build

PID1="$(e2e_start $P1)"; e2e_wait_health $P1 || { echo "instance1 unhealthy"; cat "$WORK/serve-$P1.log"; exit 1; }
ID="$(e2e_enqueue $P1 e2e:effect '{"sleep_ms":4000}' | jq -r .id)"
echo "enqueued $ID (fire-and-forget effect, 4s window) on :$P1"

# Wait until the effect fired once (marker enqueued) — that opens the crash window.
for _ in $(seq 1 40); do [ "$(markers)" -ge 1 ] && break; sleep 0.2; done
echo "pre-crash: markers=$(markers) status=$(e2e_state $P1 "$ID" | jq -r .status)"

kill -9 "$PID1" 2>/dev/null; echo ">>> kill -9 instance1 AFTER the effect, BEFORE complete"; sleep 0.5
PID2="$(e2e_start $P2)"; e2e_wait_health $P2 || { echo "instance2 unhealthy"; cat "$WORK/serve-$P2.log"; exit 1; }
echo "restarted cold as instance2 on :$P2 (same DB)"

FINAL=""
for _ in $(seq 1 80); do
	FINAL="$(e2e_state $P2 "$ID")"; ST="$(echo "$FINAL" | jq -r .status)"
	{ [ "$ST" = completed ] || [ "$ST" = failed ]; } && break; sleep 0.3
done
echo "final: $FINAL"

M="$(markers)"
STATUS="$(echo "$FINAL" | jq -r .status)"; RECLAIMS="$(echo "$FINAL" | jq -r .reclaims)"; ATT="$(echo "$FINAL" | jq -r .attempts)"
echo "markers (effect executions) = $M"
if [ "$STATUS" = completed ] && [ "$M" -eq 2 ] && [ "$RECLAIMS" -ge 1 ] && [ "$ATT" -eq 0 ]; then
	echo "✅ PASS — effect fired ${M}× (at-least-once) across a crash: reclaimed ($RECLAIMS), re-ran the whole handler, completed once. Handlers MUST be idempotent."
	exit 0
fi
echo "❌ FAIL — status=$STATUS markers=$M reclaims=$RECLAIMS attempts=$ATT (want completed / 2 / ≥1 / 0)"
echo "--- instance2 log ---"; tail -25 "$WORK/serve-$P2.log"
exit 1

#!/usr/bin/env bash
#
# Terminal failure → Sentry trigger — LOCAL HALF ONLY (read the NOTE below).
#
# A run that exhausts its retries fails terminally, which is exactly what fires the
# worker's ErrorReporter.Capture (fingerprint ["{{ default }}", "run:<kind>"]). This
# asserts the terminal-failure path FIRES in a real process.
#
# It does NOT verify the Sentry event or the run:<kind> fingerprint DELIVERY — that
# needs a real Sentry project on a live deploy (the throwaway gokick-e2e env exists
# for exactly this). The fingerprint LOGIC is unit-tested: cmd/sentry_test.go
# TestCapture_WorkerFingerprintByKind. Do not read a green run here as "Sentry E2E".
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib.sh
source "$ROOT/test/e2e/lib.sh"
APP="$ROOT/bin/app"; WORK="$(mktemp -d)"; LEASE="3s"
SECRET="e2e-terminal-failure-jwt-secret-01234567"
P1=18087; PID1=""
cleanup() { [ -n "$PID1" ] && kill -9 "$PID1" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT

e2e_require; e2e_build

PID1="$(e2e_start $P1)"; e2e_wait_health $P1 || { echo "instance1 unhealthy"; cat "$WORK/serve-$P1.log"; exit 1; }
ID="$(e2e_enqueue $P1 e2e:fail '{}' 0 | jq -r .id)"
echo "enqueued $ID (always-erroring handler, max_retries=0 → fails on first attempt) on :$P1"

FINAL=""
for _ in $(seq 1 60); do
	FINAL="$(e2e_state $P1 "$ID")"; ST="$(echo "$FINAL" | jq -r .status)"
	[ "$ST" = failed ] && break; sleep 0.3
done
echo "final: $FINAL"

STATUS="$(echo "$FINAL" | jq -r .status)"
# The worker logs a terminal-failure capture line; surface it as corroboration.
grep -iE "exhausted retries|terminal|capture" "$WORK/serve-$P1.log" | tail -3 || true
if [ "$STATUS" = failed ]; then
	echo "✅ PASS (terminal path) — run reached status=failed; this fires ErrorReporter.Capture with fingerprint run:e2e:fail."
	echo "   NOTE: Sentry event + fingerprint DELIVERY is a LIVE-DEPLOY check (real Sentry project), not this script."
	echo "   Fingerprint logic is unit-tested: cmd/sentry_test.go TestCapture_WorkerFingerprintByKind."
	exit 0
fi
echo "❌ FAIL — status=$STATUS (want failed)"; echo "--- log ---"; tail -25 "$WORK/serve-$P1.log"
exit 1

#!/usr/bin/env bash
#
# Shared helpers for the local durable-run E2E scripts. `source` this — do not run it.
# The sourcing script sets ROOT / APP / SECRET / WORK / LEASE, then calls these.
#
# These E2E scripts cover only what an in-process test STRUCTURALLY can't: a real OS
# process boundary (kill -9 / SIGTERM) + a persistent SQLite file. Pure logic
# (retry/backoff, lease/heartbeat, fencing, checkpoint encode, cancel, poison cap,
# timeout, backpressure) stays in the Go suite. No Docker, no srv3.

e2e_require() { command -v jq >/dev/null || { echo "FATAL: need jq"; exit 1; }; }

e2e_build() {
	echo "building bin/app ..."
	(cd "$ROOT" && go build -o bin/app ./cmd) || { echo "FATAL: build failed"; exit 1; }
}

# e2e_start <port> — starts a serve process on the shared DB; echoes its PID.
e2e_start() {
	APP_HTTP_PORT="$1" APP_DB_PATH="$WORK/app.db" APP_JWT_SECRET="$SECRET" \
		APP_RUN_DEBUG=true APP_RUN_WORKER_LEASE="$LEASE" APP_RUN_WORKER_POLL=300ms \
		APP_CORS_ORIGIN="http://localhost:$1" \
		"$APP" serve >"$WORK/serve-$1.log" 2>&1 &
	echo $!
}

# e2e_wait_health <port> — returns 0 once /health answers, 1 on timeout.
e2e_wait_health() {
	for _ in $(seq 1 50); do
		curl -sf "http://localhost:$1/health" >/dev/null 2>&1 && return 0
		sleep 0.2
	done
	return 1
}

# e2e_enqueue <port> <kind> <body> [max_retries] — echoes the JSON {id,kind}.
e2e_enqueue() {
	local q=""
	[ -n "${4:-}" ] && q="?max_retries=$4"
	curl -sf -XPOST "http://localhost:$1/debug/runs/$2$q" -d "$3"
}

# e2e_state <port> <id> — echoes the run's JSON view.
e2e_state() { curl -sf "http://localhost:$1/debug/runs/$2"; }

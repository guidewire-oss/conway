#!/usr/bin/env sh
# Waits until an HTTP listener actually answers, so a dev-loop "started" message
# means "serving" rather than "a process exists".
#
# Why this is not a `kill -0` check: the server dials Postgres before it binds the
# port, so an unreachable DATABASE_URL leaves a live pid that never listens. The
# `make server` recipe printed its success line and the URL regardless, which is
# one of the two ways an older binary ends up serving a newer page. The symptom was
# a 405 the UI rendered as a control labelled "Method".
#
# usage: wait-for-http.sh <url> <pid> <logfile> [timeout-seconds]
set -u
url=${1:?url}
pid=${2:?pid}
log=${3:?logfile}
timeout=${4:-30}

fail() {
	echo "$1"
	if [ -s "$log" ]; then
		echo "Last lines of $log:"
		tail -15 "$log"
	else
		echo "($log is empty - the server logged nothing before hanging.)"
	fi
	exit 1
}

# A wall-clock deadline, not an attempt count: a listener that accepts TCP but
# never answers makes curl burn --max-time each probe, plus the interprobe sleep,
# so "30 iterations" once ran about 90s while reporting a 30s timeout.
now() { date +%s; }
start=$(now)
deadline=$((start + timeout))
while :; do
	now=$(now)
	remaining=$((deadline - now))
	[ "$remaining" -gt 0 ] || break
	kill -0 "$pid" 2>/dev/null || fail "server exited after $((now - start))s (pid $pid is gone)."
	# Curl's exit status, not its %{http_code} output: without -f, any HTTP response
	# exits 0, so this accepts 401 and 404 (we are testing the listener, not a
	# route) and rejects a refused connection. The first version of this compared
	# the printed code against "000" with an `|| echo 000` fallback, which on
	# failure concatenated to "000000" and passed the check - it reported a
	# never-listening server as started, the exact bug this script exists to catch.
	# --max-time is clamped to the remaining budget so the deadline is real.
	probe="$remaining"; [ "$probe" -gt 2 ] && probe=2
	if curl -s -o /dev/null --max-time "$probe" "$url" 2>/dev/null; then
		exit 0
	fi
	# Same clamp on the pause, so a 1s budget cannot become a 3s overshoot.
	[ "$remaining" -ge 1 ] && sleep 1
done

fail "server never answered on $url within ${timeout}s, though pid $pid is alive.
The usual cause is an unreachable DATABASE_URL: startup blocks on the database
before it binds the port. Check Postgres is up: docker compose up -d postgres"

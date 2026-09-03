#!/usr/bin/env bash
# Start the relay and Vite admin UI in one terminal, with colored log prefixes.
# Bash 3.2 compatible (macOS /bin/bash).
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CYAN=$'\033[1;36m'
MAGENTA=$'\033[1;35m'
RESET=$'\033[0m'

prefix() {
	local color="$1"
	local label="$2"
	while IFS= read -r line || [ -n "$line" ]; do
		printf '%s[%s]%s %s\n' "$color" "$label" "$RESET" "$line"
	done
}

kill_tree() {
	local pid="$1"
	local child
	[ -n "$pid" ] || return 0
	for child in $(pgrep -P "$pid" 2>/dev/null || true); do
		kill_tree "$child"
	done
	kill "$pid" 2>/dev/null || true
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/congee-dev.XXXXXX")"
mkfifo "$TMP/done"

relay_pid=""
admin_pid=""
cleaning=0

cleanup() {
	if [ "$cleaning" -eq 1 ]; then
		return
	fi
	cleaning=1
	trap - EXIT INT TERM
	kill_tree "$relay_pid"
	kill_tree "$admin_pid"
	rm -rf "$TMP"
}

trap cleanup EXIT
trap 'cleanup; exit 130' INT TERM

if [ ! -d web/admin/node_modules ]; then
	printf '%s[admin]%s installing dependencies (npm ci)\n' "$MAGENTA" "$RESET"
	(cd web/admin && npm ci)
fi

(
	set +e
	set -o pipefail
	go run ./cmd/congee 2>&1 | prefix "$CYAN" relay
	echo "relay:$?" >"$TMP/done"
) &
relay_pid=$!

(
	set +e
	set -o pipefail
	(cd web/admin && FORCE_COLOR=1 npm run dev) 2>&1 | prefix "$MAGENTA" admin
	echo "admin:$?" >"$TMP/done"
) &
admin_pid=$!

IFS=: read -r _who code <"$TMP/done"
exit "${code:-0}"

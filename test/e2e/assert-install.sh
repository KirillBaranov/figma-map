#!/bin/sh
# Runs *inside* a Docker container against fixtures built by
# build-fixtures.sh and served over HTTP on the host (see
# test/e2e/run-e2e-local.sh / the e2e-install CI job). POSIX sh only — no
# bashisms, this must run under Alpine's ash/busybox too.
#
# Usage: sh test/e2e/assert-install.sh <label> [mode]
#   label  a name for this run, printed in output (e.g. the image name)
#   mode   "full" (default) or "partial" — partial soft-fails the
#          backend-execution checks (see FROM_BUNDLE below) instead of
#          hard-failing, for musl images where bun's glibc-targeted
#          compile output may not run at all.
#
# Requires on PATH: curl, tar, jq, sha256sum (or shasum), and either curl
# or wget (install.sh itself only needs one of curl/wget; this script
# additionally needs curl for its own /ping and --json assertions).

set -eu

LABEL="${1:?usage: assert-install.sh <label> [mode]}"
MODE="${2:-full}"

FIXTURE_HOST="${FIGMA_MAP_E2E_FIXTURE_HOST:-http://localhost:8080}"
INSTALL_DIR="${FIGMA_MAP_INSTALL_DIR:-/usr/local/bin}"
export FIGMA_MAP_INSTALL_DIR="$INSTALL_DIR"

PASS_COUNT=0

pass() { PASS_COUNT=$((PASS_COUNT + 1)); printf '[%s] PASS %s\n' "$LABEL" "$1"; }
fail() { printf '[%s] FAIL %s: %s\n' "$LABEL" "$1" "$2" >&2; exit 1; }
skip() { printf '[%s] SKIP %s: %s\n' "$LABEL" "$1" "$2"; }

for tool in curl tar jq; do
	command -v "$tool" >/dev/null 2>&1 || fail "prereqs" "$tool not found on PATH"
done

# ---------------------------------------------------------------------------
# 1. Install
# ---------------------------------------------------------------------------
export FIGMA_MAP_VERSION="v0.99.0"
export FIGMA_MAP_BASE_URL="${FIXTURE_HOST}/v0.99.0"
sh ./install.sh || fail "install" "install.sh exited non-zero"
export PATH="${INSTALL_DIR}:$PATH"
CLI_PATH="${INSTALL_DIR}/figma-map"
[ -x "$CLI_PATH" ] || fail "install" "figma-map not found/executable at ${CLI_PATH}"
figma-map --version >/dev/null 2>&1 || fail "install" "figma-map --version failed"
pass "install"

# ---------------------------------------------------------------------------
# 2. Paths — the exact on-disk contract install.sh and the Go fetch code
#    (internal/service/bridgeproc.go, plugin.go) must agree on.
# ---------------------------------------------------------------------------
BACKEND_BIN="$HOME/.figma-map/versions/v0.99.0/backend/figma-bridge"
PLUGIN_MANIFEST="$HOME/.figma-map/plugin/manifest.json"
PLUGIN_VERSION_FILE="$HOME/.figma-map/plugin/.version"

[ -x "$BACKEND_BIN" ] || fail "paths" "backend binary missing/not executable at ${BACKEND_BIN}"
[ -f "$PLUGIN_MANIFEST" ] || fail "paths" "plugin manifest missing at ${PLUGIN_MANIFEST}"
[ -f "$PLUGIN_VERSION_FILE" ] || fail "paths" "plugin version file missing at ${PLUGIN_VERSION_FILE}"
GOT_VERSION="$(cat "$PLUGIN_VERSION_FILE")"
[ "$GOT_VERSION" = "v0.99.0" ] || fail "paths" "plugin .version = '${GOT_VERSION}', want v0.99.0"
pass "paths"

# ---------------------------------------------------------------------------
# 3. bridge up + /ping — proves the fetched backend binary actually
#    executes in this container's environment, not just that it's present
#    on disk (this is where a glibc/musl mismatch would surface).
# ---------------------------------------------------------------------------
backend_started=0
UP_JSON="$(figma-map bridge up --json 2>/tmp/bridge-up.stderr)" && backend_started=1 || true
if [ "$backend_started" = "1" ]; then
	STARTED="$(echo "$UP_JSON" | jq -r '.started')"
	PID="$(echo "$UP_JSON" | jq -r '.pid // 0')"
	if [ "$STARTED" != "true" ] || [ "$PID" = "0" ]; then
		backend_started=0
	fi
fi
if [ "$backend_started" = "1" ] && curl -sf http://localhost:1994/ping >/dev/null 2>&1; then
	pass "bridge-ping"
else
	DIAG="bridge up output: $(cat /tmp/bridge-up.stderr 2>/dev/null || true) $(cat "$HOME/.figma-map/bridge.log" 2>/dev/null || true)"
	if [ "$MODE" = "partial" ]; then
		skip "bridge-ping" "backend did not start (suspected musl/glibc mismatch) — ${DIAG}"
	else
		fail "bridge-ping" "$DIAG"
	fi
fi

# ---------------------------------------------------------------------------
# 4. doctor — bridge-reachable check specifically, not overall pass (plugin
#    connectivity legitimately fails with no real Figma attached here).
# ---------------------------------------------------------------------------
if [ "$backend_started" = "1" ]; then
	DOCTOR_JSON="$(figma-map doctor --json || true)"
	BRIDGE_OK="$(echo "$DOCTOR_JSON" | jq -r '.checks[] | select(.name == "figma bridge (http://localhost:1994)") | .ok')"
	[ "$BRIDGE_OK" = "true" ] || fail "doctor-bridge" "bridge check not ok in: ${DOCTOR_JSON}"
	pass "doctor-bridge"
else
	skip "doctor-bridge" "backend not running (see bridge-ping)"
fi

# ---------------------------------------------------------------------------
# 5. update — figma-map update against a second, bumped-version fixture.
#    Bridge is deliberately left running (not stopped first): cmd/update.go's
#    refreshBackend checks BridgeStatus itself and only restarts if it was
#    already running, so this exercises that path.
# ---------------------------------------------------------------------------
export FIGMA_MAP_VERSION="v0.99.1"
export FIGMA_MAP_BASE_URL="${FIXTURE_HOST}/v0.99.1"
figma-map update --version v0.99.1 || fail "update" "figma-map update exited non-zero"

GOT_CLI_VERSION="$(figma-map --version)"
case "$GOT_CLI_VERSION" in
	*0.99.1*) : ;;
	*) fail "update" "figma-map --version = '${GOT_CLI_VERSION}', expected it to mention 0.99.1" ;;
esac
[ -x "$HOME/.figma-map/versions/v0.99.1/backend/figma-bridge" ] || fail "update" "backend bundle for v0.99.1 not found"
GOT_PLUGIN_VERSION="$(cat "$PLUGIN_VERSION_FILE")"
[ "$GOT_PLUGIN_VERSION" = "v0.99.1" ] || fail "update" "plugin .version = '${GOT_PLUGIN_VERSION}', want v0.99.1"

if [ "$backend_started" = "1" ]; then
	if curl -sf http://localhost:1994/ping >/dev/null 2>&1; then
		pass "update"
	elif [ "$MODE" = "partial" ]; then
		skip "update" "backend not answering after update (musl/glibc, consistent with bridge-ping)"
	else
		fail "update" "backend stopped answering /ping after update"
	fi
else
	pass "update"
fi

# ---------------------------------------------------------------------------
# 6. uninstall — clean slate: CLI gone, ~/.figma-map gone, bridge stopped.
# ---------------------------------------------------------------------------
figma-map uninstall --yes || fail "uninstall" "figma-map uninstall exited non-zero"
[ -e "$CLI_PATH" ] && fail "uninstall" "CLI binary still present at ${CLI_PATH}"
[ -e "$HOME/.figma-map" ] && fail "uninstall" "$HOME/.figma-map still present"
if [ "$backend_started" = "1" ] && curl -sf http://localhost:1994/ping >/dev/null 2>&1; then
	fail "uninstall" "bridge still answering /ping after uninstall"
fi
pass "uninstall"

printf '[%s] %s checks passed\n' "$LABEL" "$PASS_COUNT"

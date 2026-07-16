#!/usr/bin/env sh
# Runs the install e2e test (test/e2e/assert-install.sh) against fixtures
# built by build-fixtures.sh, across a small matrix of Linux Docker images.
# Requires fixtures already built at test/e2e/fixtures/v0.99.0 and
# .../v0.99.1 (see `make e2e-install`, which builds both before calling
# this) and a working `docker` on PATH.
#
# The fixture server runs as a container (not a bare host process) rather
# than assuming Docker containers can reach a process listening on the
# host's own localhost — that's true on native Linux (including GitHub
# Actions runners), but not on Docker-via-VM setups (e.g. Colima on macOS),
# where containers share the VM's network namespace, not the host OS's.
# Running the server as a container on the same --network host sidesteps
# that difference instead of special-casing it.
#
# If the repo lives under a directory macOS's privacy protections restrict
# (Desktop/Documents/Downloads) and your Docker setup can't mount it,
# either grant it access (Docker Desktop's File Sharing settings, or
# Colima's `mounts:` config) or run this from a checkout elsewhere.

set -eu

# shellcheck disable=SC1007 # CDPATH= (empty) intentionally prevents cd from printing/redirecting
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
FIXTURES="${ROOT}/test/e2e/fixtures"

[ -d "${FIXTURES}/v0.99.0" ] && [ -d "${FIXTURES}/v0.99.1" ] || {
	echo "fixtures missing — run: sh test/e2e/build-fixtures.sh 0.99.0 && sh test/e2e/build-fixtures.sh 0.99.1" >&2
	exit 1
}

FIXTURE_CONTAINER="figma-map-e2e-fixtures-$$"
cleanup() { docker rm -f "$FIXTURE_CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM

echo "› starting fixture server"
docker run -d --network host --name "$FIXTURE_CONTAINER" \
	-v "${FIXTURES}:/fixtures:ro" \
	python:3-slim python3 -m http.server 8080 --directory /fixtures >/dev/null

# shellcheck disable=SC2034 # loop counter, only used to bound the retry count
for i in $(seq 1 30); do
	docker run --rm --network host curlimages/curl -sf http://localhost:8080/ >/dev/null 2>&1 && break
	sleep 0.5
done

FAILED=0

run_image() {
	image="$1"
	prep="$2"
	mode="$3"
	echo "› running e2e in ${image} (mode: ${mode})"
	if ! docker run --rm --network host -v "${ROOT}:/repo" -w /repo "$image" sh -c "
		${prep}
		sh test/e2e/assert-install.sh '${image}' '${mode}'
	"; then
		FAILED=1
	fi
}

run_image "ubuntu:24.04" "apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates jq unzip >/dev/null 2>&1" "full"
run_image "debian:12" "apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates jq unzip >/dev/null 2>&1" "full"
run_image "alpine:3.20" "apk add --no-cache curl ca-certificates jq >/dev/null 2>&1" "partial"

if [ "$FAILED" != "0" ]; then
	echo "› e2e install test: FAILED" >&2
	exit 1
fi
echo "› e2e install test: all images passed"

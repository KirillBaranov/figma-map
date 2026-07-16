#!/usr/bin/env sh
# Builds a linux "fake release" (amd64 by default, see FIGMA_MAP_E2E_ARCH
# below) for the install e2e test: the CLI,
# a standalone backend bundle, and the Figma plugin, packaged with the
# exact same naming/checksum conventions .goreleaser.yaml/release.yml use
# for a real release, so install.sh/install.ps1 and the Go fetch code
# (internal/release) exercise their real logic against these fixtures.
#
# Usage: sh test/e2e/build-fixtures.sh <version>   (unprefixed, e.g. 0.99.0)
#
# Output: test/e2e/fixtures/v<version>/ — tag-prefixed directory, since
# FIGMA_MAP_BASE_URL embeds the tag verbatim (mirroring GitHub's own
# releases/download/<tag>/<asset> shape). Run this twice with two
# different versions before running the e2e assertions, to exercise both
# the initial install and `figma-map update`.

set -eu

VERSION="${1:?usage: build-fixtures.sh <version>}"
TAG="v${VERSION}"

# linux/amd64 by default, matching GitHub Actions' ubuntu-latest runners
# (where the real e2e-install CI job runs) — override to "arm64" only for
# local verification on an arm64 dev machine without x86 emulation set up.
ARCH="${FIGMA_MAP_E2E_ARCH:-amd64}"
case "$ARCH" in
	amd64) BUN_TARGET="bun-linux-x64" ;;
	arm64) BUN_TARGET="bun-linux-arm64" ;;
	*) echo "unsupported FIGMA_MAP_E2E_ARCH: $ARCH" >&2; exit 1 ;;
esac

# shellcheck disable=SC1007 # CDPATH= (empty) intentionally prevents cd from printing/redirecting
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
OUT="${ROOT}/test/e2e/fixtures/${TAG}"
mkdir -p "$OUT"

info() { printf '›  %s\n' "$1"; }

# Portable across CI (sha256sum, GNU coreutils) and local macOS dev
# (shasum) — same fallback install.sh's own sha256_of() uses.
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

# ---------------------------------------------------------------------------
# CLI: go build for linux/amd64, packaged like goreleaser's
# figma-map_<version>_<os>_<arch>.tar.gz
# ---------------------------------------------------------------------------
info "building CLI (linux/${ARCH})"
CLI_TMP="$(mktemp -d)"
trap 'rm -rf "$CLI_TMP"' EXIT

COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
(
	cd "$ROOT"
	# CGO_ENABLED=0 matches .goreleaser.yaml's real build exactly — without
	# it, a native (non-cross-compiling) `go build` on a Linux CI runner
	# with gcc present will link against the host's glibc, and the
	# resulting binary won't run at all on a musl (Alpine) image. This
	# doesn't show up when cross-compiling from macOS (Go auto-disables
	# cgo when there's no cross C toolchain), which is exactly why this
	# was missed locally and only surfaced on a real Linux CI runner.
	CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build \
		-ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
		-o "${CLI_TMP}/figma-map" .
)
CLI_ARCHIVE="figma-map_${VERSION}_linux_${ARCH}.tar.gz"
tar -C "$CLI_TMP" -czf "${OUT}/${CLI_ARCHIVE}" figma-map
echo "$(sha256_of "${OUT}/${CLI_ARCHIVE}")  ${CLI_ARCHIVE}" > "${OUT}/checksums.txt"

# ---------------------------------------------------------------------------
# Backend: bun-compiled standalone binary, packaged like release.yml's
# figma-map-backend_<version>_<os>_<arch>.tar.gz + its .sha256 sidecar
# ---------------------------------------------------------------------------
info "building backend (linux/${ARCH}, bun compile)"
BACKEND_TMP="$(mktemp -d)"
trap 'rm -rf "$CLI_TMP" "$BACKEND_TMP"' EXIT
(
	cd "$ROOT/backend"
	bun install --frozen-lockfile
	bun build --compile --target="$BUN_TARGET" --outfile "${BACKEND_TMP}/figma-bridge" src/index.ts
)
chmod +x "${BACKEND_TMP}/figma-bridge"
BACKEND_ARCHIVE="figma-map-backend_${VERSION}_linux_${ARCH}.tar.gz"
tar -C "$BACKEND_TMP" -czf "${OUT}/${BACKEND_ARCHIVE}" figma-bridge
sha256_of "${OUT}/${BACKEND_ARCHIVE}" > "${OUT}/${BACKEND_ARCHIVE}.sha256"

# ---------------------------------------------------------------------------
# Plugin: npm build, packaged like release.yml's "package figma plugin"
# step (figma-map-plugin/manifest.json + dist/, zipped) + its .sha256
# sidecar. Version-agnostic — rebuilt fresh each invocation (cheap), same
# zip content regardless of which fixture version it ships alongside.
# ---------------------------------------------------------------------------
info "building Figma plugin"
PLUGIN_TMP="$(mktemp -d)"
trap 'rm -rf "$CLI_TMP" "$BACKEND_TMP" "$PLUGIN_TMP"' EXIT
(
	cd "$ROOT/extensions/plugin"
	npm ci
	npm run build
)
mkdir -p "${PLUGIN_TMP}/figma-map-plugin"
cp "$ROOT/extensions/plugin/manifest.json" "${PLUGIN_TMP}/figma-map-plugin/"
cp -r "$ROOT/extensions/plugin/dist" "${PLUGIN_TMP}/figma-map-plugin/dist"
PLUGIN_ARCHIVE="figma-map-plugin.zip"
( cd "$PLUGIN_TMP" && zip -rq "${OUT}/${PLUGIN_ARCHIVE}" figma-map-plugin )
sha256_of "${OUT}/${PLUGIN_ARCHIVE}" > "${OUT}/${PLUGIN_ARCHIVE}.sha256"

info "fixture ${TAG} ready at ${OUT}"
ls -la "$OUT"

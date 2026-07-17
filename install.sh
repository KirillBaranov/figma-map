#!/usr/bin/env sh
# figma-map installer.
#
#   curl -fsSL https://raw.githubusercontent.com/kirillbaranov/figma-map/main/install.sh | sh
#
# Fetches three things from the matching GitHub release: the figma-map CLI
# binary, a standalone backend bundle (no Node install needed to run it),
# and the Figma plugin (no build step needed to load it). Run this
# yourself, in your own terminal — a coding agent should tell you to run
# it, not run it for you, since piping a remote script into a shell is
# something safety-conscious agents rightly refuse to do on their own.
#
# Environment overrides:
#   FIGMA_MAP_VERSION      install a specific tag (e.g. v0.10.0); default: latest
#   FIGMA_MAP_INSTALL_DIR  CLI install directory; default: /usr/local/bin or ~/.local/bin
#   FIGMA_MAP_BASE_URL     release-download base URL, for e2e testing against
#                          local fixtures instead of real GitHub (see test/e2e/)
#   NO_COLOR               disable colored output

set -eu

REPO="kirillbaranov/figma-map"
BINARY="figma-map"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
	BOLD="$(printf '\033[1m')"
	DIM="$(printf '\033[2m')"
	RED="$(printf '\033[31m')"
	GREEN="$(printf '\033[32m')"
	YELLOW="$(printf '\033[33m')"
	CYAN="$(printf '\033[36m')"
	RESET="$(printf '\033[0m')"
else
	BOLD="" DIM="" RED="" GREEN="" YELLOW="" CYAN="" RESET=""
fi

info()  { printf '%s  %s%s\n' "${CYAN}›${RESET}" "$1" "${2:-}"; }
ok()    { printf '%s  %s\n' "${GREEN}✓${RESET}" "$1"; }
warn()  { printf '%s  %s\n' "${YELLOW}!${RESET}" "$1"; }
die()   { printf '%s  %s\n' "${RED}✗${RESET}" "$1" >&2; exit 1; }

banner() {
	printf '%s' "$CYAN"
	cat <<'ART'
   __ _
  / _(_) __ _ _ __ ___   __ _   _ __ ___   __ _ _ __
 | |_| |/ _` | '_ ` _ \ / _` | | '_ ` _ \ / _` | '_ \
 |  _| | (_| | | | | | | (_| | | | | | | | (_| | |_) |
 |_| |_|\__, |_| |_| |_|\__,_| |_| |_| |_|\__,_| .__/
        |___/                                   |_|
ART
	printf '%s' "$RESET"
	printf '  %sMap Figma components to code with a vision LLM%s\n\n' "$DIM" "$RESET"
}

# ---------------------------------------------------------------------------
# Detect platform
# ---------------------------------------------------------------------------
detect_os() {
	os="$(uname -s)"
	case "$os" in
		Linux)  echo "linux" ;;
		Darwin) echo "darwin" ;;
		*) die "unsupported OS: $os (on Windows, use install.ps1 instead: irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex)" ;;
	esac
}

detect_arch() {
	arch="$(uname -m)"
	case "$arch" in
		x86_64 | amd64)  echo "amd64" ;;
		arm64 | aarch64) echo "arm64" ;;
		*) die "unsupported architecture: $arch" ;;
	esac
}

# ---------------------------------------------------------------------------
# HTTP helpers (curl or wget)
# ---------------------------------------------------------------------------
if command -v curl >/dev/null 2>&1; then
	fetch()    { curl -fsSL "$1"; }
	download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch()    { wget -qO- "$1"; }
	download() { wget -qO "$2" "$1"; }
else
	die "need curl or wget installed"
fi

# copy_to_clipboard copies $1 to the system clipboard, trying whichever tool
# is available (macOS pbcopy, X11 xclip/xsel, Wayland wl-copy). Best-effort:
# returns non-zero and does nothing else if no clipboard tool is found —
# callers should fall back to just printing the path.
copy_to_clipboard() {
	if command -v pbcopy >/dev/null 2>&1; then
		printf '%s' "$1" | pbcopy 2>/dev/null && return 0
	elif command -v xclip >/dev/null 2>&1; then
		printf '%s' "$1" | xclip -selection clipboard 2>/dev/null && return 0
	elif command -v xsel >/dev/null 2>&1; then
		printf '%s' "$1" | xsel --clipboard --input 2>/dev/null && return 0
	elif command -v wl-copy >/dev/null 2>&1; then
		printf '%s' "$1" | wl-copy 2>/dev/null && return 0
	fi
	return 1
}

resolve_version() {
	if [ -n "${FIGMA_MAP_VERSION:-}" ]; then
		echo "$FIGMA_MAP_VERSION"
		return
	fi
	# Parse tag_name from the GitHub latest-release API.
	tag="$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
		| grep -m1 '"tag_name"' \
		| sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/')"
	[ -n "$tag" ] || die "could not resolve latest version (set FIGMA_MAP_VERSION)"
	echo "$tag"
}

# ---------------------------------------------------------------------------
# Checksum verification
# ---------------------------------------------------------------------------
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "need sha256sum or shasum to verify the download"
	fi
}

# verify_against_checksums_txt checks $1 (a local file) against the
# `$2 checksums.txt`-shaped file at $3 — used for the CLI archive, which
# goreleaser itself builds and checksums.
verify_against_checksums_txt() {
	file="$1" name="$2" checksums="$3"
	expected="$(grep " ${name}\$" "$checksums" | awk '{print $1}')"
	[ -n "$expected" ] || die "no checksum entry for ${name}"
	actual="$(sha256_of "$file")"
	if [ "$expected" != "$actual" ]; then
		die "checksum mismatch for ${name}!
    expected ${expected}
    actual   ${actual}"
	fi
}

# verify_against_sidecar checks $1 (a local file) against a `<name>.sha256`
# sidecar containing just a hex digest — used for the backend/plugin
# archives, which are extra_files goreleaser's own checksums.txt doesn't
# cover. Prints a warning and returns non-zero instead of dying, since a
# failure here shouldn't block the CLI install that already succeeded.
verify_against_sidecar() {
	file="$1" sidecar="$2" name="$3"
	expected="$(awk '{print $1}' "$sidecar" 2>/dev/null || true)"
	if [ -z "$expected" ]; then
		warn "no checksum available for ${name} — skipping"
		return 1
	fi
	actual="$(sha256_of "$file")"
	if [ "$expected" != "$actual" ]; then
		warn "checksum mismatch for ${name} — skipping"
		return 1
	fi
	return 0
}

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
choose_install_dir() {
	if [ -n "${FIGMA_MAP_INSTALL_DIR:-}" ]; then
		echo "$FIGMA_MAP_INSTALL_DIR"
	elif [ -w /usr/local/bin ] 2>/dev/null; then
		echo "/usr/local/bin"
	else
		echo "$HOME/.local/bin"
	fi
}

# install_backend fetches the standalone backend bundle for TAG into
# ~/.figma-map/versions/${TAG}/backend/figma-bridge — the same path
# internal/service/bridgeproc.go's EnsureBackendBundle checks before
# fetching, so `bridge up` finds it already there. Best-effort: a failure
# here is a warning, not a fatal error — `bridge up`/`figma-map update`
# will just fetch it themselves later.
install_backend() {
	archive="figma-map-backend_${VERSION}_${OS}_${ARCH}.tar.gz"
	info "Downloading" " ${DIM}${archive}${RESET} (backend)"
	if ! download "${BASE_URL}/${archive}" "${TMP}/${archive}" 2>/dev/null; then
		warn "no backend release asset for ${OS}/${ARCH} at ${TAG} — 'bridge up' will fetch it on first use"
		return
	fi
	if ! download "${BASE_URL}/${archive}.sha256" "${TMP}/${archive}.sha256" 2>/dev/null; then
		warn "no checksum for ${archive} — skipping backend install"
		return
	fi
	if ! verify_against_sidecar "${TMP}/${archive}" "${TMP}/${archive}.sha256" "$archive"; then
		return
	fi
	ok "backend checksum verified"

	backend_dir="${HOME}/.figma-map/versions/${TAG}/backend"
	mkdir -p "$backend_dir"
	tar -xzf "${TMP}/${archive}" -C "$backend_dir" || { warn "extract backend failed"; return; }
	chmod +x "${backend_dir}/figma-bridge" 2>/dev/null || true
	ok "backend installed to ${BOLD}${backend_dir}${RESET}"
}

# install_plugin fetches figma-map-plugin.zip for TAG and unpacks it into
# the one fixed path ~/.figma-map/plugin/ — the same path
# internal/service/plugin.go's EnsurePlugin swaps in place on update, so a
# plugin already imported into Figma keeps pointing at the same
# manifest.json. Best-effort, same reasoning as install_backend.
install_plugin() {
	archive="figma-map-plugin.zip"
	info "Downloading" " ${DIM}${archive}${RESET}"
	if ! download "${BASE_URL}/${archive}" "${TMP}/${archive}" 2>/dev/null; then
		warn "no plugin release asset at ${TAG} — download it manually from the releases page"
		return
	fi
	if ! download "${BASE_URL}/${archive}.sha256" "${TMP}/${archive}.sha256" 2>/dev/null; then
		warn "no checksum for ${archive} — skipping plugin install"
		return
	fi
	if ! verify_against_sidecar "${TMP}/${archive}" "${TMP}/${archive}.sha256" "$archive"; then
		return
	fi
	ok "plugin checksum verified"

	command -v unzip >/dev/null 2>&1 || { warn "unzip not found — skipping plugin install"; return; }

	extract_dir="${TMP}/plugin-extract"
	mkdir -p "$extract_dir"
	unzip -q -o "${TMP}/${archive}" -d "$extract_dir" || { warn "extract plugin failed"; return; }
	[ -f "${extract_dir}/figma-map-plugin/manifest.json" ] || { warn "plugin archive missing manifest.json — unexpected layout"; return; }

	plugin_dir="${HOME}/.figma-map/plugin"
	rm -rf "${plugin_dir}.new"
	mv "${extract_dir}/figma-map-plugin" "${plugin_dir}.new"
	echo "$TAG" > "${plugin_dir}.new/.version"
	rm -rf "$plugin_dir"
	mv "${plugin_dir}.new" "$plugin_dir"
	ok "plugin installed to ${BOLD}${plugin_dir}${RESET}"
}

main() {
	banner

	OS="$(detect_os)"
	ARCH="$(detect_arch)"
	info "Platform   " "${BOLD}${OS}/${ARCH}${RESET}"

	TAG="$(resolve_version)"
	case "$TAG" in v*) ;; *) TAG="v${TAG}" ;; esac # a user-supplied FIGMA_MAP_VERSION might omit "v";
	                                                 # normalize so cache paths always agree with Go's release.NormalizeTag
	VERSION="${TAG#v}" # archive names drop the leading v
	info "Version    " "${BOLD}${TAG}${RESET}"

	DIR="$(choose_install_dir)"
	printf '\n%sThis will install:%s\n' "$BOLD" "$RESET"
	printf '  · the figma-map CLI to %s\n' "${DIR}/${BINARY}"
	printf '  · a backend bundle to %s (no Node install needed)\n' "${HOME}/.figma-map/versions/${TAG}/backend/"
	printf '  · the Figma plugin to %s (no build step needed)\n\n' "${HOME}/.figma-map/plugin/"

	ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
	BASE_URL="${FIGMA_MAP_BASE_URL:-https://github.com/${REPO}/releases/download/${TAG}}"

	TMP="$(mktemp -d)"
	trap 'rm -rf "$TMP"' EXIT INT TERM

	info "Downloading" " ${DIM}${ARCHIVE}${RESET} (CLI)"
	download "${BASE_URL}/${ARCHIVE}" "${TMP}/${ARCHIVE}" \
		|| die "download failed — no release asset for ${OS}/${ARCH} at ${TAG}?"

	info "Verifying  " " checksum"
	download "${BASE_URL}/checksums.txt" "${TMP}/checksums.txt" \
		|| die "could not download checksums.txt"
	verify_against_checksums_txt "${TMP}/${ARCHIVE}" "$ARCHIVE" "${TMP}/checksums.txt"
	ok "CLI checksum verified"

	tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP" || die "extract failed"
	[ -f "${TMP}/${BINARY}" ] || die "binary ${BINARY} not found in archive"

	mkdir -p "$DIR" 2>/dev/null || die "cannot create install dir ${DIR}"
	if [ -w "$DIR" ]; then
		install -m 0755 "${TMP}/${BINARY}" "${DIR}/${BINARY}"
	else
		warn "elevating with sudo to write ${DIR}"
		sudo install -m 0755 "${TMP}/${BINARY}" "${DIR}/${BINARY}" \
			|| die "install to ${DIR} failed"
	fi
	ok "CLI installed to ${BOLD}${DIR}/${BINARY}${RESET}"

	printf '\n'
	install_backend
	printf '\n'
	install_plugin

	printf '\n'
	on_path=1
	case ":${PATH}:" in
		*":${DIR}:"*) ;;
		*) on_path=0 ;;
	esac
	if [ "$on_path" = "1" ]; then
		ok "$("${DIR}/${BINARY}" --version 2>/dev/null || echo "$BINARY $TAG")"
	else
		ok "$("${DIR}/${BINARY}" --version 2>/dev/null || echo "$BINARY $TAG")"
		warn "${DIR} is not on your PATH — add it:"
		# shellcheck disable=SC2016 # $PATH must print literally, not expand
		printf '      %sexport PATH="%s:$PATH"%s\n' "$DIM" "$DIR" "$RESET"
	fi

	printf '\n%sInstalled:%s\n' "$BOLD" "$RESET"
	printf '  CLI      %s\n' "${DIR}/${BINARY}"
	printf '  backend  %s\n' "${HOME}/.figma-map/versions/${TAG}/backend/"
	printf '  plugin   %s\n' "${HOME}/.figma-map/plugin/"
	printf '\n%sUpdate:%s   figma-map update\n' "$BOLD" "$RESET"
	printf '%sUninstall:%s figma-map uninstall\n' "$BOLD" "$RESET"

	manifest="${HOME}/.figma-map/plugin/manifest.json"
	plugin_dir="${HOME}/.figma-map/plugin"
	if [ -f "$manifest" ]; then
		printf '\n%sLoad the plugin in Figma (one-time):%s\n' "$BOLD" "$RESET"
		printf '  Figma → Plugins → Development → Import plugin from manifest…\n'
		printf '  select: %s%s%s\n' "$BOLD" "$manifest" "$RESET"
		if copy_to_clipboard "$plugin_dir"; then
			printf '  %s(path copied to your clipboard — in the file dialog, Cmd+Shift+G / Ctrl+L\n' "$DIM"
			printf '  then paste to jump straight there — ~/.figma-map is a hidden folder)%s\n' "$RESET"
		else
			printf '  %s(~/.figma-map is a hidden folder — your file picker won'"'"'t show it by\n' "$DIM"
			printf '  default; use "Go to folder"/"show hidden files" and paste the path above)%s\n' "$RESET"
		fi
		if [ "$OS" = "darwin" ] && command -v open >/dev/null 2>&1; then
			open -R "$manifest" 2>/dev/null || true
		fi
	fi

	printf '\n%sNext:%s open your coding agent and paste:\n\n' "$BOLD" "$RESET"
	printf '  %s"figma-map is installed — read the figma-map-setup skill and finish setting it up for this project."%s\n\n' "$CYAN" "$RESET"
	printf 'No agent? Run %sfigma-map doctor%s yourself to check the bridge, then %sfigma-map init <path>%s.\n' \
		"$CYAN" "$RESET" "$CYAN" "$RESET"
}

main "$@"

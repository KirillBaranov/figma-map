#!/usr/bin/env sh
# figma-map installer.
#
#   curl -fsSL https://raw.githubusercontent.com/kirillbaranov/figma-map/main/install.sh | sh
#
# Environment overrides:
#   FIGMA_MAP_VERSION      install a specific tag (e.g. v0.1.0); default: latest
#   FIGMA_MAP_INSTALL_DIR  install directory; default: /usr/local/bin or ~/.local/bin
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

main() {
	banner

	OS="$(detect_os)"
	ARCH="$(detect_arch)"
	info "Platform   " "${BOLD}${OS}/${ARCH}${RESET}"

	TAG="$(resolve_version)"
	VERSION="${TAG#v}" # archive names drop the leading v
	info "Version    " "${BOLD}${TAG}${RESET}"

	ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
	BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

	TMP="$(mktemp -d)"
	trap 'rm -rf "$TMP"' EXIT INT TERM

	info "Downloading" " ${DIM}${ARCHIVE}${RESET}"
	download "${BASE_URL}/${ARCHIVE}" "${TMP}/${ARCHIVE}" \
		|| die "download failed — no release asset for ${OS}/${ARCH} at ${TAG}?"

	info "Verifying  " " checksum"
	download "${BASE_URL}/checksums.txt" "${TMP}/checksums.txt" \
		|| die "could not download checksums.txt"
	expected="$(grep " ${ARCHIVE}\$" "${TMP}/checksums.txt" | awk '{print $1}')"
	[ -n "$expected" ] || die "no checksum entry for ${ARCHIVE}"
	actual="$(sha256_of "${TMP}/${ARCHIVE}")"
	if [ "$expected" != "$actual" ]; then
		die "checksum mismatch!
    expected ${expected}
    actual   ${actual}"
	fi
	short="$(printf '%s' "$actual" | cut -c1-12)"
	ok "checksum verified ${DIM}(sha256 ${short}…)${RESET}"

	tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP" || die "extract failed"
	[ -f "${TMP}/${BINARY}" ] || die "binary ${BINARY} not found in archive"

	DIR="$(choose_install_dir)"
	mkdir -p "$DIR" 2>/dev/null || die "cannot create install dir ${DIR}"

	if [ -w "$DIR" ]; then
		install -m 0755 "${TMP}/${BINARY}" "${DIR}/${BINARY}"
	else
		warn "elevating with sudo to write ${DIR}"
		sudo install -m 0755 "${TMP}/${BINARY}" "${DIR}/${BINARY}" \
			|| die "install to ${DIR} failed"
	fi
	ok "installed to ${BOLD}${DIR}/${BINARY}${RESET}"

	printf '\n'
	if command -v "$BINARY" >/dev/null 2>&1 && [ "$(command -v "$BINARY")" = "${DIR}/${BINARY}" ]; then
		ok "$("${DIR}/${BINARY}" --version 2>/dev/null || echo "$BINARY $TAG")"
	else
		ok "$("${DIR}/${BINARY}" --version 2>/dev/null || echo "$BINARY $TAG")"
		case ":${PATH}:" in
			*":${DIR}:"*) ;;
			*) warn "${DIR} is not on your PATH — add it:"
			   # shellcheck disable=SC2016 # $PATH must print literally, not expand
			   printf '      %sexport PATH="%s:$PATH"%s\n' "$DIM" "$DIR" "$RESET" ;;
		esac
	fi

	printf '\n%sNext:%s run %sfigma-map doctor%s to verify your setup.\n' \
		"$BOLD" "$RESET" "$CYAN" "$RESET"
	printf '%sThen:%s run %sfigma-map init <path>%s to add the Claude Code skill + config to your project.\n' \
		"$BOLD" "$RESET" "$CYAN" "$RESET"
}

main "$@"

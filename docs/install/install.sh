#!/bin/sh
# Proxa installer — downloads, verifies, and installs a Proxa release
# binary on a fresh Linux (glibc) or macOS host.
#
# Canonical invocation:
#   curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | sh
#
# Environment variables:
#   INSTALL_VERSION  release tag to install (default: latest)
#   INSTALL_DIR      target directory (default: /usr/local/bin)
#   INSTALL_VERIFY   set to "0" to skip SHA-256 verification (NOT RECOMMENDED)
#   INSTALL_BASE_URL override the GitHub release URL base (testing only)
#
# Exit codes:
#   0  success
#   1  unsupported platform (Alpine musl, FreeBSD, Windows, etc.)
#   2  download failed (network / 404 / missing release)
#   3  checksum mismatch (downloaded file does not match published checksum)
#   4  install directory not writable AND sudo unavailable or refused
#   5  neither curl nor wget available
#
# POSIX sh — no bash-isms. Tested with dash + bash + zsh.

set -eu

# ----- defaults ----------------------------------------------------------

INSTALL_VERSION="${INSTALL_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
INSTALL_VERIFY="${INSTALL_VERIFY:-1}"
INSTALL_BASE_URL="${INSTALL_BASE_URL:-https://github.com/proxa-server/proxa/releases}"

# ----- helpers -----------------------------------------------------------

event() { printf '>>> %s\n' "$*"; }
errln() { printf 'error: %s\n' "$*" >&2; }
hintln() { printf 'hint: %s\n' "$*" >&2; }

die_unsupported() {
	errln "$1"
	hintln 'see https://github.com/proxa-server/proxa/releases for manual download'
	exit 1
}

die_download() {
	errln "$1"
	hintln 'check the version exists at https://github.com/proxa-server/proxa/releases'
	exit 2
}

die_checksum() {
	errln "$1"
	hintln 're-run with INSTALL_VERIFY=0 to skip (NOT recommended) or re-download manually'
	exit 3
}

die_write() {
	errln "$1"
	hintln "set INSTALL_DIR=\$HOME/.local/bin and ensure that directory is in PATH, or run with sudo"
	exit 4
}

die_nofetch() {
	errln "neither curl nor wget is available — cannot download anything"
	hintln 'install one: apt-get install -y curl (Debian/Ubuntu) | yum install -y curl (RHEL/Fedora)'
	exit 5
}

# ----- platform detection ------------------------------------------------

event 'detecting platform'

os_raw="$(uname -s)"
case "$os_raw" in
	Linux)
		# Alpine ships musl libc; our binaries are glibc — refuse early.
		if [ -f /etc/alpine-release ]; then
			die_unsupported 'Alpine Linux (musl libc) is not supported in v0.4.2 — Proxa binaries are statically linked but use Go runtime which has no musl prebuilds'
		fi
		os='linux'
		;;
	Darwin)
		os='darwin'
		;;
	FreeBSD|OpenBSD|NetBSD)
		die_unsupported "BSD ($os_raw) is not supported — build from source via: go install github.com/proxa-server/proxa/cmd/proxa@$INSTALL_VERSION"
		;;
	MINGW*|MSYS*|CYGWIN*)
		die_unsupported 'Windows is not supported by install.sh — download the .exe from the releases page'
		;;
	*)
		die_unsupported "unsupported OS: $os_raw"
		;;
esac

arch_raw="$(uname -m)"
case "$arch_raw" in
	x86_64|amd64)
		arch='amd64'
		;;
	aarch64|arm64)
		arch='arm64'
		;;
	*)
		die_unsupported "unsupported architecture: $arch_raw"
		;;
esac

event "detected: $os/$arch"

# ----- pick downloader ---------------------------------------------------

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
else
	die_nofetch
fi

# ----- version resolution ------------------------------------------------

if [ "$INSTALL_VERSION" = "latest" ]; then
	event 'resolving latest version'
	# The GitHub latest-release URL redirects to the actual tag.
	latest_url="$INSTALL_BASE_URL/latest"
	resolved="$(
		if command -v curl >/dev/null 2>&1; then
			curl -fsSL -o /dev/null -w '%{url_effective}' "$latest_url" 2>/dev/null
		else
			echo ''
		fi
	)"
	if [ -n "$resolved" ]; then
		INSTALL_VERSION="${resolved##*/}"
	fi
	if [ -z "$INSTALL_VERSION" ] || [ "$INSTALL_VERSION" = "latest" ]; then
		die_download 'could not resolve latest version (network issue or no releases yet)'
	fi
fi

event "resolved version: $INSTALL_VERSION"

# ----- download archive + checksum ---------------------------------------

archive_name="proxa_${INSTALL_VERSION}_${os}_${arch}.tar.gz"
archive_url="$INSTALL_BASE_URL/download/$INSTALL_VERSION/$archive_name"
checksums_url="$INSTALL_BASE_URL/download/$INSTALL_VERSION/checksums.txt"

tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t proxa-install)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

event "downloading $archive_name"
if ! fetch "$archive_url" "$tmpdir/$archive_name"; then
	die_download "download failed for $archive_url"
fi

if [ "$INSTALL_VERIFY" = "1" ]; then
	event 'downloading checksums.txt'
	if ! fetch "$checksums_url" "$tmpdir/checksums.txt"; then
		die_download "download failed for $checksums_url"
	fi
	event 'verifying SHA-256 checksum'
	expected="$(awk -v f="$archive_name" '$2 == f { print $1 }' "$tmpdir/checksums.txt")"
	if [ -z "$expected" ]; then
		die_checksum "no checksum entry for $archive_name in checksums.txt"
	fi
	if command -v sha256sum >/dev/null 2>&1; then
		actual="$(sha256sum "$tmpdir/$archive_name" | awk '{print $1}')"
	elif command -v shasum >/dev/null 2>&1; then
		actual="$(shasum -a 256 "$tmpdir/$archive_name" | awk '{print $1}')"
	else
		die_checksum 'no sha256sum or shasum binary available'
	fi
	if [ "$expected" != "$actual" ]; then
		die_checksum "checksum mismatch for $archive_name (expected $expected, got $actual)"
	fi
	event "verified: ${actual%??????????????????????????????????????????????????????????}... matches"
else
	event 'SKIPPING checksum verification (INSTALL_VERIFY=0)'
fi

# ----- extract -----------------------------------------------------------

event 'extracting archive'
tar -xzf "$tmpdir/$archive_name" -C "$tmpdir"
if [ ! -f "$tmpdir/proxa" ]; then
	die_download "archive did not contain a proxa binary"
fi

# ----- install -----------------------------------------------------------

mkdir -p "$INSTALL_DIR" 2>/dev/null || true
install_target="$INSTALL_DIR/proxa"

if [ -w "$INSTALL_DIR" ] || [ "$(id -u)" = "0" ]; then
	event "installing $install_target (mode 0755)"
	mv "$tmpdir/proxa" "$install_target"
	chmod 0755 "$install_target"
elif command -v sudo >/dev/null 2>&1; then
	event "installing $install_target (mode 0755, via sudo)"
	sudo mv "$tmpdir/proxa" "$install_target"
	sudo chmod 0755 "$install_target"
else
	die_write "$INSTALL_DIR is not writable and sudo is not available"
fi

# ----- verify install ----------------------------------------------------

event 'verifying installed binary'
installed_version="$("$install_target" version 2>&1 | head -n 1 || true)"
event "installed: $installed_version"
event 'done'

# ----- print suggested service unit --------------------------------------

if [ "$os" = "linux" ]; then
	cat <<'EOF'

Suggested systemd unit (copy to /etc/systemd/system/proxa.service):

  [Unit]
  Description=Proxa orchestrator
  After=network.target

  [Service]
  Type=simple
  ExecStart=/usr/local/bin/proxa server
  Restart=on-failure
  User=proxa
  Environment=PROXA_DATA_DIR=/var/lib/proxa

  [Install]
  WantedBy=multi-user.target

Then: sudo systemctl daemon-reload && sudo systemctl enable --now proxa

EOF
else
	cat <<'EOF'

For macOS / launchd, see https://github.com/proxa-server/proxa#service-management
(or run `proxa server` interactively from a terminal).

EOF
fi

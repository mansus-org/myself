#!/bin/sh
# Installs the myself CLI from GitHub Releases.
#   curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh
# Environment:
#   MYSELF_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   MYSELF_INSTALL_DIR  target directory (default: $HOME/.local/bin)
#   MYSELF_BASE_URL     override the releases URL (testing); expects
#                       $MYSELF_BASE_URL/<tag>/<files>
set -eu

REPO="mansus-org/myself"
BASE_URL="${MYSELF_BASE_URL:-https://github.com/$REPO/releases/download}"
INSTALL_DIR="${MYSELF_INSTALL_DIR:-}"

err() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

have() {
	command -v "$1" >/dev/null 2>&1
}

# fetch URL DEST
fetch() {
	if have curl; then
		curl -fsSL -o "$2" "$1"
	elif have wget; then
		wget -q -O "$2" "$1"
	else
		err "curl or wget is required"
	fi
}

# final URL after redirects, for the /releases/latest lookup
resolve_url() {
	if have curl; then
		curl -fsSL -o /dev/null -w '%{url_effective}' "$1"
	elif have wget; then
		wget -q -S --spider "$1" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r'
	else
		err "curl or wget is required"
	fi
}

sha256_of() {
	if have sha256sum; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif have shasum; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		err "sha256sum or shasum is required to verify the download"
	fi
}

if [ -z "$INSTALL_DIR" ]; then
	[ -n "${HOME:-}" ] || err "HOME is not set; set MYSELF_INSTALL_DIR"
	INSTALL_DIR="$HOME/.local/bin"
fi

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) err "unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) err "unsupported architecture: $(uname -m)" ;;
esac

tag="${MYSELF_VERSION:-}"
if [ -z "$tag" ]; then
	final=$(resolve_url "https://github.com/$REPO/releases/latest") || err "could not resolve the latest release"
	tag="${final##*/}"
	case "$tag" in
	v[0-9]*) ;;
	*) err "could not determine the latest release (got '$final'); set MYSELF_VERSION" ;;
	esac
fi
case "$tag" in
v*) ;;
*) tag="v$tag" ;;
esac
version="${tag#v}"

archive="myself_${version}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t myself)
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Installing myself %s (%s/%s)\n' "$tag" "$os" "$arch"
fetch "$BASE_URL/$tag/$archive" "$tmp/$archive" || err "download failed: $BASE_URL/$tag/$archive"
fetch "$BASE_URL/$tag/checksums.txt" "$tmp/checksums.txt" || err "download failed: $BASE_URL/$tag/checksums.txt"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
[ -n "$want" ] || err "$archive not listed in checksums.txt"
got=$(sha256_of "$tmp/$archive")
[ "$want" = "$got" ] || err "checksum mismatch for $archive (expected $want, got $got)"

mkdir "$tmp/x"
tar -xzf "$tmp/$archive" -C "$tmp/x" myself || err "could not extract $archive"

mkdir -p "$INSTALL_DIR"
# install via a temp name + rename so a running binary is replaced atomically
cp "$tmp/x/myself" "$INSTALL_DIR/.myself.$$"
chmod 755 "$INSTALL_DIR/.myself.$$"
mv -f "$INSTALL_DIR/.myself.$$" "$INSTALL_DIR/myself"

printf 'Installed %s/myself\n' "$INSTALL_DIR"

case ":${PATH:-}:" in
*":$INSTALL_DIR:"*) ;;
*)
	printf '\n%s is not on your PATH. Add it with:\n' "$INSTALL_DIR"
	# shellcheck disable=SC2016
	printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
	;;
esac

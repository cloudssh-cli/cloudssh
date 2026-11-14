#!/bin/sh
# Install cloudssh from a GitHub Release.
#   curl -fsSL https://raw.githubusercontent.com/cloudssh-cli/cloudssh/main/scripts/install.sh | sh
# Environment:
#   VERSION      release to install (e.g. 0.1.0 or v0.1.0); default: latest
#   INSTALL_DIR  where to put the binary; default: /usr/local/bin if writable, else ~/.local/bin
#   CLOUDSSH_RELEASES_URL  override the releases base URL (testing)
set -eu

REPO="cloudssh-cli/cloudssh"
BASE="${CLOUDSSH_RELEASES_URL:-https://github.com/$REPO/releases}"

die() { printf 'install: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

have curl || die "curl is required"
have tar || die "tar is required"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported OS $(uname -s) (use a release archive manually)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac

version="${VERSION:-}"
if [ -z "$version" ]; then
  # /releases/latest redirects to /releases/tag/vX.Y.Z
  url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "$BASE/latest") || die "cannot find the latest release (is the repo public and does a release exist?)"
  version="${url##*/}"
fi
version="${version#v}"

archive="cloudssh_${version}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

printf 'Downloading cloudssh %s (%s/%s)...\n' "$version" "$os" "$arch"
curl -fsSL -o "$tmp/$archive" "$BASE/download/v$version/$archive" || die "download failed: $BASE/download/v$version/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$BASE/download/v$version/checksums.txt" || die "cannot download checksums.txt"

want=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$want" ] || die "$archive not listed in checksums.txt"
if have sha256sum; then got=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif have shasum; then got=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else die "sha256sum or shasum is required to verify the download"; fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" cloudssh || die "archive does not contain cloudssh"

dir="${INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
install -m 0755 "$tmp/cloudssh" "$dir/cloudssh"

printf 'Installed %s/cloudssh\n' "$dir"
case ":$PATH:" in *":$dir:"*) ;; *) printf 'Note: %s is not on your PATH.\n' "$dir" ;; esac
have ssh || printf 'Note: cloudssh needs the ssh client installed.\n'

#!/bin/sh
# Install the release binary into a directory already on PATH.
# Override with PREFIX=... or VERSION=...
set -eu

REPO="markcmarshall/token-top"
VERSION="${VERSION:-v1.0.2}"

on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

writable_dir() {
  [ -d "$1" ] && [ -w "$1" ]
}

pick_prefix() {
  if [ -n "${PREFIX:-}" ]; then
    printf '%s\n' "$PREFIX"
    return
  fi
  # First allowlisted dir already on PATH, in PATH order.
  oldifs=$IFS
  IFS=:
  for dir in $PATH; do
    IFS=$oldifs
    [ -n "$dir" ] || continue
    case "$dir" in
      /opt/homebrew/bin|/usr/local/bin|"$HOME/.local/bin"|"$HOME/bin")
        if writable_dir "$dir" || [ "$dir" = "$HOME/.local/bin" ] || [ "$dir" = "$HOME/bin" ]; then
          printf '%s\n' "$dir"
          return
        fi
        ;;
    esac
  done
  IFS=$oldifs
  printf '%s\n' "$HOME/.local/bin"
}

PREFIX=$(pick_prefix)

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
  darwin|linux) ;;
  *)
    echo "ttop: unsupported OS $os" >&2
    exit 1
    ;;
esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)
    echo "ttop: unsupported arch $arch" >&2
    exit 1
    ;;
esac

asset="ttop-${os}-${arch}"
base="https://github.com/${REPO}/releases/download/${VERSION}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}"
  curl -fsSL "${base}/SHA256SUMS" -o "${tmp}/SHA256SUMS"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmp}/${asset}" "${base}/${asset}"
  wget -qO "${tmp}/SHA256SUMS" "${base}/SHA256SUMS"
else
  echo "ttop: need curl or wget" >&2
  exit 1
fi

expected=$(awk -v f="$asset" '$2 == f { print $1; exit }' "${tmp}/SHA256SUMS")
if [ -z "$expected" ]; then
  echo "ttop: ${asset} not in SHA256SUMS" >&2
  exit 1
fi
if command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "${tmp}/${asset}" | awk '{ print $1 }')
elif command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "${tmp}/${asset}" | awk '{ print $1 }')
else
  echo "ttop: need shasum or sha256sum" >&2
  exit 1
fi
if [ "$got" != "$expected" ]; then
  echo "ttop: checksum mismatch for ${asset}" >&2
  exit 1
fi

mkdir -p "$PREFIX"
install -m 0755 "${tmp}/${asset}" "${PREFIX}/ttop"

echo "installed ${VERSION} -> ${PREFIX}/ttop"
case ":$PATH:" in
  *":${PREFIX}:"*) ;;
  *)
    echo "ttop: ${PREFIX} is not on PATH. Add:" >&2
    echo "  export PATH=\"${PREFIX}:\$PATH\"" >&2
    exit 1
    ;;
esac
"${PREFIX}/ttop" --version

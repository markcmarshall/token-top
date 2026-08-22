#!/bin/sh
# Install the release binary into a directory already on PATH.
# Override with PREFIX=... or VERSION=...
set -eu

REPO="markcmarshall/token-top"
VERSION="${VERSION:-latest}"

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

  # Update the command the shell already resolves. Installing a second copy
  # later on PATH reports success but leaves the old binary active.
  existing=$(command -v ttop 2>/dev/null || true)
  case "$existing" in
    /*/ttop)
      dir=${existing%/ttop}
      case "$dir" in
        /opt/homebrew/bin|/usr/local/bin|"$HOME/.local/bin"|"$HOME/bin")
          if writable_dir "$dir" || [ -w "$existing" ]; then
            printf '%s\n' "$dir"
            return
          fi
          echo "ttop: existing ${existing} is not writable; refusing to install a shadowed copy" >&2
          echo "ttop: rerun with permission to replace it, or set PREFIX explicitly" >&2
          exit 1
          ;;
        *)
          echo "ttop: existing ${existing} is outside the managed install directories" >&2
          echo "ttop: set PREFIX=${dir} explicitly to replace it" >&2
          exit 1
          ;;
      esac
      ;;
  esac

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
resolved_before=$(command -v ttop 2>/dev/null || true)
if [ -n "$resolved_before" ] && [ "$resolved_before" != "${PREFIX}/ttop" ]; then
  echo "ttop: ${PREFIX}/ttop would be shadowed by ${resolved_before}" >&2
  echo "ttop: choose PREFIX=${resolved_before%/ttop} or remove the conflicting command" >&2
  exit 1
fi

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
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi
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
if writable_dir "$PREFIX"; then
  install -m 0755 "${tmp}/${asset}" "${PREFIX}/ttop"
elif [ -w "${PREFIX}/ttop" ]; then
  # Some package-manager bin directories are not writable even though the
  # user owns the existing executable. Replace its contents in place.
  cat "${tmp}/${asset}" > "${PREFIX}/ttop"
  chmod 0755 "${PREFIX}/ttop"
else
  echo "ttop: cannot write ${PREFIX}/ttop" >&2
  exit 1
fi

case ":$PATH:" in
  *":${PREFIX}:"*) ;;
  *)
    echo "ttop: ${PREFIX} is not on PATH. Add:" >&2
    echo "  export PATH=\"${PREFIX}:\$PATH\"" >&2
    exit 1
    ;;
esac
resolved=$(command -v ttop 2>/dev/null || true)
if [ "$resolved" != "${PREFIX}/ttop" ]; then
  echo "ttop: installed ${PREFIX}/ttop, but your shell resolves ${resolved:-no ttop}" >&2
  echo "ttop: refusing to report a shadowed install as successful" >&2
  exit 1
fi
installed_version=$("${PREFIX}/ttop" --version)
echo "installed ${installed_version} -> ${PREFIX}/ttop"

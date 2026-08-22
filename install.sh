#!/bin/sh
# Install the latest release binary for the current user.
# Override with PREFIX=..., VERSION=..., or TTOP_NO_MODIFY_PATH=1.
set -eu

REPO="markcmarshall/token-top"
VERSION="${VERSION:-latest}"
DEFAULT_PREFIX="$HOME/.local/bin"
PREFIX_WAS_SET="${PREFIX+x}"
PREFIX="${PREFIX:-$DEFAULT_PREFIX}"

on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

writable_dir() {
  [ -d "$1" ] && [ -w "$1" ]
}

resolved_before=$(command -v ttop 2>/dev/null || true)
legacy_install=""

if [ -n "$resolved_before" ] && [ "$resolved_before" != "${PREFIX}/ttop" ]; then
  if [ -n "$PREFIX_WAS_SET" ]; then
    echo "ttop: ${PREFIX}/ttop would be shadowed by ${resolved_before}" >&2
    echo "ttop: remove the conflicting command or choose PREFIX=${resolved_before%/ttop}" >&2
    exit 1
  fi

  case "$resolved_before" in
    /opt/homebrew/bin/ttop|/usr/local/bin/ttop|"$HOME/bin/ttop")
      if [ -L "$resolved_before" ]; then
        echo "ttop: ${resolved_before} is managed by another installer; refusing to replace it" >&2
        echo "ttop: use that installer to upgrade or uninstall it first" >&2
        exit 1
      fi
      if [ ! -w "$resolved_before" ]; then
        echo "ttop: legacy install ${resolved_before} is not writable; cannot migrate it" >&2
        exit 1
      fi
      legacy_version=$("$resolved_before" --version 2>/dev/null || true)
      case "$legacy_version" in
        v[0-9]*) ;;
        *)
          echo "ttop: ${resolved_before} is not a recognized Token Top release; refusing to remove it" >&2
          exit 1
          ;;
      esac
      legacy_install="$resolved_before"
      ;;
    *)
      echo "ttop: existing command ${resolved_before} is outside Token Top's standalone install directory" >&2
      echo "ttop: remove it or set PREFIX=${resolved_before%/ttop} explicitly to keep using that location" >&2
      exit 1
      ;;
  esac
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
  cat "${tmp}/${asset}" > "${PREFIX}/ttop"
  chmod 0755 "${PREFIX}/ttop"
else
  echo "ttop: cannot write ${PREFIX}/ttop" >&2
  exit 1
fi

path_configured=""
if ! on_path "$PREFIX"; then
  if [ "$PREFIX" = "$DEFAULT_PREFIX" ] && [ -z "${TTOP_NO_MODIFY_PATH:-}" ]; then
    profile=""
    case "${SHELL##*/}" in
      zsh) profile="$HOME/.zprofile" ;;
      bash)
        if [ -f "$HOME/.bash_profile" ]; then
          profile="$HOME/.bash_profile"
        elif [ -f "$HOME/.bash_login" ]; then
          profile="$HOME/.bash_login"
        else
          profile="$HOME/.profile"
        fi
        ;;
    esac

    if [ -n "$profile" ]; then
      path_line='export PATH="$HOME/.local/bin:$PATH"'
      if ! grep -F "$path_line" "$profile" >/dev/null 2>&1; then
        printf '\n# Token Top\n%s\n' "$path_line" >> "$profile"
      fi
      path_configured="$profile"
    fi
  fi

  if [ -z "$path_configured" ] && ! on_path "$PREFIX"; then
    echo "ttop: ${PREFIX} is not on PATH. Add this to your shell profile:" >&2
    echo "  export PATH=\"${PREFIX}:\$PATH\"" >&2
  fi
fi

# The installer cannot mutate its parent shell. Put the new binary first for
# verification here; the profile change takes effect in the next shell.
PATH="${PREFIX}:$PATH"
export PATH
resolved=$(command -v ttop 2>/dev/null || true)
if [ "$resolved" != "${PREFIX}/ttop" ]; then
  echo "ttop: installed ${PREFIX}/ttop, but this process resolves ${resolved:-no ttop}" >&2
  exit 1
fi

installed_version=$("${PREFIX}/ttop" --version)
if [ -n "$legacy_install" ]; then
  rm -f "$legacy_install"
  echo "migrated ${legacy_install} -> ${PREFIX}/ttop"
fi
echo "installed ${installed_version} -> ${PREFIX}/ttop"
if [ -n "$path_configured" ]; then
  echo "PATH configured in ${path_configured}; open a new terminal to run ttop"
fi

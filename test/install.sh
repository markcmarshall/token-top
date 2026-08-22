#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

fixture_dir="$test_root/fixture"
fake_bin="$test_root/fake-bin"
mkdir -p "$fixture_dir" "$fake_bin"

cat > "$fixture_dir/ttop-darwin-arm64" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
  echo "v-test"
fi
EOF
chmod 0755 "$fixture_dir/ttop-darwin-arm64"
if command -v shasum >/dev/null 2>&1; then
  fixture_sum=$(shasum -a 256 "$fixture_dir/ttop-darwin-arm64" | awk '{ print $1 }')
else
  fixture_sum=$(sha256sum "$fixture_dir/ttop-darwin-arm64" | awk '{ print $1 }')
fi
printf '%s  %s\n' "$fixture_sum" "ttop-darwin-arm64" > "$fixture_dir/SHA256SUMS"

cat > "$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s) echo Darwin ;;
  -m) echo arm64 ;;
  *) exit 1 ;;
esac
EOF
chmod 0755 "$fake_bin/uname"

cat > "$fake_bin/curl" <<'EOF'
#!/bin/sh
set -eu
url=""
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
case "$url" in
  */SHA256SUMS) cp "$TTOP_TEST_FIXTURE/SHA256SUMS" "$output" ;;
  *) cp "$TTOP_TEST_FIXTURE/ttop-darwin-arm64" "$output" ;;
esac
EOF
chmod 0755 "$fake_bin/curl"

base_path="$fake_bin:/usr/bin:/bin"

fail() {
  echo "installer test: $*" >&2
  exit 1
}

# A clean zsh install owns ~/.local/bin and adds it to PATH exactly once.
clean_home="$test_root/clean-home"
mkdir -p "$clean_home"
HOME="$clean_home" SHELL=/bin/zsh PATH="$base_path" TTOP_TEST_FIXTURE="$fixture_dir" \
  sh "$repo/install.sh" > "$test_root/clean-first.out"
[ -x "$clean_home/.local/bin/ttop" ] || fail "clean install did not create ttop"
[ -f "$clean_home/.zprofile" ] || fail "clean install did not create .zprofile"
[ "$(grep -c '# Token Top' "$clean_home/.zprofile")" -eq 1 ] || fail "PATH marker missing"

HOME="$clean_home" SHELL=/bin/zsh PATH="$base_path" TTOP_TEST_FIXTURE="$fixture_dir" \
  sh "$repo/install.sh" > "$test_root/clean-second.out"
[ "$(grep -c '# Token Top' "$clean_home/.zprofile")" -eq 1 ] || fail "PATH edit is not idempotent"

# The opt-out installs successfully without touching a profile.
no_modify_home="$test_root/no-modify-home"
mkdir -p "$no_modify_home"
HOME="$no_modify_home" SHELL=/bin/zsh PATH="$base_path" TTOP_TEST_FIXTURE="$fixture_dir" \
  TTOP_NO_MODIFY_PATH=1 sh "$repo/install.sh" > "$test_root/no-modify.out" 2>&1
[ -x "$no_modify_home/.local/bin/ttop" ] || fail "opt-out did not install ttop"
[ ! -e "$no_modify_home/.zprofile" ] || fail "opt-out modified .zprofile"

# A regular file created by the old installer migrates out of ~/bin.
migrate_home="$test_root/migrate-home"
mkdir -p "$migrate_home/bin"
cp "$fixture_dir/ttop-darwin-arm64" "$migrate_home/bin/ttop"
HOME="$migrate_home" SHELL=/bin/zsh PATH="$migrate_home/bin:$base_path" \
  TTOP_TEST_FIXTURE="$fixture_dir" sh "$repo/install.sh" > "$test_root/migrate.out"
[ -x "$migrate_home/.local/bin/ttop" ] || fail "migration did not install new copy"
[ ! -e "$migrate_home/bin/ttop" ] || fail "migration left the old copy"
grep -F "migrated $migrate_home/bin/ttop" "$test_root/migrate.out" >/dev/null || fail "migration was not reported"

# A symlink in an old managed location is treated as externally owned.
symlink_home="$test_root/symlink-home"
mkdir -p "$symlink_home/bin" "$symlink_home/Cellar/ttop/1/bin"
cp "$fixture_dir/ttop-darwin-arm64" "$symlink_home/Cellar/ttop/1/bin/ttop"
ln -s "../Cellar/ttop/1/bin/ttop" "$symlink_home/bin/ttop"
if HOME="$symlink_home" SHELL=/bin/zsh PATH="$symlink_home/bin:$base_path" \
  TTOP_TEST_FIXTURE="$fixture_dir" sh "$repo/install.sh" > "$test_root/symlink.out" 2>&1; then
  fail "package-manager symlink was accepted"
fi
[ -L "$symlink_home/bin/ttop" ] || fail "package-manager symlink was changed"

echo "installer behavior: ok"

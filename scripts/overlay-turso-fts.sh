#!/usr/bin/env bash
# Overlay an FTS-enabled libturso_sync_sdk_kit onto the tursogo module cache.
#
# Published turso-go-platform-libs builds omit the sdk-kit `fts` feature, so
# CREATE INDEX ... USING fts fails with "unknown module name 'fts'".
# This script builds turso_sync_sdk_kit with that feature (or copies
# TURSO_FTS_LIB) and replaces the embedded library for this OS/arch.
set -euo pipefail

TURSO_REF="${TURSO_FTS_REF:-36da5b2e435cb07bba3bed2c7e7eef236b6b2e64}"
PLATFORM_LIBS_VERSION="${TURSO_PLATFORM_LIBS_VERSION:-v0.8.1}"
SRC="${TURSO_FTS_SRC:-${XDG_CACHE_HOME:-$HOME/.cache}/congee/turso-fts}"

if ! command -v go >/dev/null 2>&1; then
	echo "go is required" >&2
	exit 1
fi

MOD="$(go env GOMODCACHE)/github.com/tursodatabase/turso-go-platform-libs@${PLATFORM_LIBS_VERSION}"
if [[ ! -d "$MOD/libs" ]]; then
	echo "platform libs module is not downloaded at $MOD" >&2
	echo "run: go mod download" >&2
	exit 1
fi

uname_s="$(uname -s)"
uname_m="$(uname -m)"
case "$uname_s" in
Linux*) os=linux ;;
Darwin*) os=darwin ;;
*)
	echo "unsupported OS: $uname_s" >&2
	exit 1
	;;
esac
case "$uname_m" in
x86_64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*)
	echo "unsupported arch: $uname_m" >&2
	exit 1
	;;
esac
libc=""
if [[ "$os" == linux ]]; then
	if [[ -f /etc/alpine-release ]] || ldd --version 2>&1 | grep -qi musl; then
		libc="_musl"
	fi
fi
case "$os" in
linux) filename="libturso_sync_sdk_kit.so" ;;
darwin) filename="libturso_sync_sdk_kit.dylib" ;;
esac
dest_dir="$MOD/libs/${os}_${arch}${libc}"
dest="$dest_dir/$filename"

has_fts() {
	local file="$1"
	[[ -f "$file" ]] || return 1
	# grep -q under pipefail exits 1 when strings is closed early, so match in a subshell.
	local hits
	hits="$(strings "$file" | grep -F tantivy || true)"
	[[ -n "$hits" ]]
}

stamp="${dest}.fts-ref"
if [[ -z "${TURSO_FTS_LIB:-}" ]] && has_fts "$dest" && [[ "$(cat "$stamp" 2>/dev/null || true)" == "$TURSO_REF" ]]; then
	echo "turso FTS library already overlaid at $dest"
	exit 0
fi

if [[ -n "${TURSO_FTS_LIB:-}" ]]; then
	lib="$TURSO_FTS_LIB"
else
	if ! command -v cargo >/dev/null 2>&1; then
		echo "cargo is required to build the FTS-enabled turso library" >&2
		exit 1
	fi
	if [[ ! -d "$SRC/.git" ]]; then
		mkdir -p "$(dirname "$SRC")"
		git clone --depth 1 "https://github.com/tursodatabase/turso.git" "$SRC"
	fi
	git -C "$SRC" fetch --depth 1 origin "$TURSO_REF"
	git -C "$SRC" checkout --detach "$TURSO_REF"
	profile_dir="lib-release"
	if [[ -n "$libc" ]]; then
		case "$arch" in
		amd64) target="x86_64-unknown-linux-musl" ;;
		arm64) target="aarch64-unknown-linux-musl" ;;
		esac
		export RUSTFLAGS="${RUSTFLAGS:-} -C target-feature=-crt-static"
		rustup target add "$target"
		cargo build --manifest-path "$SRC/Cargo.toml" --profile lib-release --package turso_sync_sdk_kit --features fts --target "$target"
		lib="$SRC/target/${target}/${profile_dir}/$filename"
	else
		cargo build --manifest-path "$SRC/Cargo.toml" --profile lib-release --package turso_sync_sdk_kit --features fts
		lib="$SRC/target/${profile_dir}/$filename"
	fi
fi

if [[ ! -f "$lib" ]]; then
	echo "built library not found: $lib" >&2
	exit 1
fi
if ! has_fts "$lib"; then
	echo "library has no tantivy/FTS symbols: $lib" >&2
	exit 1
fi

chmod -R u+w "$dest_dir"
cp "$lib" "$dest"
if command -v shasum >/dev/null 2>&1; then
	shasum -a 256 "$dest" | awk '{print $1}' >"${dest}.sha256"
else
	sha256sum "$dest" | awk '{print $1}' >"${dest}.sha256"
fi
printf '%s\n' "$TURSO_REF" >"$stamp"
echo "overlaid FTS-enabled turso library at $dest"

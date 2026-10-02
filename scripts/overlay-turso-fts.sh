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
	echo "platform libs module is not downloaded at $MOD; running go mod download" >&2
	go mod download
fi
if [[ ! -d "$MOD/libs" ]]; then
	echo "platform libs module is not downloaded at $MOD" >&2
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

sha256_file() {
	local file="$1"
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
	else
		sha256sum "$file" | awk '{print $1}'
	fi
}

# os.UserCacheDir, including its os.TempDir fallback when that call fails.
go_user_cache_dir() {
	case "$os" in
	darwin)
		if [[ -n "${HOME:-}" ]]; then
			printf '%s\n' "${HOME}/Library/Caches"
			return
		fi
		;;
	linux)
		if [[ -n "${XDG_CACHE_HOME:-}" ]]; then
			case "$XDG_CACHE_HOME" in
			/*)
				printf '%s\n' "$XDG_CACHE_HOME"
				return
				;;
			esac
		elif [[ -n "${HOME:-}" ]]; then
			printf '%s\n' "${HOME}/.cache"
			return
		fi
		;;
	esac
	printf '%s\n' "${TMPDIR:-/tmp}"
}

# Seed the runtime cache tursogo reads in embeddedLibraryTryCreate
# ($TURSO_GO_CACHE_DIR or os.UserCacheDir()/turso-go/<sha256[:8]>/<filename>).
# The loader writes that file in place with no cross-process lock. go test -race
# starts many packages together, so one process can stat a peer's partial extract
# and panic on a hash mismatch. A complete file lets every process take the
# existing-cache path.
seed_runtime_cache() {
	local digest hash_file recorded cache_root cache_dir library_path tmp existing
	digest="$(sha256_file "$dest")"
	hash_file="${dest}.sha256"
	recorded=""
	if [[ -f "$hash_file" ]]; then
		recorded="$(tr -d '[:space:]' <"$hash_file")"
	fi
	if [[ "$recorded" != "$digest" ]]; then
		printf '%s\n' "$digest" >"$hash_file"
	fi

	if [[ -n "${TURSO_GO_CACHE_DIR:-}" ]]; then
		cache_root="$TURSO_GO_CACHE_DIR"
	else
		cache_root="$(go_user_cache_dir)"
	fi
	cache_dir="${cache_root}/turso-go/${digest:0:8}"
	mkdir -p "$cache_dir"
	library_path="${cache_dir}/${filename}"

	if [[ -f "$library_path" ]]; then
		existing="$(sha256_file "$library_path")"
		if [[ "$existing" == "$digest" ]]; then
			echo "turso runtime cache already seeded at $library_path"
			return 0
		fi
	fi

	tmp="$(mktemp "${cache_dir}/.${filename}.XXXXXX")"
	chmod 0755 "$tmp"
	if ! dd if="$dest" of="$tmp" bs=1048576 conv=fsync status=none; then
		rm -f "$tmp"
		echo "failed to write turso runtime cache at $library_path" >&2
		exit 1
	fi
	mv -f "$tmp" "$library_path"
	echo "seeded turso runtime cache at $library_path"
}

stamp="${dest}.fts-ref"
if [[ -z "${TURSO_FTS_LIB:-}" ]] && has_fts "$dest" && [[ "$(cat "$stamp" 2>/dev/null || true)" == "$TURSO_REF" ]]; then
	echo "turso FTS library already overlaid at $dest"
	seed_runtime_cache
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
printf '%s\n' "$(sha256_file "$dest")" >"${dest}.sha256"
printf '%s\n' "$TURSO_REF" >"$stamp"
seed_runtime_cache
echo "overlaid FTS-enabled turso library at $dest"

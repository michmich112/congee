#!/usr/bin/env bash
# Fail unless VERSION is plain MAJOR.MINOR.PATCH and matches the admin package.
# Bash 3.2 compatible (macOS /bin/bash).
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ver="$(tr -d '[:space:]' < VERSION)"
if ! printf '%s\n' "$ver" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "VERSION must be MAJOR.MINOR.PATCH (no v prefix, no -rc/-nightly); got: ${ver}" >&2
	exit 1
fi

pkg="$(node -p "require('./web/admin/package.json').version")"
lock="$(node -p "require('./web/admin/package-lock.json').version")"
lockpkg="$(node -p "require('./web/admin/package-lock.json').packages[''].version")"

if [ "$pkg" != "$ver" ] || [ "$lock" != "$ver" ] || [ "$lockpkg" != "$ver" ]; then
	echo "version mismatch: VERSION=${ver} package.json=${pkg} package-lock.json=${lock} packages['']=${lockpkg}" >&2
	exit 1
fi

echo "version ${ver} matches the admin package"

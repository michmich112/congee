#!/usr/bin/env bash
# Increment VERSION and the admin package to the same MAJOR.MINOR.PATCH.
# Usage: scripts/bump-version.sh patch|minor|major
# Bash 3.2 compatible (macOS /bin/bash).
set -eu

PART="${1:-}"
case "$PART" in
patch | minor | major) ;;
*)
	echo "usage: scripts/bump-version.sh patch|minor|major" >&2
	exit 1
	;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

current="$(tr -d '[:space:]' < VERSION)"
if ! printf '%s\n' "$current" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "VERSION must be MAJOR.MINOR.PATCH before bumping; got: ${current}" >&2
	exit 1
fi

major="${current%%.*}"
rest="${current#*.}"
minor="${rest%%.*}"
patch="${rest#*.}"

case "$PART" in
major)
	major=$((major + 1))
	minor=0
	patch=0
	;;
minor)
	minor=$((minor + 1))
	patch=0
	;;
patch)
	patch=$((patch + 1))
	;;
esac

next="${major}.${minor}.${patch}"
printf '%s\n' "$next" > VERSION

node --input-type=module -e '
import { readFileSync, writeFileSync } from "node:fs";
const version = process.argv[1];
function replaceFirstVersions(text, next, count) {
  let n = 0;
  return text.replace(/("version"\s*:\s*")[^"]+(")/g, (all, open, close) => {
    n += 1;
    if (n <= count) return open + next + close;
    return all;
  });
}
const pkgPath = "web/admin/package.json";
const lockPath = "web/admin/package-lock.json";
writeFileSync(pkgPath, replaceFirstVersions(readFileSync(pkgPath, "utf8"), version, 1));
writeFileSync(lockPath, replaceFirstVersions(readFileSync(lockPath, "utf8"), version, 2));
' "$next"

echo "bumped version ${current} -> ${next}"

#!/usr/bin/env bash
set -euo pipefail

# Build the pinned upstream theme with YuanLook's tracked changes. Work in a
# temporary tree so builds never modify the upstream checkout or require a fork.
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
theme_source="${1:-../pika-default-theme}"
theme_output="${2:-themes/default}"
theme_ref="$(tr -d '\r\n' < "$project_dir/.github/default-theme.ref")"
staging="$(mktemp -d "${TMPDIR:-/tmp}/yuanlook-theme.XXXXXXXX")"
staging="$(cd "$staging" && pwd -P)"
trap 'rm -rf "$staging"' EXIT

git -C "$theme_source" archive "$theme_ref" | tar -x -C "$staging"
git -C "$staging" apply "$project_dir/patches/default-theme-device-access.patch"
(
    cd "$staging"
    npm ci
    npm run build
    npm run lint
)

mkdir -p "$theme_output/dist"
cp "$staging/pika-theme.json" "$theme_output/pika-theme.json"
# Preserve old hashed assets for open browser tabs during an upgrade.
cp -R "$staging/dist/." "$theme_output/dist/"

#!/usr/bin/env bash
# Release smoke test (AT-39): run the complete CLI flow with an isolated
# data directory, then verify the same flow on a cross-compiled binary.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

data_dir="$(mktemp -d)"
csv_out="$data_dir/export.csv"
backup_out="$data_dir/backup.db"
bin_dir="$(mktemp -d)"
trap 'rm -rf "$data_dir" "$bin_dir"' EXIT

platforms=(
  "linux/amd64"
  "linux/arm64"
  "darwin/arm64"
  "windows/amd64"
)

smoke() {
  local exe="$1"
  local dir="$2"
  local ext="${3:-}"

  "$exe" --data-dir "$dir" init
  "$exe" --data-dir "$dir" start "smoke tracking"
  "$exe" --data-dir "$dir" stop
  "$exe" --data-dir "$dir" projects add "smoke-project"
  "$exe" --data-dir "$dir" --json status
  "$exe" --data-dir "$dir" entries list
  "$exe" --data-dir "$dir" report
  "$exe" --data-dir "$dir" export csv --output "$dir/export$ext"
  test -s "$dir/export$ext"
  "$exe" --data-dir "$dir" backup "$dir/backup$ext"
  test -s "$dir/backup$ext"
  "$exe" --data-dir "$dir" doctor > /dev/null
  echo "smoke ok: $exe"
}

echo "== host build smoke =="
go build -o "$bin_dir/timetracker-host" ./cmd/timetracker
smoke "$bin_dir/timetracker-host" "$data_dir/host"

for platform in "${platforms[@]}"; do
  os="${platform%/*}"
  arch="${platform#*/}"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  exe="$bin_dir/timetracker-$os-$arch$ext"
  echo "== cross-compile $os/$arch =="
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$exe" ./cmd/timetracker
  test -s "$exe"
done

echo "release smoke passed: host flow verified, all platform artifacts built"

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/runtime-sdk-tidy.XXXXXX")"
cleanup() {
  cp "$tmpdir/go.mod" go.mod
  cp "$tmpdir/go.sum" go.sum
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

cp go.mod "$tmpdir/go.mod"
cp go.sum "$tmpdir/go.sum"

go mod tidy

if ! cmp -s go.mod "$tmpdir/go.mod" || ! cmp -s go.sum "$tmpdir/go.sum"; then
  echo "go.mod or go.sum is not tidy; run go mod tidy"
  exit 1
fi

echo "go module tidy verification passed"

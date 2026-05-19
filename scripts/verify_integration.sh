#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ETCD_ENDPOINT="${ETCD_ENDPOINT:-127.0.0.1:2379}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required for integration verification."
  exit 1
fi

if ! curl -fsS "http://${ETCD_ENDPOINT}/version" >/dev/null; then
  echo "etcd is required at ${ETCD_ENDPOINT}; set ETCD_ENDPOINT or start etcd"
  exit 1
fi

ETCD_INTEGRATION=1 ETCD_ENDPOINT="$ETCD_ENDPOINT" go test ./runtime/config ./runtime/registry

echo "runtime-sdk integration verification passed"

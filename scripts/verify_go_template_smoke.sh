#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_TEMPLATE_ROOT="${GO_TEMPLATE_ROOT:-$(cd "$ROOT/../go-template" 2>/dev/null && pwd || true)}"

if [[ -z "$GO_TEMPLATE_ROOT" || ! -d "$GO_TEMPLATE_ROOT" ]]; then
  echo "go-template checkout not found; set GO_TEMPLATE_ROOT to run consumer smoke tests"
  exit 1
fi

cd "$GO_TEMPLATE_ROOT"

bash scripts/smoke_external.sh
bash scripts/smoke_monolith.sh
MONOLITH_REGISTRY_PROVIDER=etcd bash scripts/smoke_monolith.sh

echo "go-template consumer smoke verification passed"

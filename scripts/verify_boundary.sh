#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v rg >/dev/null 2>&1; then
  echo "rg is required for boundary verification."
  exit 1
fi

check_external_internal_imports() {
  if rg -n --glob '!**/*_test.go' --glob '!scripts/**' --glob '!docs/**' --glob '!examples/**' --glob '!*.md' --glob '!go.sum' '"[^" ]*/internal/[^" ]*"' . \
    | rg -v '"github.com/opencode-sig/runtime-sdk/[^" ]*/internal/[^" ]*"'; then
    echo "boundary check failed: sdk must not import external internal packages"
    exit 1
  fi
}

check_absent() {
  local name="$1"
  local pattern="$2"
  shift 2

  if rg -n --glob '!**/*_test.go' --glob '!scripts/**' --glob '!docs/**' --glob '!examples/**' --glob '!*.md' --glob '!go.sum' "$pattern" "$@"; then
    echo "boundary check failed: ${name}"
    exit 1
  fi
}

check_project_defaults() {
  local matches
  matches="$(rg -n --glob '!**/*_test.go' --glob '!scripts/**' --glob '!docs/**' --glob '!examples/**' --glob '!*.md' --glob '!go.sum' \
    'go-template|configs/|/go-template/' . \
    | rg -v '^\./servicekit/config_loader\.go:.*configs/service' || true)"
  if [[ -n "$matches" ]]; then
    echo "$matches"
    echo "boundary check failed: sdk must not carry go-template project defaults"
    exit 1
  fi
}

check_external_internal_imports

check_project_defaults

check_absent \
  "sdk non-test code must not hardcode template service names" \
  '"(user|order|runtimeadmin)"' \
  .

check_absent \
  "servicekit must not depend on business protocols, HTTP frameworks, or platform applications" \
  'protobuf/|github.com/gin-gonic/gin|response envelope|runtimeadmin' \
  servicekit

check_absent \
  "core runtime must not depend on optional business infra" \
  'github.com/opencode-sig/runtime-sdk/infra/(mysql|redis|kafka)' \
  runtime logger rpcerror apperror observability

check_absent \
  "core runtime must not depend on servicekit facade" \
  'github.com/opencode-sig/runtime-sdk/servicekit' \
  runtime

check_absent \
  "logger, rpcerror and apperror must not depend on runtime or infra" \
  'github.com/opencode-sig/runtime-sdk/(runtime|infra)/' \
  logger rpcerror apperror

echo "runtime-sdk boundary verification passed"

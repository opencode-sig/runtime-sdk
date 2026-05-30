#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAMPLE_ROOT="$ROOT/examples/go-template-payment"
ETCD_ENDPOINT="${ETCD_ENDPOINT:-127.0.0.1:2379}"
ETCD_CONTAINER="${ETCD_CONTAINER:-etcd}"
REGISTRY_PREFIX="${REGISTRY_PREFIX:-/runtime/registry}"
PAYMENT_HEALTH="${PAYMENT_HEALTH:-http://127.0.0.1:9104/healthz}"
USER_HEALTH="${USER_HEALTH:-http://127.0.0.1:9105/healthz}"
TMP_DIR="$(mktemp -d)"
SERVICE_PID=""

cleanup() {
  if [[ -n "$SERVICE_PID" ]] && kill -0 "$SERVICE_PID" >/dev/null 2>&1; then
    kill "$SERVICE_PID" >/dev/null 2>&1 || true
    wait "$SERVICE_PID" >/dev/null 2>&1 || true
  fi
  for port in 9004 9005 9104 9105; do
    pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$pids" ]]; then
      kill $pids >/dev/null 2>&1 || true
    fi
  done
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

etcdctl_cmd() {
  if [[ -n "${ETCDCTL:-}" ]]; then
    ${ETCDCTL} "$@"
  else
    docker exec "$ETCD_CONTAINER" etcdctl "$@"
  fi
}

require_port_free() {
  local port="$1"
  if lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port $port is already in use"
    exit 1
  fi
}

wait_for_http_ok() {
  local name="$1"
  local url="$2"
  local deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for $name health at $url"
  echo "--- distributed log ---"
  sed -n '1,240p' "$TMP_DIR/distributed.log" || true
  exit 1
}

registry_key_count() {
  etcdctl_cmd get --prefix --keys-only "$REGISTRY_PREFIX" | grep -c "^${REGISTRY_PREFIX}/" || true
}

wait_for_registry_keys() {
  local want="$1"
  local deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    local count
    count="$(registry_key_count)"
    if (( count >= want )); then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for at least $want registry keys under $REGISTRY_PREFIX"
  etcdctl_cmd get --prefix "$REGISTRY_PREFIX" || true
  echo "--- distributed log ---"
  sed -n '1,240p' "$TMP_DIR/distributed.log" || true
  exit 1
}

for port in 9004 9005 9104 9105; do
  require_port_free "$port"
done

if ! curl -fsS "http://${ETCD_ENDPOINT}/version" >/dev/null; then
  echo "etcd is required at ${ETCD_ENDPOINT}; set ETCD_ENDPOINT or start etcd"
  exit 1
fi

etcdctl_cmd del --prefix "$REGISTRY_PREFIX" >/dev/null

cd "$EXAMPLE_ROOT"
go run ./cmd/distributed/main.go >"$TMP_DIR/distributed.log" 2>&1 &
SERVICE_PID="$!"

wait_for_http_ok "payment" "$PAYMENT_HEALTH"
wait_for_http_ok "user" "$USER_HEALTH"
wait_for_registry_keys 2

go run ./cmd/client/main.go >/dev/null

echo "distributed smoke verification passed"

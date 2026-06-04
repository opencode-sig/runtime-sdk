#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAMPLE_ROOT="$ROOT/examples/go-template-payment"
ETCD_ENDPOINT="${ETCD_ENDPOINT:-127.0.0.1:2379}"
ETCD_CONTAINER="${ETCD_CONTAINER:-etcd}"
REGISTRY_PREFIX="${REGISTRY_PREFIX:-/runtime/registry}"
PAYMENT_HEALTH="${PAYMENT_HEALTH:-http://127.0.0.1:9104/healthz}"
USER_HEALTH="${USER_HEALTH:-http://127.0.0.1:9105/healthz}"
PAYMENT_READY="${PAYMENT_READY:-http://127.0.0.1:9104/readyz}"
USER_READY="${USER_READY:-http://127.0.0.1:9105/readyz}"
TMP_DIR="$(mktemp -d)"
SERVICE_PID=""
STOPPED_ETCD="false"

cleanup() {
  if [[ "$STOPPED_ETCD" == "true" ]]; then
    docker restart "$ETCD_CONTAINER" >/dev/null 2>&1 || true
  fi
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

wait_for_http_status() {
  local name="$1"
  local url="$2"
  local want="$3"
  local deadline=$((SECONDS + 40))
  while (( SECONDS < deadline )); do
    local status
    status="$(curl -sS -o /dev/null -w "%{http_code}" --max-time 5 "$url" 2>/dev/null || true)"
    if [[ "$status" == "$want" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for $name health status $want at $url"
  echo "--- distributed log ---"
  sed -n '1,260p' "$TMP_DIR/distributed.log" || true
  exit 1
}

registry_key_count() {
  etcdctl_cmd get --prefix --keys-only "$REGISTRY_PREFIX" | grep -c "^${REGISTRY_PREFIX}/" || true
}

wait_for_registry_keys() {
  local want="$1"
  local deadline=$((SECONDS + 40))
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
  sed -n '1,260p' "$TMP_DIR/distributed.log" || true
  exit 1
}

wait_for_etcd() {
  local deadline=$((SECONDS + 40))
  while (( SECONDS < deadline )); do
    if curl -fsS "http://${ETCD_ENDPOINT}/version" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for etcd at ${ETCD_ENDPOINT}"
  exit 1
}

for port in 9004 9005 9104 9105; do
  require_port_free "$port"
done

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for resilience verification"
  exit 1
fi

wait_for_etcd
etcdctl_cmd del --prefix "$REGISTRY_PREFIX" >/dev/null

cd "$EXAMPLE_ROOT"
go run ./cmd/distributed/main.go >"$TMP_DIR/distributed.log" 2>&1 &
SERVICE_PID="$!"

wait_for_http_status "payment" "$PAYMENT_HEALTH" "200"
wait_for_http_status "user" "$USER_HEALTH" "200"
wait_for_http_status "payment readiness" "$PAYMENT_READY" "200"
wait_for_http_status "user readiness" "$USER_READY" "200"
wait_for_registry_keys 2
go run ./cmd/client/main.go >/dev/null

etcdctl_cmd del --prefix "$REGISTRY_PREFIX" >/dev/null
wait_for_registry_keys 2
go run ./cmd/client/main.go >/dev/null

docker stop "$ETCD_CONTAINER" >/dev/null
STOPPED_ETCD="true"
wait_for_http_status "payment" "$PAYMENT_HEALTH" "200"
wait_for_http_status "user" "$USER_HEALTH" "200"
wait_for_http_status "payment readiness" "$PAYMENT_READY" "200"
wait_for_http_status "user readiness" "$USER_READY" "200"

docker restart "$ETCD_CONTAINER" >/dev/null
STOPPED_ETCD="false"
wait_for_etcd
wait_for_http_status "payment" "$PAYMENT_HEALTH" "200"
wait_for_http_status "user" "$USER_HEALTH" "200"
wait_for_http_status "payment readiness" "$PAYMENT_READY" "200"
wait_for_http_status "user readiness" "$USER_READY" "200"
wait_for_registry_keys 2
go run ./cmd/client/main.go >/dev/null

echo "distributed resilience verification passed"

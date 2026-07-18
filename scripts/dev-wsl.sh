#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="${repo_root}/backend/docker-compose.dev-deps.yml"
dev_data="${repo_root}/.dev-data"
frontend_dir="${repo_root}/frontend"

prepare_data() {
  mkdir -p "${dev_data}/images/.staging" "${dev_data}/images/v2"
  chmod 0750 "${dev_data}/images" "${dev_data}/images/.staging" "${dev_data}/images/v2"
}

prepare_cert() {
  local key_file="${frontend_dir}/localhost+1-key.pem"
  local cert_file="${frontend_dir}/localhost+1.pem"
  if [[ -f "$key_file" && -f "$cert_file" ]]; then
    return
  fi
  command -v openssl >/dev/null 2>&1 || {
    echo "openssl is required to create the localhost development certificate" >&2
    exit 1
  }
  openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -keyout "$key_file" \
    -out "$cert_file" \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" >/dev/null 2>&1
  chmod 0600 "$key_file"
  chmod 0644 "$cert_file"
}

export_backend_env() {
  prepare_data
  export GOCACHE="${GOCACHE:-/tmp/summerain-go-cache}"
  export GOTMPDIR="${GOTMPDIR:-/tmp}"
  export GOMEMLIMIT="${GOMEMLIMIT:-512MiB}"
  export SERVER_PORT="${SERVER_PORT:-${SUMMERAIN_DEV_BACKEND_PORT:-18080}}"
  export GIN_MODE="${GIN_MODE:-debug}"
  export COOKIE_SECRET="${COOKIE_SECRET:-summerain-local-development-only}"
  export DB_HOST="${DB_HOST:-127.0.0.1}"
  export DB_PORT="${DB_PORT:-${SUMMERAIN_DEV_MYSQL_PORT:-13306}}"
  export DB_USER="${DB_USER:-root}"
  export DB_PASSWORD="${DB_PASSWORD:-summerain-dev}"
  export DB_NAME="${DB_NAME:-summerain}"
  export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:${SUMMERAIN_DEV_REDIS_PORT:-16379}}"
  export IMGPROXY_URL="${IMGPROXY_URL:-http://127.0.0.1:${SUMMERAIN_DEV_IMGPROXY_PORT:-18081}}"
  export IMGPROXY_PUBLIC_URL="${IMGPROXY_PUBLIC_URL:-/img}"
  export IMGPROXY_LOCAL_FILESYSTEM_ROOT="${IMGPROXY_LOCAL_FILESYSTEM_ROOT:-${dev_data}}"
  export STORAGE_PATH="${STORAGE_PATH:-${dev_data}/images}"
  export TEMP_PATH="${TEMP_PATH:-${dev_data}/images/.staging}"
  export V2_STAGING_PATH="${V2_STAGING_PATH:-${dev_data}/images/.staging}"
  export V2_UPLOAD_ENABLED="${V2_UPLOAD_ENABLED:-true}"
  export CROSS_ORIGIN_ISOLATION="${CROSS_ORIGIN_ISOLATION:-true}"
  export CAPTCHA_PROVIDER="${CAPTCHA_PROVIDER:-none}"
}

case "${1:-help}" in
  deps-up)
    prepare_data
    exec docker compose -p summerain-dev -f "$compose_file" up -d --wait
    ;;
  deps-down)
    exec docker compose -p summerain-dev -f "$compose_file" down
    ;;
  deps-status)
    exec docker compose -p summerain-dev -f "$compose_file" ps
    ;;
  backend)
    export_backend_env
    cd "${repo_root}/backend"
    exec go run ./cmd/server
    ;;
  frontend)
    prepare_cert
    export npm_config_cache="${npm_config_cache:-/tmp/summerain-npm-cache}"
    export VITE_DEV_BACKEND_URL="${VITE_DEV_BACKEND_URL:-http://127.0.0.1:${SUMMERAIN_DEV_BACKEND_PORT:-18080}}"
    cd "${frontend_dir}"
    exec npm run dev -- --host 127.0.0.1 --port "${SUMMERAIN_DEV_FRONTEND_PORT:-5173}"
    ;;
  *)
    echo "usage: $0 {deps-up|deps-down|deps-status|backend|frontend}" >&2
    exit 2
    ;;
esac

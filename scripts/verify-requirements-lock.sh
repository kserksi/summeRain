#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
lock_file="${repo_root}/requirements.lock"

fail() {
  echo "requirements.lock verification failed: $*" >&2
  exit 1
}

require_line() {
  local file="$1"
  local expected="$2"
  grep -Fqx "$expected" "$file" || fail "${file#"${repo_root}/"} is missing: ${expected}"
}

require_text() {
  local file="$1"
  local expected="$2"
  grep -Fq "$expected" "$file" || fail "${file#"${repo_root}/"} is missing: ${expected}"
}

require_single_text() {
  local file="$1"
  local expected="$2"
  local count
  count="$(grep -Fc "$expected" "$file" || true)"
  if [ "$count" -ne 1 ]; then
    fail "${file#"${repo_root}/"} must contain exactly one: ${expected}"
  fi
}

require_single_yaml_value() {
  local file="$1"
  local key="$2"
  local expected="$3"
  local all_count
  local expected_count
  all_count="$(grep -Ec "^[[:space:]]+${key}:" "$file" || true)"
  expected_count="$(grep -Ec "^[[:space:]]+${key}:[[:space:]]+\"${expected}\"[[:space:]]*$" "$file" || true)"
  if [ "$all_count" -ne 1 ] || [ "$expected_count" -ne 1 ]; then
    fail "${file#"${repo_root}/"} must define ${key}: \"${expected}\" exactly once"
  fi
}

command -v jq >/dev/null 2>&1 || fail "jq is required"
jq -e '.schema_version == 1' "$lock_file" >/dev/null || fail "unsupported schema_version"
jq -e '.policy.container_images | contains("OCI multi-platform index digest")' "$lock_file" >/dev/null || \
  fail "container image digest strategy must document OCI multi-platform index pinning"

go_version="$(jq -er '.toolchains.go.version' "$lock_file")"
node_version="$(jq -er '.toolchains.node.version' "$lock_file")"
mysql_image="$(jq -er '.services.mysql.image' "$lock_file")"
redis_image="$(jq -er '.services.redis.image' "$lock_file")"
imgproxy_image="$(jq -er '.services.imgproxy.image' "$lock_file")"
alpine_image="$(jq -er '.services.alpine.image' "$lock_file")"
go_lock="$(jq -er '.ecosystem_locks.go_modules.path' "$lock_file")"
npm_lock="$(jq -er '.ecosystem_locks.npm_packages.path' "$lock_file")"
pica_version="$(jq -er '.image_processing.pica.version' "$lock_file")"
wasm_vips_version="$(jq -er '.image_processing["wasm-vips"].version' "$lock_file")"

for image in "$mysql_image" "$redis_image" "$imgproxy_image" "$alpine_image"; do
  if [[ "$image" != *:* ]] || [[ "$image" == *:latest ]]; then
    fail "service images must use exact non-latest version tags: ${image}"
  fi
done

test -f "${repo_root}/${go_lock}" || fail "missing authoritative Go lock: ${go_lock}"
test -f "${repo_root}/${npm_lock}" || fail "missing authoritative npm lock: ${npm_lock}"

package_json="${repo_root}/frontend/package.json"
package_lock="${repo_root}/${npm_lock}"
require_text "$package_json" "\"pica\": \"${pica_version}\""
require_text "$package_json" "\"wasm-vips\": \"${wasm_vips_version}\""
test "$(jq -er '.packages["node_modules/pica"].version' "$package_lock")" = "$pica_version" || \
  fail "frontend/package-lock.json pica version differs from requirements.lock"
test "$(jq -er '.packages["node_modules/wasm-vips"].version' "$package_lock")" = "$wasm_vips_version" || \
  fail "frontend/package-lock.json wasm-vips version differs from requirements.lock"

require_line "${repo_root}/Dockerfile" "FROM --platform=\$BUILDPLATFORM node:${node_version}-alpine AS frontend-builder"
require_line "${repo_root}/Dockerfile" "FROM --platform=\$BUILDPLATFORM golang:${go_version}-alpine AS backend-builder"
require_line "${repo_root}/Dockerfile" "FROM ${alpine_image}"

for compose in \
  "${repo_root}/backend/docker-compose.yml" \
  "${repo_root}/backend/docker-compose.deploy.yml" \
  "${repo_root}/backend/docker-compose.dev-deps.yml"; do
  require_text "$compose" "image: ${mysql_image}"
  require_text "$compose" "image: ${redis_image}"
  require_text "$compose" "image: ${imgproxy_image}"
  require_single_text "$compose" 'IMGPROXY_WORKERS: ${IMGPROXY_WORKERS:-2}'
  require_single_yaml_value "$compose" "IMGPROXY_REQUESTS_QUEUE_SIZE" "4"
  require_text "$compose" '"--maxmemory", "128mb", "--maxmemory-policy", "noeviction"'
  if grep -Fq 'IMGPROXY_CONCURRENCY' "$compose"; then
    fail "${compose#"${repo_root}/"} uses removed imgproxy v4 concurrency setting"
  fi
done

for compose in \
  "${repo_root}/backend/docker-compose.yml" \
  "${repo_root}/backend/docker-compose.deploy.yml"; do
  require_text "$compose" "image: ${alpine_image}"
  require_text "$compose" "GOMEMLIMIT: 512MiB"
  require_text "$compose" "TEMP_PATH: /data/images/.staging"
  require_text "$compose" "image_storage:/data/images"
  require_text "$compose" "pids_limit:"
  require_text "$compose" "max-size: \"10m\""
done

env_example="${repo_root}/backend/.env.example"
for guardrail in \
  'CROSS_ORIGIN_ISOLATION=true' \
  'DB_MAX_OPEN_CONNS=8' \
  'DB_MAX_IDLE_CONNS=4' \
  'DB_CONN_MAX_LIFETIME=30m' \
  'REDIS_POOL_SIZE=8' \
  'IMGPROXY_WORKERS=2' \
  'DISK_SOFT_LIMIT_PERCENT=80' \
  'DISK_HARD_LIMIT_PERCENT=90'; do
  require_line "$env_example" "$guardrail"
done
require_line "$env_example" 'V2_WATERMARK_CONCURRENCY=2'
require_text "${repo_root}/scripts/dev-wsl.sh" 'CROSS_ORIGIN_ISOLATION:-true'

for compose in \
  "${repo_root}/backend/docker-compose.yml" \
  "${repo_root}/backend/docker-compose.deploy.yml" \
  "${repo_root}/backend/docker-compose.dev-deps.yml"; do
  if grep -Eq '^[[:space:]]+build:' "$compose"; then
    fail "${compose#"${repo_root}/"} must not build the application image"
  fi
done

default_compose="${repo_root}/backend/docker-compose.yml"
require_text "$default_compose" 'image: ${DOCKER_IMAGE:-jaykserks/summerain:edge}'
require_text "$default_compose" 'MYSQL_ROOT_PASSWORD: ${DB_PASSWORD:?Set DB_PASSWORD}'
require_text "$default_compose" 'IMGPROXY_KEY: ${IMGPROXY_KEY:?Set IMGPROXY_KEY}'
require_text "$default_compose" 'IMGPROXY_SALT: ${IMGPROXY_SALT:?Set IMGPROXY_SALT}'

deploy_compose="${repo_root}/backend/docker-compose.deploy.yml"
require_text "$deploy_compose" 'image: ${DOCKER_IMAGE:?Set DOCKER_IMAGE to an exact release tag or digest}'
require_text "$deploy_compose" 'MYSQL_ROOT_PASSWORD: ${DB_PASSWORD:?Set DB_PASSWORD}'
require_text "$deploy_compose" 'IMGPROXY_KEY: ${IMGPROXY_KEY:?Set IMGPROXY_KEY}'
require_text "$deploy_compose" 'IMGPROXY_SALT: ${IMGPROXY_SALT:?Set IMGPROXY_SALT}'
if grep -Eq 'image:[[:space:]]+[^#[:space:]]*:latest([[:space:]]|$)' "$deploy_compose"; then
  fail "production compose must not use latest tags"
fi
if grep -Fq 'temp_storage' "$deploy_compose"; then
  fail "production staging must share image_storage"
fi

ci="${repo_root}/.github/workflows/ci.yml"
require_text "$ci" "go-version: ${go_version}"
require_text "$ci" "node-version: ${node_version}"
require_text "$ci" "run: go mod verify"
require_text "$ci" "run: npm run lint"
require_text "$ci" "concurrency:"
require_text "$ci" "set-dockerhub-tag-policy.sh release"
require_text "$ci" "reconcile-release-images.sh"
require_text "$ci" "DOCKER_METADATA_SHORT_SHA_LENGTH: 12"
require_text "$ci" 'if [ "$version_changed" != "true" ]; then'
require_text "$ci" 'bash scripts/validate-release-version.sh "$version"'
require_text "$ci" 'version="$(< VERSION)"'
if grep -Fq "tr -d '[:space:]' < VERSION" "$ci"; then
  fail "release workflow must preserve malformed whitespace for strict validation"
fi
if grep -Eq 'mode=mutable|set-dockerhub-tag-policy\.sh[[:space:]]+mutable' "$ci"; then
  fail "release workflow must never disable Docker Hub tag immutability"
fi

bash "${repo_root}/.github/scripts/test-reconcile-release-images.sh"

while IFS= read -r action_ref; do
  if [[ "$action_ref" == ./* ]]; then
    continue
  fi
  revision="${action_ref##*@}"
  if ! [[ "$revision" =~ ^[0-9a-f]{40}$ ]]; then
    fail "GitHub Action must be pinned to a full commit SHA: ${action_ref}"
  fi
done < <(sed -nE 's/^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*([^[:space:]#]+).*/\2/p' "$ci")
require_text "$ci" "peter-evans/dockerhub-description@"

tag_policy="${repo_root}/.github/scripts/set-dockerhub-tag-policy.sh"
require_text "$tag_policy" 'immutable_tags: true'
require_text "$tag_policy" 'immutable_tags_rules: [$rule]'
if grep -Fq '"immutable_tags":false' "$tag_policy"; then
  fail "tag policy helper must not expose a mutable mode"
fi

version_validator="${repo_root}/scripts/validate-release-version.sh"
for version in 0.0.0 1.2.3 1.2.3-0 1.2.3-rc.1 10.20.30-alpha-beta.7; do
  bash "$version_validator" "$version" || fail "valid release version rejected: ${version}"
done
for version in 01.2.3 1.02.3 1.2.03 1.2 1.2.3-01 1.2.3-alpha..1 1.2.3+build.1 v1.2.3; do
  if bash "$version_validator" "$version"; then
    fail "invalid release version accepted: ${version}"
  fi
done
max_length_version="1.2.3-$(printf 'a%.0s' {1..121})"
bash "$version_validator" "$max_length_version" || fail "maximum-length Docker-compatible release version rejected"
too_long_version="1.2.3-$(printf 'a%.0s' {1..122})"
if bash "$version_validator" "$too_long_version"; then
  fail "release version exceeds the Docker tag length limit"
fi

tag_rule="$(bash "$tag_policy" print-rule)"
for tag in 0.0.0 v1.2.3 1.2.3-0 v1.2.3-rc.1; do
  [[ "$tag" =~ $tag_rule ]] || fail "valid immutable image tag rejected: ${tag}"
done
for tag in 01.2.3 v1.02.3 1.2.03 1.2.3-01 v1.2.3-alpha..1 latest edge; do
  if [[ "$tag" =~ $tag_rule ]]; then
    fail "invalid immutable image tag accepted: ${tag}"
  fi
done

require_line "${repo_root}/.gitignore" "docs/incident-2026-07-14.md"

echo "requirements.lock verification passed"

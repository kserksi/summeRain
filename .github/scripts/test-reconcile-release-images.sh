#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
subject="${script_dir}/reconcile-release-images.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

mock_bin="${test_root}/bin"
mkdir -p "$mock_bin"
cat >"${mock_bin}/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail

ref_key() {
  printf '%s' "$1" | sha256sum | awk '{print $1}'
}

if [[ "${1:-}" == buildx && "${2:-}" == imagetools && "${3:-}" == inspect && "${4:-}" == --format && "${5:-}" == '{{json .Manifest}}' && $# -eq 6 ]]; then
  ref="$6"
  key="$(ref_key "$ref")"
  if [[ ! -f "${MOCK_DOCKER_STATE}/refs/${key}" ]]; then
    echo "ERROR: ${ref}: not found" >&2
    exit 1
  fi
  digest="$(< "${MOCK_DOCKER_STATE}/refs/${key}")"
  printf '{"digest":"sha256:%s"}\n' "$digest"
  exit 0
fi

if [[ "${1:-}" == buildx && "${2:-}" == imagetools && "${3:-}" == create && "${4:-}" == --tag && $# -eq 6 ]]; then
  target="$5"
  source="$6"
  digest="${source##*@}"
  digest="${digest#sha256:}"
  [[ -f "${MOCK_DOCKER_STATE}/blobs/${digest}" ]] || { echo "missing source ${source}" >&2; exit 1; }
  printf '%s\n' "$digest" >"${MOCK_DOCKER_STATE}/refs/$(ref_key "$target")"
  printf 'create %s %s\n' "$target" "$source" >>"${MOCK_DOCKER_STATE}/commands"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 2
MOCK
chmod +x "${mock_bin}/docker"
export PATH="${mock_bin}:${PATH}"

new_state() {
  MOCK_DOCKER_STATE="${test_root}/state-$1"
  export MOCK_DOCKER_STATE
  mkdir -p "${MOCK_DOCKER_STATE}/refs" "${MOCK_DOCKER_STATE}/blobs"
  : >"${MOCK_DOCKER_STATE}/commands"
}

ref_key() {
  printf '%s' "$1" | sha256sum | awk '{print $1}'
}

add_manifest() {
  local ref="$1"
  local body="$2"
  local digest
  digest="$(printf '%s' "$body" | sha256sum | awk '{print $1}')"
  printf '%s' "$body" >"${MOCK_DOCKER_STATE}/blobs/${digest}"
  printf '%s\n' "$digest" >"${MOCK_DOCKER_STATE}/refs/$(ref_key "$ref")"
}

digest_for() {
  local ref="$1"
  local digest
  digest="$(< "${MOCK_DOCKER_STATE}/refs/$(ref_key "$ref")")"
  printf 'sha256:%s\n' "$digest"
}

assert_digest() {
  local ref="$1"
  local expected="$2"
  local actual
  actual="$(digest_for "$ref")"
  [[ "$actual" == "$expected" ]] || { echo "${ref}: got ${actual}, want ${expected}" >&2; exit 1; }
}

dh='docker.io/jaykserks/summerain'
gh='ghcr.io/kserksi/summerain'
sha='abcdef1234567890abcdef1234567890abcdef12'

# Both registries absent: recovery requests one normal multi-registry build.
new_state absent
result="$(bash "$subject" recover "$dh" "$gh" 1.2.3 false main "$sha")"
[[ "$result" == build ]]
[[ ! -s "${MOCK_DOCKER_STATE}/commands" ]]

# One exact tag is enough to restore the other exact tag, copy to the other
# registry, and recreate all moving aliases without touching the existing tag.
new_state partial
add_manifest "${dh}:v1.2.3" '{"release":"one"}'
release_digest="$(digest_for "${dh}:v1.2.3")"
result="$(bash "$subject" recover "$dh" "$gh" 1.2.3 false main "$sha")"
[[ "$result" == recovered ]]
for image in "$dh" "$gh"; do
  for tag in v1.2.3 1.2.3 1.2 1 latest main main-sha-abcdef123456; do
    assert_digest "${image}:${tag}" "$release_digest"
  done
done
if grep -Fq "create ${dh}:v1.2.3 " "${MOCK_DOCKER_STATE}/commands"; then
  echo "existing immutable tag was pushed again" >&2
  exit 1
fi

# Exact tags that disagree within or across registries are never overwritten.
new_state mismatch
add_manifest "${dh}:v1.2.3" '{"release":"one"}'
add_manifest "${gh}:1.2.3" '{"release":"two"}'
if bash "$subject" recover "$dh" "$gh" 1.2.3 false main "$sha" >/dev/null 2>&1; then
  echo "mismatched exact digests were accepted" >&2
  exit 1
fi
[[ ! -s "${MOCK_DOCKER_STATE}/commands" ]]

# Complete exact tags still repair absent or stale moving aliases.
new_state alias-repair
for ref in "${dh}:v1.2.3" "${dh}:1.2.3" "${gh}:v1.2.3" "${gh}:1.2.3"; do
  add_manifest "$ref" '{"release":"one"}'
done
add_manifest "${dh}:latest" '{"release":"old"}'
release_digest="$(digest_for "${dh}:v1.2.3")"
result="$(bash "$subject" recover "$dh" "$gh" 1.2.3 false main "$sha")"
[[ "$result" == recovered ]]
for image in "$dh" "$gh"; do
  for tag in 1.2 1 latest main main-sha-abcdef123456; do
    assert_digest "${image}:${tag}" "$release_digest"
  done
done
if grep -Eq 'create .+:(v1\.2\.3|1\.2\.3) ' "${MOCK_DOCKER_STATE}/commands"; then
  echo "complete immutable tags were pushed again" >&2
  exit 1
fi

# Development releases use an isolated namespace and never move stable aliases.
new_state dev-release
add_manifest "${dh}:dev-v1.2.4" '{"release":"dev"}'
release_digest="$(digest_for "${dh}:dev-v1.2.4")"
result="$(bash "$subject" recover "$dh" "$gh" 1.2.4 true dev "$sha")"
[[ "$result" == recovered ]]
for image in "$dh" "$gh"; do
  for tag in dev-v1.2.4 dev-1.2.4 dev dev-sha-abcdef123456; do
    assert_digest "${image}:${tag}" "$release_digest"
  done
  for tag in v1.2.4 1.2.4 latest main; do
    key="$(ref_key "${image}:${tag}")"
    [[ ! -f "${MOCK_DOCKER_STATE}/refs/${key}" ]] || {
      echo "development release unexpectedly published ${image}:${tag}" >&2
      exit 1
    }
  done
done

echo "release image reconciliation tests passed"

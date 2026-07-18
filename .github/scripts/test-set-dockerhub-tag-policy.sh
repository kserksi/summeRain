#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
subject="${script_dir}/set-dockerhub-tag-policy.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

mock_bin="${test_root}/bin"
mkdir -p "$mock_bin"
cat >"${mock_bin}/curl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\0' "$@" >>"${MOCK_CURL_CALLS}"
printf '\n' >>"${MOCK_CURL_CALLS}"

url="${*: -1}"
case "$url" in
  https://hub.docker.com/v2/auth/token)
    printf '{"access_token":"test-token"}\n'
    ;;
  https://hub.docker.com/v2/namespaces/jaykserks/repositories/summerain/immutabletags)
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == --data ]]; then
        printf '%s\n' "$2" >"${MOCK_POLICY_PAYLOAD}"
        exit 0
      fi
      shift
    done
    echo "missing PATCH payload" >&2
    exit 2
    ;;
  *)
    echo "unexpected URL: ${url}" >&2
    exit 2
    ;;
esac
MOCK
chmod +x "${mock_bin}/curl"

export PATH="${mock_bin}:${PATH}"
export MOCK_CURL_CALLS="${test_root}/curl-calls"
export MOCK_POLICY_PAYLOAD="${test_root}/policy.json"
export DOCKERHUB_USERNAME=test-user
export DOCKERHUB_TOKEN=test-token

bash "$subject" release

tag_rule="$(bash "$subject" print-rule)"
test "$(jq -er '.immutable_tags' "$MOCK_POLICY_PAYLOAD")" = true
test "$(jq -er '.immutable_tags_rules | length' "$MOCK_POLICY_PAYLOAD")" = 1
test "$(jq -er '.immutable_tags_rules[0]' "$MOCK_POLICY_PAYLOAD")" = "$tag_rule"

for tag in 0.0.0 v1.2.3 1.2.3-0 v1.2.3-alpha- 1.2.3--; do
  [[ "$tag" =~ $tag_rule ]] || { echo "release tag not protected: ${tag}" >&2; exit 1; }
done
for tag in latest edge sha-abcdef123456 1.2 1; do
  if [[ "$tag" =~ $tag_rule ]]; then
    echo "moving tag unexpectedly protected: ${tag}" >&2
    exit 1
  fi
done

test "$(tr '\0' '\n' <"$MOCK_CURL_CALLS" | grep -Fxc -- '--retry')" -eq 2
test "$(tr '\0' '\n' <"$MOCK_CURL_CALLS" | grep -Fxc -- '5')" -ge 2
test "$(tr '\0' '\n' <"$MOCK_CURL_CALLS" | grep -Fxc -- '--retry-max-time')" -eq 2

echo "Docker Hub tag policy tests passed"

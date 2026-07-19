#!/usr/bin/env bash
set -euo pipefail

# Keep the registry rule deliberately simple: Docker Hub returned HTTP 500 for
# the nested strict-SemVer expression. validate-release-version.sh enforces
# strict SemVer; this compatible superset covers every generated exact tag.
tag_rule='^(dev-)?v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

if [ "${1:-}" = "print-rule" ]; then
  printf '%s\n' "$tag_rule"
  exit 0
fi
if [ "${1:-}" != "release" ]; then
  echo "usage: $0 release|print-rule" >&2
  exit 2
fi

policy="$(jq -nc --arg rule "$tag_rule" '{immutable_tags: true, immutable_tags_rules: [$rule]}')"

auth_payload="$(jq -nc \
  --arg identifier "$DOCKERHUB_USERNAME" \
  --arg secret "$DOCKERHUB_TOKEN" \
  '{identifier: $identifier, secret: $secret}')"
auth_response="$(curl --fail-with-body --silent --show-error \
  --retry 5 \
  --retry-delay 2 \
  --retry-max-time 30 \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "$auth_payload" \
  https://hub.docker.com/v2/auth/token)"
hub_token="$(jq -er '.access_token' <<< "$auth_response")"

curl --fail-with-body --silent --show-error \
  --retry 5 \
  --retry-delay 2 \
  --retry-max-time 30 \
  --request PATCH \
  --header "Authorization: Bearer ${hub_token}" \
  --header 'Content-Type: application/json' \
  --data "$policy" \
  https://hub.docker.com/v2/namespaces/jaykserks/repositories/summerain/immutabletags

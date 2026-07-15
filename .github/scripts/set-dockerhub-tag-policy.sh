#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
case "$mode" in
  mutable)
    policy='{"immutable_tags":false,"immutable_tags_rules":[]}'
    ;;
  release)
    policy='{"immutable_tags":true,"immutable_tags_rules":["^v?[0-9]+\\.[0-9]+\\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$"]}'
    ;;
  *)
    echo "usage: $0 {mutable|release}" >&2
    exit 2
    ;;
esac

auth_payload="$(jq -nc \
  --arg identifier "$DOCKERHUB_USERNAME" \
  --arg secret "$DOCKERHUB_TOKEN" \
  '{identifier: $identifier, secret: $secret}')"
auth_response="$(curl --fail-with-body --silent --show-error \
  --request POST \
  --header 'Content-Type: application/json' \
  --data "$auth_payload" \
  https://hub.docker.com/v2/auth/token)"
hub_token="$(jq -er '.access_token' <<< "$auth_response")"

curl --fail-with-body --silent --show-error \
  --request PATCH \
  --header "Authorization: Bearer ${hub_token}" \
  --header 'Content-Type: application/json' \
  --data "$policy" \
  https://hub.docker.com/v2/namespaces/jaykserks/repositories/summerain/immutabletags

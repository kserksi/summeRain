#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 fresh|recover|verify <dockerhub-image> <ghcr-image> <version> <prerelease> <channel> <commit-sha>" >&2
  exit 2
}

[[ $# -eq 7 ]] || usage

mode="$1"
dockerhub_image="$2"
ghcr_image="$3"
version="$4"
prerelease="$5"
channel="$6"
commit_sha="$7"

case "$mode" in
  fresh | recover | verify) ;;
  *) usage ;;
esac

if [[ "$prerelease" != "true" && "$prerelease" != "false" ]]; then
  echo "prerelease must be true or false" >&2
  exit 2
fi
if [[ "$channel" != "dev" && "$channel" != "main" ]]; then
  echo "channel must be dev or main" >&2
  exit 2
fi
if ! [[ "$commit_sha" =~ ^[0-9a-fA-F]{12,64}$ ]]; then
  echo "commit SHA must contain at least 12 hexadecimal characters" >&2
  exit 2
fi

if [[ "$channel" == "dev" ]]; then
  exact_tags=("dev-v${version}" "dev-${version}")
  alias_tags=("dev-sha-${commit_sha:0:12}" dev)
else
  exact_tags=("v${version}" "${version}")
  alias_tags=("main-sha-${commit_sha:0:12}" main)
fi
if [[ "$channel" == "main" && "$prerelease" == "false" ]]; then
  core_version="${version%%-*}"
  IFS=. read -r major minor patch <<< "$core_version"
  if [[ -z "${major:-}" || -z "${minor:-}" || -z "${patch:-}" ]]; then
    echo "stable version must contain major, minor, and patch components" >&2
    exit 2
  fi
  alias_tags+=("${major}.${minor}" "${major}" latest)
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
inspect_counter=0

# Read the registry descriptor digest reported by buildx. A missing tag returns
# 3; registry, authentication, parsing, and transport failures remain fatal so
# they cannot be mistaken for an unpublished immutable tag.
inspect_digest() {
  local ref="$1"
  local manifest_file error_file error_text digest

  inspect_counter=$((inspect_counter + 1))
  manifest_file="${tmp_dir}/manifest-${inspect_counter}.json"
  error_file="${tmp_dir}/manifest-${inspect_counter}.err"
  if docker buildx imagetools inspect --format '{{json .Manifest}}' "$ref" >"$manifest_file" 2>"$error_file"; then
    if digest="$(jq -er '.digest | strings | select(test("^sha256:[0-9a-f]{64}$"))' "$manifest_file")"; then
      printf '%s\n' "$digest"
      return 0
    fi
    echo "failed to read a descriptor digest for ${ref}" >&2
    return 1
  fi

  error_text="$(< "$error_file")"
  if grep -Eqi '(manifest unknown|name unknown|(^|: )[Nn]ot [Ff]ound([[:space:]]|$))' "$error_file"; then
    return 3
  fi

  echo "failed to inspect ${ref}: ${error_text}" >&2
  return 1
}

read_tag() {
  local image="$1"
  local tag="$2"
  local ref="${image}:${tag}"
  local digest status

  if digest="$(inspect_digest "$ref")"; then
    printf '%s\n' "$digest"
    return 0
  else
    status=$?
  fi
  if [[ $status -eq 3 ]]; then
    printf '%s\n' missing
    return 0
  fi
  return "$status"
}

tag_source() {
  local target_ref="$1"
  local source_ref="$2"
  local expected_digest="$3"
  local immutable="$4"
  local current_digest

  current_digest="$(read_tag "${target_ref%:*}" "${target_ref##*:}")"
  if [[ "$current_digest" == "$expected_digest" ]]; then
    return 0
  fi
  if [[ "$immutable" == "true" && "$current_digest" != missing ]]; then
    echo "immutable release tag ${target_ref} has digest ${current_digest}, expected ${expected_digest}" >&2
    return 1
  fi

  if [[ "$current_digest" == missing ]]; then
    echo "::notice::Restoring ${target_ref} at ${expected_digest}" >&2
  else
    echo "::notice::Moving ${target_ref} from ${current_digest} to ${expected_digest}" >&2
  fi
  docker buildx imagetools create --tag "$target_ref" "$source_ref"

  current_digest="$(read_tag "${target_ref%:*}" "${target_ref##*:}")"
  if [[ "$current_digest" != "$expected_digest" ]]; then
    echo "tag reconciliation produced ${current_digest} for ${target_ref}, expected ${expected_digest}" >&2
    return 1
  fi
}

images=("$dockerhub_image" "$ghcr_image")
source_ref=""
release_digest=""
found_exact=false

for image in "${images[@]}"; do
  for tag in "${exact_tags[@]}"; do
    digest="$(read_tag "$image" "$tag")"
    if [[ "$digest" == missing ]]; then
      continue
    fi
    found_exact=true
    if [[ -z "$release_digest" ]]; then
      release_digest="$digest"
      source_ref="${image}@${digest}"
    elif [[ "$digest" != "$release_digest" ]]; then
      echo "release digest mismatch: ${image}:${tag} is ${digest}, expected ${release_digest}" >&2
      exit 1
    fi
  done
done

if [[ "$mode" == fresh ]]; then
  if [[ "$found_exact" == true ]]; then
    echo "an exact release tag already exists during a first-attempt publish; refusing to overwrite it" >&2
    exit 1
  fi
  echo build
  exit 0
fi

if [[ "$found_exact" == false ]]; then
  if [[ "$mode" == verify ]]; then
    echo "release publish completed without either exact tag in either registry" >&2
    exit 1
  fi
  echo build
  exit 0
fi

for image in "${images[@]}"; do
  for tag in "${exact_tags[@]}"; do
    tag_source "${image}:${tag}" "$source_ref" "$release_digest" true
  done
  for tag in "${alias_tags[@]}"; do
    tag_source "${image}:${tag}" "$source_ref" "$release_digest" false
  done
done

# Re-read every exact tag after recovery. This catches incomplete cross-registry
# copies and eventual registry errors before the GitHub Release is created.
for image in "${images[@]}"; do
  for tag in "${exact_tags[@]}"; do
    digest="$(read_tag "$image" "$tag")"
    if [[ "$digest" != "$release_digest" ]]; then
      echo "release verification failed: ${image}:${tag} is ${digest}, expected ${release_digest}" >&2
      exit 1
    fi
  done
done

echo recovered

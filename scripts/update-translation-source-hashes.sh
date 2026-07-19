#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
summary="${repo_root}/SUMMARY.md"
locale_keys=("zh-CN" "ja-JP")
locale_roots=("${repo_root}/translations/zh-CN" "${repo_root}/translations/ja-JP")
temporary_manifests=()

fail() {
  echo "Translation source hash update failed: $*" >&2
  exit 1
}

cleanup() {
  if (( ${#temporary_manifests[@]} > 0 )); then
    rm -f "${temporary_manifests[@]}"
  fi
}
trap cleanup EXIT

for required_command in sed sha256sum; do
  command -v "$required_command" >/dev/null || fail "required command is unavailable: $required_command"
done

[[ -r "$summary" ]] || fail "SUMMARY.md is missing or unreadable"
navigation="$({
  sed -nE 's/^[[:space:]]*\*[[:space:]]+\[[^]]+\]\(([^#[:space:])]+)(#[^[:space:])]+)?([[:space:]]+"[^"]*")?\)[[:space:]]*$/\1/p' "$summary" |
    sed 's#^\./##'
})"
[[ -n "$navigation" ]] || fail "SUMMARY.md contains no parseable page links"
mapfile -t canonical_paths < <(printf '%s\n' "$navigation")

declare -A seen_paths=()
for path in "${canonical_paths[@]}"; do
  [[ -n "$path" ]] || fail "SUMMARY.md contains an empty page path"
  [[ -z "${seen_paths[$path]:-}" ]] || fail "SUMMARY.md contains a duplicate page path: $path"
  [[ -r "${repo_root}/${path}" && -s "${repo_root}/${path}" ]] ||
    fail "canonical page is missing, unreadable, or empty: $path"
  seen_paths["$path"]=1
done

for locale_root in "${locale_roots[@]}"; do
  [[ -d "$locale_root" ]] || fail "translation directory is missing: ${locale_root#${repo_root}/}"
  for path in "${canonical_paths[@]}"; do
    [[ -r "${locale_root}/${path}" && -s "${locale_root}/${path}" ]] ||
      fail "translation is missing, unreadable, or empty: ${locale_root#${repo_root}/}/${path}"
  done
done

for index in "${!locale_roots[@]}"; do
  locale_key="${locale_keys[$index]}"
  locale_root="${locale_roots[$index]}"
  manifest="${locale_root}/SOURCE_HASHES"
  tmp="$(mktemp "${locale_root}/.SOURCE_HASHES.XXXXXX")"
  temporary_manifests+=("$tmp")
  declare -A old_source_hashes=()
  declare -A old_translation_hashes=()

  if [[ -f "$manifest" ]]; then
    while read -r old_source old_translation old_path extra; do
      [[ -z "${extra:-}" && "$old_source" =~ ^[0-9a-f]{64}$ && "$old_translation" =~ ^[0-9a-f]{64}$ && -n "${old_path:-}" ]] ||
        continue
      old_source_hashes["$old_path"]="$old_source"
      old_translation_hashes["$old_path"]="$old_translation"
    done < "$manifest"
  fi

  for path in "${canonical_paths[@]}"; do
    source_hash="$(sha256sum "${repo_root}/${path}" | cut -d' ' -f1)"
    translation_hash="$(sha256sum "${locale_root}/${path}" | cut -d' ' -f1)"
    if [[ -n "${old_source_hashes[$path]:-}" &&
          "${old_source_hashes[$path]}" != "$source_hash" &&
          "${old_translation_hashes[$path]}" == "$translation_hash" ]]; then
      fail "${locale_key} translation was not updated after its English source changed: $path"
    fi
    printf '%s  %s  %s\n' "$source_hash" "$translation_hash" "$path" >> "$tmp"
  done
done

for index in "${!locale_roots[@]}"; do
  mv "${temporary_manifests[$index]}" "${locale_roots[$index]}/SOURCE_HASHES"
done
temporary_manifests=()

echo "Translation source and locale hashes updated (${#canonical_paths[@]} pages per locale)"

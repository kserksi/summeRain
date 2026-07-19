#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
incident="docs/incident-2026-07-14.md"
public_site="https://summerain-1.gitbook.io/summerain/"
expected_config=$'root: ./\n\nstructure:\n  readme: README.md\n  summary: SUMMARY.md'

locale_names=("English" "Simplified Chinese" "Japanese")
locale_keys=("en" "zh-CN" "ja-JP")
locale_roots=("${repo_root}" "${repo_root}/translations/zh-CN" "${repo_root}/translations/ja-JP")
minimum_locale_script_characters=100

fail() {
  echo "GitBook documentation verification failed: $*" >&2
  exit 1
}

extract_navigation_paths() {
  sed -nE 's/^[[:space:]]*\*[[:space:]]+\[[^]]+\]\(([^#[:space:])]+)(#[^[:space:])]+)?([[:space:]]+"[^"]*")?\)[[:space:]]*$/\1/p' "$1"
}

extract_navigation_records() {
  sed -nE 's/^([[:space:]]*)\*[[:space:]]+\[[^]]+\]\(([^#[:space:])]+)(#[^[:space:])]+)?([[:space:]]+"[^"]*")?\)[[:space:]]*$/\1\2/p' "$1"
}

extract_summary_structure() {
  local summary="$1"
  local line
  local record
  local leading
  local path

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^(#{1,6})[[:space:]] ]]; then
      printf 'H:%s\n' "${#BASH_REMATCH[1]}"
      continue
    fi
    record="$(printf '%s\n' "$line" | sed -nE 's/^([[:space:]]*)\*[[:space:]]+\[[^]]+\]\(([^#[:space:])]+)(#[^[:space:])]+)?([[:space:]]+"[^"]*")?\)[[:space:]]*$/\1\2/p')"
    [[ -n "$record" ]] || continue
    leading="${record%%[! ]*}"
    path="${record#"$leading"}"
    path="${path#./}"
    printf 'P:%s:%s\n' "$(( ${#leading} / 2 ))" "$path"
  done < "$summary"
}

count_cjk_characters() {
  perl -CSD -ne '$n += () = /[\x{3040}-\x{30ff}\x{3400}-\x{9fff}\x{f900}-\x{faff}\x{ff66}-\x{ff9f}]/g; END { print $n + 0 }' "$1"
}

count_han_characters() {
  perl -CSD -ne '$n += () = /[\x{3400}-\x{9fff}\x{f900}-\x{faff}]/g; END { print $n + 0 }' "$1"
}

count_kana_characters() {
  perl -CSD -ne '$n += () = /[\x{3040}-\x{30ff}\x{ff66}-\x{ff9f}]/g; END { print $n + 0 }' "$1"
}

validate_config() {
  local locale_name="$1"
  local config="$2"
  [[ -f "$config" ]] || fail "${locale_name} .gitbook.yaml is missing"
  [[ "$(< "$config")" == "$expected_config" ]] ||
    fail "${locale_name} .gitbook.yaml must contain only the supported README/SUMMARY configuration"
}

validate_summary() {
  local locale_name="$1"
  local locale_root="$2"
  local summary="${locale_root}/SUMMARY.md"
  local entry_count
  local depth
  local leading
  local path
  local previous_depth=-1
  local record
  local -a paths=()
  local -a records=()
  local -A seen=()

  [[ -f "$summary" ]] || fail "${locale_name} SUMMARY.md is missing"
  if grep -q $'\t' "$summary"; then
    fail "${locale_name} SUMMARY.md must use spaces, not tabs"
  fi
  mapfile -t paths < <(extract_navigation_paths "$summary")
  mapfile -t records < <(extract_navigation_records "$summary")
  entry_count="$(grep -Ec '^[[:space:]]*\*[[:space:]]+\[' "$summary" || true)"
  [[ "$entry_count" -eq "${#paths[@]}" ]] ||
    fail "every ${locale_name} SUMMARY.md entry must be a simple local Markdown link"

  for path in "${paths[@]}"; do
    path="${path#./}"
    [[ "$path" != /* ]] || fail "${locale_name} navigation path must be relative: $path"
    [[ "$path" != *://* ]] || fail "external links do not belong in ${locale_name} SUMMARY.md: $path"
    [[ "$path" != '..' && "$path" != ../* && "$path" != */../* && "$path" != */.. ]] ||
      fail "${locale_name} navigation path escapes its locale root: $path"
    [[ "$path" == *.md ]] || fail "${locale_name} navigation target must be Markdown: $path"
    [[ -z "${seen[$path]:-}" ]] || fail "${locale_name} navigation path appears more than once: $path"
    [[ -f "${locale_root}/${path}" ]] || fail "${locale_name} navigation target does not exist: $path"
    seen["$path"]=1
  done

  for record in "${records[@]}"; do
    leading="${record%%[! ]*}"
    (( ${#leading} % 2 == 0 )) ||
      fail "${locale_name} SUMMARY.md indentation must use two-space levels"
    depth="$(( ${#leading} / 2 ))"
    if (( previous_depth < 0 && depth != 0 )); then
      fail "the first ${locale_name} navigation page must be top-level"
    fi
    if (( previous_depth >= 0 && depth > previous_depth + 1 )); then
      fail "${locale_name} SUMMARY.md contains an invalid navigation depth jump"
    fi
    previous_depth="$depth"
  done
}

compare_path_sets() {
  local locale_name="$1"
  local expected_file="$2"
  local actual_file="$3"
  local missing
  local extra

  missing="$(comm -23 "$expected_file" "$actual_file")"
  extra="$(comm -13 "$expected_file" "$actual_file")"
  [[ -z "$missing" ]] || fail "${locale_name} pages are missing: ${missing//$'\n'/, }"
  [[ -z "$extra" ]] || fail "${locale_name} has extra Markdown pages: ${extra//$'\n'/, }"
}

check_local_links() {
  local locale_name="$1"
  local locale_root="$2"
  shift 2
  local path
  local candidate
  local locale_real
  local resolved
  local target

  locale_real="$(realpath -e "$locale_root")"

  for path in "$@"; do
    while IFS= read -r target; do
      target="${target#](}"
      target="${target%)}"
      case "$target" in
        ''|'#'*|http://*|https://*|mailto:*) continue ;;
      esac
      [[ "$target" != *'incident-2026-07-14.md'* ]] ||
        fail "private incident material must not be linked from ${locale_name} ${path}"
      target="${target%%#*}"
      target="${target%%\?*}"
      target="${target%% *}"
      [[ -n "$target" ]] || continue
      candidate="$(realpath -m "${locale_root}/$(dirname "$path")/${target}")"
      [[ "$candidate" == "$locale_real" || "$candidate" == "$locale_real/"* ]] ||
        fail "local Markdown link escapes the ${locale_name} project root in ${path}: ${target}"
      [[ -e "$candidate" ]] ||
        fail "broken local Markdown link in ${locale_name} ${path}: ${target}"
      resolved="$(realpath -e "$candidate")"
      [[ "$resolved" == "$locale_real" || "$resolved" == "$locale_real/"* ]] ||
        fail "local Markdown symlink escapes the ${locale_name} project root in ${path}: ${target}"
    done < <(grep -oE '\]\([^)]*\)' "${locale_root}/${path}" || true)
  done
}

expected_hashes() {
  local locale_root="$1"
  local path
  local source_hash
  local translation_hash
  for path in "${canonical_paths[@]}"; do
    source_hash="$(sha256sum "${repo_root}/${path}" | cut -d' ' -f1)"
    translation_hash="$(sha256sum "${locale_root}/${path}" | cut -d' ' -f1)"
    printf '%s  %s  %s\n' "$source_hash" "$translation_hash" "$path"
  done
}

for required_command in git grep perl realpath sed sha256sum; do
  command -v "$required_command" >/dev/null || fail "required command is unavailable: $required_command"
done

for index in "${!locale_names[@]}"; do
  validate_config "${locale_names[$index]}" "${locale_roots[$index]}/.gitbook.yaml"
  validate_summary "${locale_names[$index]}" "${locale_roots[$index]}"
done

if LC_ALL=C grep -n '[^ -~]' "${repo_root}/README.md" >/dev/null; then
  fail "the canonical README.md must remain ASCII English"
fi

grep -Fq "[Documentation](${public_site})" "${repo_root}/README.md" ||
  fail "README.md must link its primary Documentation entry to ${public_site}"
if grep -Fq '](./LICENSE)' "${repo_root}/README.md"; then
  fail "README.md must use an explicit GitHub LICENSE URL because GitBook rewrites the relative path"
fi
grep -Fq "[summerain-1.gitbook.io/summerain](${public_site})" \
  "${repo_root}/docs/GITBOOK.md" ||
  fail "docs/GITBOOK.md must identify ${public_site} as the canonical public site"

if git -C "$repo_root" ls-files --cached -- '*incident-2026-07-14.md' | grep -q .; then
  fail "incident-2026-07-14.md must never be tracked in any locale"
fi
grep -Fxq "$incident" "${repo_root}/.gitignore" || fail "$incident must remain ignored"
if find "${repo_root}/translations" -type f -name 'incident-2026-07-14.md' -print -quit | grep -q .; then
  fail "incident-2026-07-14.md must never be copied into a translation"
fi
for locale_root in "${locale_roots[@]}"; do
  if grep -Fq 'incident-2026-07-14.md' "${locale_root}/SUMMARY.md"; then
    fail "incident-2026-07-14.md must never appear in GitBook navigation"
  fi
done

unexpected_translation_markdown="$({
  git -C "$repo_root" ls-files --cached --others --exclude-standard -- translations |
    grep '\.md$' |
    grep -Ev '^translations/(zh-CN|ja-JP)/' || true
})"
[[ -z "$unexpected_translation_markdown" ]] ||
  fail "Markdown under translations/ must belong to zh-CN or ja-JP: ${unexpected_translation_markdown//$'\n'/, }"

mapfile -t canonical_paths < <(extract_navigation_paths "${repo_root}/SUMMARY.md" | sed 's#^\./##')
mapfile -t canonical_structure < <(extract_summary_structure "${repo_root}/SUMMARY.md")
[[ "${#canonical_paths[@]}" -gt 0 ]] || fail "the canonical SUMMARY.md contains no pages"

cleanup_files=()
cleanup() {
  if (( ${#cleanup_files[@]} > 0 )); then
    rm -f "${cleanup_files[@]}"
  fi
}
trap cleanup EXIT

canonical_expected="$(mktemp)"
cleanup_files+=("$canonical_expected")
canonical_actual="$(mktemp)"
cleanup_files+=("$canonical_actual")

printf '%s\n' "${canonical_paths[@]}" | sort > "$canonical_expected"
git -C "$repo_root" ls-files --cached --others --exclude-standard -- '*.md' |
  sed 's#^\./##' |
  grep -v '^SUMMARY\.md$' |
  grep -v '^translations/' |
  sort > "$canonical_actual"
compare_path_sets "English" "$canonical_expected" "$canonical_actual"

for path in "${canonical_paths[@]}"; do
  if (( $(count_cjk_characters "${repo_root}/${path}") > 0 )); then
    fail "canonical English page contains Chinese or Japanese text: $path"
  fi
done
check_local_links "English" "$repo_root" "${canonical_paths[@]}"

for index in 1 2; do
  locale_name="${locale_names[$index]}"
  locale_key="${locale_keys[$index]}"
  locale_root="${locale_roots[$index]}"
  translation_actual="$(mktemp)"
  cleanup_files+=("$translation_actual")

  git -C "$repo_root" ls-files --cached --others --exclude-standard -- "translations/${locale_key}" |
    grep '\.md$' |
    sed "s#^translations/${locale_key}/##" |
    grep -v '^SUMMARY\.md$' |
    sort > "$translation_actual"
  compare_path_sets "$locale_name" "$canonical_expected" "$translation_actual"

  mapfile -t locale_structure < <(extract_summary_structure "${locale_root}/SUMMARY.md")
  [[ "$(printf '%s\n' "${canonical_structure[@]}")" == "$(printf '%s\n' "${locale_structure[@]}")" ]] ||
    fail "${locale_name} SUMMARY.md must preserve canonical sections, page order, and hierarchy"

  for path in "${canonical_paths[@]}"; do
    if cmp -s "${repo_root}/${path}" "${locale_root}/${path}"; then
      fail "${locale_name} page is an untranslated English copy: $path"
    fi
    if [[ "$locale_key" == "zh-CN" ]]; then
      script_characters="$(count_han_characters "${locale_root}/${path}")"
      (( script_characters >= minimum_locale_script_characters )) ||
        fail "Simplified Chinese page has too little Han text (${script_characters} characters): $path"
    else
      script_characters="$(count_kana_characters "${locale_root}/${path}")"
      (( script_characters >= minimum_locale_script_characters )) ||
        fail "Japanese page has too little kana (${script_characters} characters): $path"
    fi
  done

  check_local_links "$locale_name" "$locale_root" "${canonical_paths[@]}"
  [[ -f "${locale_root}/SOURCE_HASHES" ]] || fail "${locale_name} SOURCE_HASHES is missing"
  if ! diff -u <(expected_hashes "$locale_root") "${locale_root}/SOURCE_HASHES" >/dev/null; then
    fail "${locale_name} translations are stale; translate changed pages and run scripts/update-translation-source-hashes.sh"
  fi

  echo "${locale_name} documentation verification passed (${#canonical_paths[@]} pages)"
done

echo "English documentation verification passed (${#canonical_paths[@]} pages)"
echo "GitBook multilingual documentation verification passed"

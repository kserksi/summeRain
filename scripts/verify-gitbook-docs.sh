#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
config="${repo_root}/.gitbook.yaml"
summary="${repo_root}/SUMMARY.md"
incident="docs/incident-2026-07-14.md"
public_site="https://summerain-1.gitbook.io/summerain/"

fail() {
  echo "GitBook documentation verification failed: $*" >&2
  exit 1
}

[[ -f "$config" ]] || fail ".gitbook.yaml is missing"
[[ -f "$summary" ]] || fail "SUMMARY.md is missing"

expected_config=$'root: ./\n\nstructure:\n  readme: README.md\n  summary: SUMMARY.md'
[[ "$(< "$config")" == "$expected_config" ]] ||
  fail ".gitbook.yaml must contain only the supported root README/SUMMARY configuration"

if LC_ALL=C grep -n '[^ -~]' "${repo_root}/README.md" >/dev/null; then
  fail "README.md must remain ASCII English"
fi

grep -Fq "[Documentation](${public_site})" "${repo_root}/README.md" ||
  fail "README.md must link its primary Documentation entry to ${public_site}"
grep -Fq "[summerain-1.gitbook.io/summerain](${public_site})" \
  "${repo_root}/docs/GITBOOK.md" ||
  fail "docs/GITBOOK.md must identify ${public_site} as the canonical public site"

if git -C "$repo_root" ls-files --error-unmatch "$incident" >/dev/null 2>&1; then
  fail "$incident must never be tracked"
fi
grep -Fxq "$incident" "${repo_root}/.gitignore" || fail "$incident must remain ignored"
if grep -Fq "$incident" "$summary"; then
  fail "$incident must never appear in GitBook navigation"
fi

mapfile -t navigation_paths < <(
  sed -nE 's/^[[:space:]]*\*[[:space:]]+\[[^]]+\]\(([^#[:space:])]+)(#[^[:space:])]+)?([[:space:]]+"[^"]*")?\)[[:space:]]*$/\1/p' "$summary"
)

navigation_entries="$(grep -Ec '^[[:space:]]*\*[[:space:]]+\[' "$summary" || true)"
[[ "$navigation_entries" -eq "${#navigation_paths[@]}" ]] ||
  fail "every SUMMARY.md navigation entry must be a simple Markdown file link"

declare -A seen=()
for path in "${navigation_paths[@]}"; do
  path="${path#./}"
  [[ "$path" != /* ]] || fail "navigation path must be relative: $path"
  [[ "$path" != *://* ]] || fail "external links do not belong in SUMMARY.md: $path"
  [[ -z "${seen[$path]:-}" ]] || fail "navigation path appears more than once: $path"
  [[ -f "${repo_root}/${path}" ]] || fail "navigation target does not exist: $path"
  seen["$path"]=1
done

mapfile -t expected_paths < <(
  git -C "$repo_root" ls-files --cached -- '*.md' |
    sed 's#^\./##' |
    grep -vx 'SUMMARY.md' |
    sort
)
mapfile -t actual_paths < <(printf '%s\n' "${navigation_paths[@]}" | sed 's#^\./##' | sort)

missing="$(comm -23 <(printf '%s\n' "${expected_paths[@]}") <(printf '%s\n' "${actual_paths[@]}"))"
extra="$(comm -13 <(printf '%s\n' "${expected_paths[@]}") <(printf '%s\n' "${actual_paths[@]}"))"

[[ -z "$missing" ]] || fail "tracked Markdown is missing from SUMMARY.md: ${missing//$'\n'/, }"
[[ -z "$extra" ]] || fail "SUMMARY.md references non-public or untracked Markdown: ${extra//$'\n'/, }"

while IFS= read -r file; do
  while IFS= read -r target; do
    target="${target#](}"
    target="${target%)}"
    case "$target" in
      ''|'#'*|http://*|https://*|mailto:*) continue ;;
    esac
    target="${target%%#*}"
    target="${target%% *}"
    [[ -e "${repo_root}/$(dirname "$file")/${target}" ]] ||
      fail "broken local Markdown link in ${file}: ${target}"
  done < <(grep -oE '\]\([^)]*\)' "${repo_root}/${file}" || true)
done < <(git -C "$repo_root" ls-files --cached -- '*.md')

echo "GitBook documentation verification passed (${#actual_paths[@]} pages)"

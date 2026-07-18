#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"

# Docker tags cannot represent SemVer build metadata (`+...`) without a lossy
# rewrite, so releases use the strict SemVer 2.0.0 core and pre-release forms.
core_identifier='(0|[1-9][0-9]*)'
prerelease_identifier='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
release_pattern="^${core_identifier}\\.${core_identifier}\\.${core_identifier}(-${prerelease_identifier}(\\.${prerelease_identifier})*)?$"

[[ -n "$version" && "$version" =~ $release_pattern ]] || exit 1

# The workflow publishes both <version> and v<version>. Docker tag names are
# limited to 128 ASCII characters, so reserve one character for the v prefix.
(( ${#version} <= 127 ))

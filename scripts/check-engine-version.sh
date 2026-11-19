#!/usr/bin/env bash
# Verify the integrated version independently of the bundled upstream release.
set -euo pipefail

expected=${1:?expected net-merge version required}
output=${2:-}
read -r name actual _ <<< "$output"
if [[ "$name" != net-merge || "${actual#v}" != "${expected#v}" ]]; then
  printf "::error::Engine version mismatch: binary reports '%s', expected net-merge %s\n" "$output" "$expected" >&2
  exit 1
fi
printf 'Verified integrated engine: %s\n' "$output"

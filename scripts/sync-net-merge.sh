#!/usr/bin/env bash
# sync-net-merge.sh [tag]
#
# Pulls a tagged net-merge/net-merge release (or latest if omitted) into .reference/net-merge
# (gitignored, so git history and repos are never mixed) and syncs this repo's engine packages
# and cmd/net-merge entrypoint so NetPilot always embeds both net-merge and net-merge-tray.
#
# Usage:  scripts/sync-net-merge.sh [0.3.6]
set -euo pipefail

cd "$(dirname "$0")/.."

tag="${1:-}"
if [ -z "$tag" ]; then
  tag=$(curl -fsSL https://api.github.com/repos/net-merge/net-merge/releases/latest \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
fi
tag="${tag#v}"

src_url="https://github.com/net-merge/net-merge/archive/refs/tags/v${tag}.tar.gz"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo ">> Fetching net-merge v${tag} source into .reference/net-merge"
curl -fsSL "$src_url" -o "$work/net-merge.tar.gz"
mkdir -p .reference/net-merge
rm -rf .reference/net-merge/*
tar -xzf "$work/net-merge.tar.gz" -C .reference/net-merge --strip-components=1

# Sync cmd/net-merge so NetPilot can always build/embed both net-merge and net-merge-tray directly
if [ -d ".reference/net-merge/cmd/net-merge" ]; then
  echo ">> Syncing cmd/net-merge"
  rm -rf cmd/net-merge
  mkdir -p cmd/net-merge
  find ".reference/net-merge/cmd/net-merge" -maxdepth 1 -type f -name '*.go' | while read -r f; do
    rel=$(basename "$f")
    sed "s|net-merge/|netpilot/|g" "$f" > "cmd/net-merge/$rel"
  done
fi

echo "$tag" > internal/netmerge/VERSION
echo ">> Synced net-merge v${tag} reference and cmd/net-merge"

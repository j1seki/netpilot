#!/usr/bin/env bash
# sync-net-merge.sh <tag>
#
# Pulls a tagged net-merge/net-merge release into this repo's engine packages, so NetPilot
# always ships the newest net-merge engine. Only the engine packages are replaced — everything
# NetPilot adds (the desktop app, branding, tests, packaging) is left untouched.
#
# Usage:  scripts/sync-net-merge.sh 0.3.6
set -euo pipefail

tag=${1:?usage: sync-net-merge.sh <tag>}
tag=${tag#v}
src_url="https://github.com/net-merge/net-merge/archive/refs/tags/v${tag}.tar.gz"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Packages that come from the net-merge engine. Everything else in internal/ is NetPilot's.
engine_pkgs="engine icon logx nics platform state tray tun update"

echo ">> Fetching net-merge v${tag} source"
curl -fsSL "$src_url" -o "$work/net-merge.tar.gz"
mkdir -p "$work/src"
tar -xzf "$work/net-merge.tar.gz" -C "$work/src"
engine_root=$(find "$work/src" -maxdepth 1 -type d -name 'net-merge-*' | head -n1)
if [ -z "$engine_root" ]; then
  echo "could not locate extracted net-merge source" >&2
  exit 1
fi

echo ">> Replacing engine packages"
for pkg in $engine_pkgs; do
  src_dir="$engine_root/internal/$pkg"
  dst_dir="$(pwd)/internal/$pkg"
  if [ ! -d "$src_dir" ]; then
    echo "  WARN: $pkg not present in net-merge v${tag}; leaving ours in place" >&2
    continue
  fi
  rm -rf "$dst_dir"
  mkdir -p "$dst_dir"
  # Rewrite imports net-merge/ -> netpilot/: Go's internal/ rule means an external module
  # cannot import these packages, so they must live inside our own module.
  find "$src_dir" -type f | while read -r f; do
    rel=${f#"$src_dir"/}
    sed "s|net-merge/|netpilot/|g" "$f" > "$dst_dir/$rel"
  done
  echo "  - $pkg"
done

echo ">> Synced net-merge v${tag}"
echo "   Run 'go build ./...' and 'go test ./...' to verify, then commit."
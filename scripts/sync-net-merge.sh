#!/usr/bin/env bash
# sync-net-merge.sh [tag]
#
# Pulls a tagged net-merge/net-merge release (or the latest release if omitted) and
# applies the upstream changes to the net-merge packages vendored in this repo.
#
# NetPilot is a downstream fork: the engine packages intentionally contain local
# changes (Speed/Backup routing, extra platform probes, ...). Replacing them
# wholesale would silently drop those changes, which is how a previous sync
# shipped a new VERSION while the engine fixes from that release were missing.
# Instead this script performs a three-way merge for every file:
#
#   base   = the net-merge tag currently recorded in internal/netmerge/VERSION
#   ours   = the working tree
#   theirs = the requested net-merge tag
#
# Upstream-only changes merge cleanly; local-only changes are kept; overlapping
# changes fail loudly with conflict markers so a human can resolve them instead
# of shipping a half-synced engine.
#
# Usage:  scripts/sync-net-merge.sh [0.3.7]
set -euo pipefail

cd "$(dirname "$0")/.."

# Packages that come from the net-merge engine. Everything else in internal/ is
# NetPilot's own code and must never be touched by this script.
engine_pkgs="engine icon logx nics platform state tray tun update"
entry_pkgs="cmd/net-merge"

new_tag="${1:-}"
if [ -z "$new_tag" ]; then
  new_tag=$(gh api repos/net-merge/net-merge/releases/latest --jq .tag_name)
fi
new_tag="${new_tag#v}"

have_tag=""
if [ -f internal/netmerge/VERSION ]; then
  have_tag=$(tr -d '[:space:]' < internal/netmerge/VERSION)
fi
if [ -z "$have_tag" ]; then
  have_tag="$new_tag"
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetch_source() {
  local tag="$1" dest="$2"
  mkdir -p "$dest"
  echo ">> Fetching net-merge v${tag}"
  curl -fsSL "https://github.com/net-merge/net-merge/archive/refs/tags/v${tag}.tar.gz" \
    -o "$work/net-merge-${tag}.tar.gz"
  tar -xzf "$work/net-merge-${tag}.tar.gz" -C "$dest" --strip-components=1
}

# Go's internal/ rule means an external module cannot import these packages, so
# rewrite the module path while syncing.
rewrite_imports() {
  sed 's|net-merge/|netpilot/|g' "$1"
}

fetch_source "$have_tag" "$work/base"
if [ "$have_tag" != "$new_tag" ]; then
  fetch_source "$new_tag" "$work/new"
else
  # Still allow "missing file was added upstream" repair when re-running the
  # same tag, without treating every unchanged file as a merge candidate.
  cp -a "$work/base" "$work/new"
fi

conflicts=0

sync_path() {
  local rel="$1"
  local base="$work/base/$rel"
  local new="$work/new/$rel"
  local ours="$rel"

  if [ ! -d "$new" ]; then
    echo "  WARN: $rel is absent from net-merge v${new_tag}; leaving ours in place" >&2
    return 0
  fi
  mkdir -p "$ours"

  # Remove files upstream deleted, but only when our copy was untouched.
  if [ -d "$base" ]; then
    while IFS= read -r -d '' bf; do
      local relf="${bf#"$base"/}"
      if [ -e "$new/$relf" ] || [ ! -e "$ours/$relf" ]; then
        continue
      fi
      local base_tmp="$work/base-rewritten"
      rewrite_imports "$bf" > "$base_tmp"
      if cmp -s "$base_tmp" "$ours/$relf"; then
        rm -f "$ours/$relf"
        echo "  - $rel/$relf (deleted upstream)"
      else
        echo "  WARN: upstream deleted $rel/$relf but NetPilot has local changes; keeping it" >&2
      fi
    done < <(find "$base" -type f -print0)
  fi

  while IFS= read -r -d '' nf; do
    local relf="${nf#"$new"/}"
    local dst="$ours/$relf"
    mkdir -p "$(dirname "$dst")"

    if [ ! -e "$dst" ]; then
      rewrite_imports "$nf" > "$dst"
      echo "  + $rel/$relf"
      continue
    fi

    # Upstream did not change this file between the old and new tags.
    if [ -e "$base/$relf" ] && cmp -s "$base/$relf" "$nf"; then
      continue
    fi

    local new_tmp="$work/new-rewritten"
    rewrite_imports "$nf" > "$new_tmp"

    if [ -e "$base/$relf" ]; then
      local base_tmp="$work/base-rewritten"
      rewrite_imports "$base/$relf" > "$base_tmp"
      if cmp -s "$base_tmp" "$dst"; then
        # Our copy was identical to the old upstream file: take the new one.
        cp "$new_tmp" "$dst"
        echo "  ~ $rel/$relf (upstream update)"
      else
        # Both sides changed the file: merge instead of overwriting.
        if git merge-file -L "netpilot" -L "net-merge v${have_tag}" -L "net-merge v${new_tag}" \
          "$dst" "$base_tmp" "$new_tmp"; then
          echo "  ~ $rel/$relf (merged)"
        else
          echo "  !! conflict in $rel/$relf: resolve the conflict markers and re-run the tests" >&2
          conflicts=$((conflicts + 1))
        fi
      fi
    else
      # Upstream added a file with the same path as a NetPilot-only file.
      if cmp -s "$new_tmp" "$dst"; then
        :
      else
        echo "  !! conflict: upstream added $rel/$relf but NetPilot already has a different file" >&2
        conflicts=$((conflicts + 1))
      fi
    fi
  done < <(find "$new" -type f -print0)
}

for pkg in $engine_pkgs; do
  sync_path "internal/$pkg"
done
for pkg in $entry_pkgs; do
  sync_path "$pkg"
done

if [ "$conflicts" -ne 0 ]; then
  echo ">> ${conflicts} conflict(s): resolve them before committing" >&2
  exit 1
fi

echo "$new_tag" > internal/netmerge/VERSION
echo ">> Synced net-merge v${new_tag} (was v${have_tag})"

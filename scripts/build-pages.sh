#!/usr/bin/env bash
# Build a static GitHub Pages artifact without publishing anything.
set -euo pipefail
cd "$(dirname "$0")/.."
out="${1:-dist/pages}"
mkdir -p "$out/assets"
install -m 644 site/index.html site/style.css "$out/"
install -m 644 docs/screenshots/dashboard.png "$out/assets/dashboard.png"
install -m 644 packaging/icons/hicolor/64x64/apps/netpilot.png "$out/assets/netpilot.png"
touch "$out/.nojekyll"
printf 'GitHub Pages artifact: %s/index.html\n' "$out"

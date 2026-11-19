#!/bin/bash
# Use only with a NetPilot package you downloaded from the official release page.
set -euo pipefail
here="$(cd -- "$(dirname -- "$0")" && pwd -P)"
app="/Applications/NetPilot.app"
if [[ ! -d "$app" ]]; then
  app="$here/NetPilot.app"
fi
if [[ ! -f "$app/Contents/MacOS/netpilot" ]]; then
  printf 'Copy NetPilot.app to Applications, then run this helper again.\n'
  exit 1
fi
printf 'Open %s? This removes the download quarantine from NetPilot only.\n' "$app"
printf 'Continue only if you trust this copy. Type yes to proceed: '
read -r answer
[[ "$answer" == yes ]] || exit 0
/usr/bin/codesign --verify --deep --strict "$app"
if /usr/bin/xattr -r "$app" | /usr/bin/grep 'com.apple.quarantine' >/dev/null; then
  /usr/bin/xattr -rd com.apple.quarantine "$app"
fi
/usr/bin/open "$app"

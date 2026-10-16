#!/usr/bin/env bash
# Regenerate README screenshots from mock data (no network, no real keys).
# Requires: go, freeze (github.com/charmbracelet/freeze), google-chrome.
# Each PNG is rendered directly from the screen named in its file, so the
# README captions can't drift out of sync the way recorded tapes could.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
HAX_SCREENSHOT_DIR="$tmp" go test ./internal/ui/models -tags screenshots -run TestScreenshots -count=1
for f in dashboard receive send history utxos help; do
  freeze --language ansi --window=false --theme catppuccin-mocha --padding 24 \
    --font.size 15 --output "$tmp/$f.svg" "$tmp/$f.ansi" </dev/null >/dev/null
  read -r W H < <(head -c 400 "$tmp/$f.svg" | grep -oE 'width="[0-9.]+" height="[0-9.]+"' | head -1 | tr -dc '0-9. ' | awk '{print int($1)+1, int($2)+1}')
  printf '<html><body style="margin:0;background:#1e1e2e"><img src="%s.svg"></body></html>' "$f" >"$tmp/$f.html"
  google-chrome --headless=new --no-sandbox --disable-gpu --hide-scrollbars \
    --force-device-scale-factor=2 --window-size="$W,$H" \
    --screenshot="docs/images/$f.png" "file://$tmp/$f.html" >/dev/null 2>&1
done
echo "screenshots written to docs/images/"

#!/usr/bin/env bash
# Regenerate README screenshots and the tour GIF from mock data (no network,
# no real keys). Requires: go, freeze (github.com/charmbracelet/freeze),
# google-chrome, ffmpeg.
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
# Tour GIF: the same PNGs, 2 s each, centred on one canvas. Built from still
# images, so there is no terminal cursor in any frame.
tour=(dashboard receive send history utxos help)
inputs=() ; chain=""
for i in "${!tour[@]}"; do
  inputs+=(-loop 1 -t 2 -i "docs/images/${tour[$i]}.png")
  chain+="[$i:v]scale=1180:820:force_original_aspect_ratio=decrease,pad=1180:820:(ow-iw)/2:(oh-ih)/2:color=0x1e1e2e,setsar=1,fps=10[v$i];"
done
for i in "${!tour[@]}"; do chain+="[v$i]"; done
chain+="concat=n=${#tour[@]}:v=1:a=0,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=none"
ffmpeg -loglevel error -y "${inputs[@]}" -filter_complex "$chain" -loop 0 docs/images/hax-tour.gif
echo "screenshots and hax-tour.gif written to docs/images/"

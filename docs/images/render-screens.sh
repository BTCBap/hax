#!/usr/bin/env bash
# Regenerate the README screenshots and the tour GIF from mock data (no network,
# no real keys). Requires: go, freeze (github.com/charmbracelet/freeze),
# google-chrome, ffmpeg, python3.
# The screenshot test renders each screen as true-color ANSI at 110x34; every
# PNG is then produced from the screen named after its file, so the README
# captions can't drift out of sync the way recorded tapes could.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
HAX_SCREENSHOT_DIR="$tmp" go test ./internal/ui/models -tags screenshots -run TestScreenshots -count=1

# render_png <screen> <dir>: freeze the screen's .ansi render to SVG, snap the
# text to the terminal cell grid (grid-svg.py), then screenshot that with
# headless Chrome at 2x scale. The README images and the tour-only frames share
# these settings, so they look identical.
render_png() {
  local name=$1 outdir=$2 W H
  # freeze ignores SGR 7, so resolve reverse video to explicit colors first.
  python3 docs/images/ansi-reverse.py "$tmp/$name.ansi"
  freeze --language ansi --window=false --theme catppuccin-mocha --padding 24 \
    --font.size 15 --output "$tmp/$name.svg" "$tmp/$name.ansi" </dev/null >/dev/null
  python3 docs/images/grid-svg.py "$tmp/$name.svg"
  read -r W H < <(head -c 400 "$tmp/$name.svg" | grep -oE 'width="[0-9.]+" height="[0-9.]+"' | head -1 | tr -dc '0-9. ' | awk '{print int($1)+1, int($2)+1}')
  printf '<html><body style="margin:0;background:#1e1e2e"><img src="%s.svg"></body></html>' "$name" >"$tmp/$name.html"
  google-chrome --headless=new --no-sandbox --disable-gpu --hide-scrollbars \
    --force-device-scale-factor=2 --window-size="$W,$H" \
    --screenshot="$outdir/$name.png" "file://$tmp/$name.html" >/dev/null 2>&1
}

# PNGs shown in the README grid, committed to docs/images.
readme=(dashboard receive send history utxos help txflow verify)
for f in "${readme[@]}"; do
  render_png "$f" docs/images
done

# Tour GIF: 28 v1.4 screens in a user-journey order, 2 s each, on one 1180x820
# canvas. Frames are stills, so fps=2 (four frames per screen) keeps the GIF
# small. Tour-only screens are rendered with the same settings into the temp
# dir and are never committed.
tour=(splash setup_network setup_choice setup_seed setup_quiz unlock dashboard
  receive verify send_address send_amount send_fee send_hw_devices send txflow
  psbt_inspect fees history history_speedup_tray utxos addresses settings
  settings_server multisig labels_import_preview backup_export_confirm
  dashboard_stealth help)
inputs=() ; chain=""
for i in "${!tour[@]}"; do
  f=${tour[$i]}
  png="docs/images/$f.png"
  if [[ ! -f "$png" ]]; then
    render_png "$f" "$tmp"
    png="$tmp/$f.png"
  fi
  inputs+=(-loop 1 -t 2 -i "$png")
  chain+="[$i:v]scale=1180:820:force_original_aspect_ratio=decrease,pad=1180:820:(ow-iw)/2:(oh-ih)/2:color=0x1e1e2e,setsar=1,fps=2[v$i];"
done
for i in "${!tour[@]}"; do chain+="[v$i]"; done
chain+="concat=n=${#tour[@]}:v=1:a=0,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=none"
ffmpeg -loglevel error -y "${inputs[@]}" -filter_complex "$chain" -loop 0 docs/images/hax-tour.gif
echo "screenshots and hax-tour.gif written to docs/images/"

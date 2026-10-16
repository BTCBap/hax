# Records the README tour GIF only. Static screenshots come from
# docs/images/render-screens.sh (rendered directly, one screen per file).
#   HAX_SCREENSHOT_DIR=/tmp/hax-screens go test ./internal/ui/models -tags screenshots -run TestScreenshots
Output "docs/images/hax-tour.gif"
Set Shell bash
Set Width 1180
Set Height 820
Set FontSize 15
Set Padding 24
Set Theme "Catppuccin Mocha"
Hide
Type "export PS1=; clear" Enter
Show
Hide
Type@0ms "clear; cat /tmp/hax-screens/dashboard.ansi" Enter
Show
Sleep 2s
Hide
Type@0ms "clear; cat /tmp/hax-screens/receive.ansi" Enter
Show
Sleep 2s
Hide
Type@0ms "clear; cat /tmp/hax-screens/send.ansi" Enter
Show
Sleep 2s
Hide
Type@0ms "clear; cat /tmp/hax-screens/history.ansi" Enter
Show
Sleep 2s
Hide
Type@0ms "clear; cat /tmp/hax-screens/utxos.ansi" Enter
Show
Sleep 2s
Hide
Type@0ms "clear; cat /tmp/hax-screens/help.ansi" Enter
Show
Sleep 2s
Sleep 1s

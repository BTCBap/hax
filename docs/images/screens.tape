# Regenerate README screenshots (mock data, no network, no real keys):
#   HAX_SCREENSHOT_DIR=/tmp/hax-screens go test ./internal/ui/models -tags screenshots -run TestScreenshots
#   vhs docs/images/screens.tape
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
Screenshot "docs/images/dashboard.png"
Hide
Type@0ms "clear; cat /tmp/hax-screens/receive.ansi" Enter
Show
Sleep 2s
Screenshot "docs/images/receive.png"
Hide
Type@0ms "clear; cat /tmp/hax-screens/history.ansi" Enter
Show
Sleep 2s
Screenshot "docs/images/history.png"
Hide
Type@0ms "clear; cat /tmp/hax-screens/help.ansi" Enter
Show
Sleep 2s
Screenshot "docs/images/help.png"
Sleep 1s

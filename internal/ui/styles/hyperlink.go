package styles

import (
	"hash/fnv"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Hyperlink wraps each line of text in an OSC 8 hyperlink to url. Terminals
// that support OSC 8 open the full URL even when the visible text is
// shortened or wrapped. The shared id groups the fragments of one link.
func Hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	style := lipgloss.NewStyle().Hyperlink(url, "id="+hyperlinkID(url))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func hyperlinkID(url string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(url))
	return "hax-" + strconv.FormatUint(uint64(h.Sum32()), 16)
}

// FitURL returns url unchanged when it fits in width display cells. Otherwise
// it shortens the middle with an ellipsis so the result stays on one line.
func FitURL(url string, width int) string {
	if width <= 0 || ansi.StringWidth(url) <= width {
		return url
	}
	if width == 1 {
		return "…"
	}
	keep := width - 1
	head := (keep + 1) / 2
	tail := keep - head
	runes := []rune(url)
	if tail >= len(runes) {
		return "…"
	}
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFitURL(t *testing.T) {
	url := "https://mempool.space/testnet4/tx/" + strings.Repeat("a", 64)
	urlWidth := ansi.StringWidth(url)

	cases := []struct {
		name  string
		width int
		want  string
	}{
		{name: "fits", width: urlWidth, want: url},
		{name: "zero width", width: 0, want: url},
		{name: "one cell", width: 1, want: "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FitURL(url, tc.width); got != tc.want {
				t.Errorf("FitURL(%d) = %q, want %q", tc.width, got, tc.want)
			}
		})
	}

	for _, width := range []int{12, 40, 66, urlWidth - 1} {
		got := FitURL(url, width)
		if cells := ansi.StringWidth(got); cells > width {
			t.Errorf("FitURL(%d) is %d cells: %q", width, cells, got)
		}
		if !strings.Contains(got, "…") {
			t.Errorf("FitURL(%d) has no middle ellipsis: %q", width, got)
		}
		if !strings.HasPrefix(got, "h") || !strings.HasSuffix(got, "aa") {
			t.Errorf("FitURL(%d) lost the URL ends: %q", width, got)
		}
	}
}

func TestHyperlinkWrapsEachLine(t *testing.T) {
	url := "https://mempool.space/testnet4/tx/b1769e4f6a2c8d0e3b5f7a9c1d2e4f6081a3b5c7d9e0f2a4b6c8d0e2f4a67685a2"
	visible := "https://mempool.space/testnet4/tx/b1769e…7685a2"
	out := Hyperlink(url, visible)
	if !strings.Contains(out, url) {
		t.Fatalf("hyperlink does not contain the full URL: %q", out)
	}
	if !strings.Contains(out, "id=") {
		t.Fatalf("hyperlink has no id parameter: %q", out)
	}
	if got := ansi.Strip(out); got != visible {
		t.Errorf("visible text = %q, want %q", got, visible)
	}
	if got := ansi.StringWidth(ansi.Strip(out)); got != ansi.StringWidth(visible) {
		t.Errorf("hyperlink changed the measured width to %d, want %d", got, ansi.StringWidth(visible))
	}

	// Each line is wrapped separately so a wrapped link stays clickable.
	multi := Hyperlink(url, "first\nsecond")
	lines := strings.Split(multi, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for i, line := range lines {
		if !strings.Contains(line, url) {
			t.Errorf("line %d is not linked: %q", i, line)
		}
	}
}

func TestHyperlinkEmptyURLIsPlainText(t *testing.T) {
	if got := Hyperlink("", "text"); got != "text" {
		t.Errorf("Hyperlink with empty URL = %q, want plain text", got)
	}
}

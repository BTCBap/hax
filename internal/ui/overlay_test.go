package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func plainLines(s string) []string {
	lines := strings.Split(ansi.Strip(s), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

func TestCompositePlacesOverlayAtExplicitCells(t *testing.T) {
	base := strings.Join([]string{
		"abcdefghij",
		"klmnopqrst",
		"uvwxyz0123",
	}, "\n")

	got := Composite(base, 10, 3, Overlay{Content: "XY", X: 3, Y: 1}.TopLeft())

	want := []string{"abcdefghij", "klmXYpqrst", "uvwxyz0123"}
	for i, line := range plainLines(got) {
		if line != want[i] {
			t.Fatalf("line %d = %q, want %q", i, line, want[i])
		}
	}
}

func TestCompositeAnchorsUseDisplayWidth(t *testing.T) {
	base := strings.Repeat(".", 8)
	cases := []struct {
		name   string
		anchor Overlay
		want   string
	}{
		{"top-right one cell", Overlay{Content: "X"}.TopRight(), ".......X"},
		{"top-right wide rune", Overlay{Content: "界"}.TopRight(), "......界"},
		{"bottom-center", Overlay{Content: "ab"}.BottomCenter(), "...ab..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Composite(base, 8, 1, tc.anchor)
			if line := plainLines(got)[0]; line != tc.want {
				t.Fatalf("line = %q, want %q", line, tc.want)
			}
			if w := ansi.StringWidth(got); w != 8 {
				t.Fatalf("width = %d, want 8", w)
			}
		})
	}
}

func TestCompositeWideRunesKeepTheirCells(t *testing.T) {
	// The base mixes a CJK rune, a bitcoin sign, and ASCII. The overlay starts
	// after three display cells, so it lands after 界 and ₿ and overwrites the
	// first ASCII rune.
	base := "界₿abc"
	got := Composite(base, 7, 1, Overlay{Content: "Z", X: 3, Y: 0}.TopLeft())

	if line := plainLines(got)[0]; line != "界₿Zbc" {
		t.Fatalf("line = %q, want %q", line, "界₿Zbc")
	}
	if w := ansi.StringWidth(got); w != 7 {
		t.Fatalf("width = %d, want 7", w)
	}
}

func TestCompositePreservesBaseStylingOutsideOverlay(t *testing.T) {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("hello") +
		" world"
	got := Composite(base, 11, 1, Overlay{Content: "X", X: 0, Y: 0}.TopLeft())

	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("base ANSI styling was lost: %q", got)
	}
	if line := plainLines(got)[0]; line != "Xello world" {
		t.Fatalf("line = %q, want %q", line, "Xello world")
	}
}

func TestCompositeOutputSizeExact(t *testing.T) {
	got := Composite("short\nlonger line than the frame", 10, 4,
		Overlay{Content: "x", X: 9, Y: 3}.TopLeft())
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("height = %d, want 4", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != 10 {
			t.Fatalf("line %d width = %d, want 10: %q", i, w, ansi.Strip(line))
		}
	}
}

func TestCompositeClipsLargeOverlayInsteadOfWrapping(t *testing.T) {
	overlay := strings.Join([]string{
		"1234567890",
		"abcdefghij",
		"ABCDEFGHIJ",
	}, "\n")
	got := Composite("base", 5, 2, Overlay{Content: overlay, X: 0, Y: 0}.TopLeft())

	want := []string{"12345", "abcde"}
	for i, line := range plainLines(got) {
		if line != want[i] {
			t.Fatalf("line %d = %q, want %q", i, line, want[i])
		}
	}
	if lines := strings.Split(got, "\n"); len(lines) != 2 {
		t.Fatalf("overlay wrapped to %d lines, want 2", len(lines))
	}
}

func TestCompositeClipsOverlayAtFrameEdge(t *testing.T) {
	got := Composite("......\n......", 6, 2,
		Overlay{Content: "1234\n5678", X: 4, Y: 1}.TopLeft())
	want := []string{"......", "....12"}
	for i, line := range plainLines(got) {
		if line != want[i] {
			t.Fatalf("line %d = %q, want %q", i, line, want[i])
		}
	}
}

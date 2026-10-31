package models

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const testXpub = "tpubDC8msFGeGuwnKG9Upg7DM2b4DaRqg3CUZa5g8v2SRQ6K4NSkxUgd7HsL2XVWbVm39yBA4LAxysQAm397zwQSQoQgewGiYZqrA9DsP4zbQ1M"

// testDescriptorLines returns the border-stripped content lines of the box.
func testDescriptorLines(t *testing.T, box string) []string {
	t.Helper()
	lines := strings.Split(box, "\n")
	if len(lines) < 3 {
		t.Fatalf("descriptor box has %d lines, want at least 3:\n%s", len(lines), ansi.Strip(box))
	}
	content := make([]string, 0, len(lines)-2)
	for _, line := range lines[1 : len(lines)-1] {
		content = append(content, strings.TrimSuffix(strings.TrimPrefix(ansi.Strip(line), "│"), "│"))
	}
	return content
}

// testLineStarts returns the byte offsets at which lines begin in text.
func testLineStarts(lines []string) map[int]bool {
	starts := make(map[int]bool, len(lines)+1)
	off := 0
	starts[off] = true
	for _, line := range lines {
		off += len(line)
		starts[off] = true
	}
	return starts
}

func TestDescriptorSegmentsStartKeyOrigins(t *testing.T) {
	descriptor := "wsh(sortedmulti(2," +
		"[0f056943/48h/1h/0h/2h]" + testXpub + "/0/*," +
		"[abcd1234/48h/1h/0h/2h]" + testXpub + "/1/*," +
		"[cafebabe/48h/1h/0h/2h]" + testXpub + "/2/*))#cs"

	segments := descriptorSegments(descriptor)
	if len(segments) != 4 {
		t.Fatalf("got %d segments, want 4: %q", len(segments), segments)
	}
	if !strings.HasPrefix(segments[0], "wsh(sortedmulti(2,") {
		t.Errorf("first segment %q does not start the descriptor", segments[0])
	}
	for i, segment := range segments[1:] {
		if !strings.HasPrefix(segment, "[") {
			t.Errorf("segment %d does not start a key origin: %q", i+1, segment)
		}
	}
	if got := strings.Join(segments, ""); got != descriptor {
		t.Error("segments do not reassemble the descriptor")
	}

	// Every key origin's bracket prefix stays on one line at every width.
	for _, width := range []int{200, 120, 80} {
		lines := wrapDescriptor(descriptor, width)
		starts := testLineStarts(lines)
		off := 0
		for i, segment := range segments {
			if !starts[off] {
				t.Errorf("width %d: segment %d does not start a line: %q", width, i, segment)
			}
			off += len(segment)
		}
		for i, r := range descriptor {
			if r != '[' {
				continue
			}
			end := strings.IndexByte(descriptor[i:], ']')
			if end < 0 {
				break
			}
			for boundary := range starts {
				if boundary > i && boundary < i+end {
					t.Errorf("width %d: line break inside key origin %q", width, descriptor[i:i+end+1])
				}
			}
		}
	}

	// When a key segment fits, it occupies exactly one line.
	wide := wrapDescriptor(descriptor, 200)
	if len(wide) != len(segments) {
		t.Fatalf("at width 200 got %d lines, want %d: %q", len(wide), len(segments), wide)
	}
}

func TestWrapDescriptorSegmenting(t *testing.T) {
	descriptor := "wsh(sortedmulti(2,[a/48h]xpub/0/*,[b/48h]xpub/0/*))#cs"
	want := []string{
		"wsh(sortedmulti(2,",
		"[a/48h]xpub/0/*,",
		"[b/48h]xpub/0/*))#cs",
	}
	lines := wrapDescriptor(descriptor, 200)
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestWrapDescriptorBalancesLongSegment(t *testing.T) {
	descriptor := "wpkh([0f056943/84h/1h/0h]" + testXpub + "/0/*)#8f3a2b1c"
	total := ansi.StringWidth(descriptor)

	for _, width := range []int{65, 80, 120} {
		lines := wrapDescriptor(descriptor, width)
		wantLines := (total + width - 1) / width
		if len(lines) != wantLines {
			t.Fatalf("width %d: got %d lines, want %d: %q", width, len(lines), wantLines, lines)
		}
		if got := strings.Join(lines, ""); got != descriptor {
			t.Errorf("width %d: wrapped lines do not reassemble the descriptor", width)
		}
		target := (total + wantLines - 1) / wantLines
		for i, line := range lines {
			lineWidth := ansi.StringWidth(line)
			if lineWidth > width {
				t.Errorf("width %d: line %d is %d cells, want at most %d: %q", width, i, lineWidth, width, line)
			}
			if i == len(lines)-1 {
				continue
			}
			if lineWidth != target {
				t.Errorf("width %d: line %d is %d cells, want the balanced %d: %q", width, i, lineWidth, target, line)
			}
			if i+1 < len(lines)-1 {
				diff := lineWidth - ansi.StringWidth(lines[i+1])
				if diff < -1 || diff > 1 {
					t.Errorf("width %d: lines %d and %d differ by %d cells: %q / %q", width, i, i+1, diff, line, lines[i+1])
				}
			}
		}
	}
}

func TestDescriptorBoxCentersWrappedLines(t *testing.T) {
	descriptor := "sortedmulti(2," +
		"[0f056943/48h/1h/0h/2h]" + testXpub + "/0/*," +
		"[abcd1234/48h/1h/0h/2h]" + testXpub + "/1/*," +
		"[cafebabe/48h/1h/0h/2h]" + testXpub + "/2/*)"

	for _, width := range []int{80, 120} {
		m := SettingsModel{width: width, descriptorText: descriptor}
		box := m.renderDescriptorBox()
		boxWidth := descriptorBoxWidth(m.width)
		inner := descriptorInnerWidth(boxWidth)
		want := wrapDescriptor(descriptor, inner)

		// The first and last lines are the border.
		for i, line := range strings.Split(box, "\n")[1:] {
			if got := ansi.StringWidth(line); got > boxWidth {
				t.Errorf("width %d: descriptor line %d is %d cells wide, want at most %d: %q", width, i, got, boxWidth, ansi.Strip(line))
			}
		}

		content := testDescriptorLines(t, box)
		if len(content) != len(want) {
			t.Fatalf("width %d: box has %d content lines, want %d:\n%s", width, len(content), len(want), ansi.Strip(box))
		}
		for i, line := range content {
			if got := strings.TrimSpace(line); got != want[i] {
				t.Errorf("width %d: line %d = %q, want %q", width, i, got, want[i])
			}
			left := len(line) - len(strings.TrimLeft(line, " "))
			right := len(line) - len(strings.TrimRight(line, " "))
			if left-right > 1 || right-left > 1 {
				t.Errorf("width %d: descriptor line %d is not centered (left %d, right %d): %q", width, i, left, right, line)
			}
			if ansi.StringWidth(strings.TrimSpace(line)) > inner {
				t.Errorf("width %d: descriptor line %d exceeds inner width %d: %q", width, i, inner, line)
			}
		}
	}
}

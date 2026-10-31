package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripFormatANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func TestFormatAddress(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	const (
		p2wpkh = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
		p2wsh  = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
		p2tr   = "tb1pqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesf3hn0c"
	)

	tests := []struct {
		name       string
		address    string
		width      int
		wantText   string
		wantMiddle string
		headLen    int
		tailLen    int
	}{
		{
			name: "p2wpkh full", address: p2wpkh, width: 0,
			wantText:   "tb1qw508 d6qejxtdg4y5r3zarvary0c5xw 7kxpjzsx",
			wantMiddle: "d6qejxtdg4y5r3zarvary0c5xw", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wpkh exact address length", address: p2wpkh, width: 42,
			wantText:   "tb1qw508 d6qejxtdg4y5r3zarvary0c… 7kxpjzsx",
			wantMiddle: "d6qejxtdg4y5r3zarvary0c…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wpkh full width", address: p2wpkh, width: 44,
			wantText:   "tb1qw508 d6qejxtdg4y5r3zarvary0c5xw 7kxpjzsx",
			wantMiddle: "d6qejxtdg4y5r3zarvary0c5xw", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wpkh mid width", address: p2wpkh, width: 33,
			wantText:   "tb1qw508 d6qejxtdg4y5r3… 7kxpjzsx",
			wantMiddle: "d6qejxtdg4y5r3…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wpkh width 20", address: p2wpkh, width: 20,
			wantText:   "tb1qw508 d… 7kxpjzsx",
			wantMiddle: "d…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wpkh width 12", address: p2wpkh, width: 12,
			wantText: "tb1qw … jzsx", wantMiddle: "…", headLen: 5, tailLen: 4,
		},
		{
			name: "p2wpkh width 9", address: p2wpkh, width: 9,
			wantText: "tb1 … zsx", wantMiddle: "…", headLen: 3, tailLen: 3,
		},
		{
			name: "p2wpkh width 7", address: p2wpkh, width: 7,
			wantText: "tb...sx", wantMiddle: "...", headLen: 2, tailLen: 2,
		},
		{
			name: "p2wsh full", address: p2wsh, width: 0,
			wantText:   "tb1qrp33 g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf 3q0sl5k7",
			wantMiddle: "g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wsh exact address length", address: p2wsh, width: 62,
			wantText:   "tb1qrp33 g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpy… 3q0sl5k7",
			wantMiddle: "g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpy…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wsh full width", address: p2wsh, width: 64,
			wantText:   "tb1qrp33 g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf 3q0sl5k7",
			wantMiddle: "g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wsh mid width", address: p2wsh, width: 43,
			wantText:   "tb1qrp33 g0q5c5txsp9arysrx4k6zdkf… 3q0sl5k7",
			wantMiddle: "g0q5c5txsp9arysrx4k6zdkf…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wsh width 20", address: p2wsh, width: 20,
			wantText:   "tb1qrp33 g… 3q0sl5k7",
			wantMiddle: "g…", headLen: 8, tailLen: 8,
		},
		{
			name: "p2wsh width 12", address: p2wsh, width: 12,
			wantText: "tb1qr … l5k7", wantMiddle: "…", headLen: 5, tailLen: 4,
		},
		{
			name: "p2wsh width 9", address: p2wsh, width: 9,
			wantText: "tb1 … 5k7", wantMiddle: "…", headLen: 3, tailLen: 3,
		},
		{
			name: "p2wsh width 7", address: p2wsh, width: 7,
			wantText: "tb...k7", wantMiddle: "...", headLen: 2, tailLen: 2,
		},
		{
			name: "p2tr full", address: p2tr, width: 0,
			wantText:   "tb1pqqqq p399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvs esf3hn0c",
			wantMiddle: "p399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvs", headLen: 8, tailLen: 8,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatAddress(tc.address, tc.width)
			if plain := stripFormatANSI(got); plain != tc.wantText {
				t.Fatalf("FormatAddress(%q, %d) text = %q, want %q", tc.address, tc.width, plain, tc.wantText)
			}
			if w := lipgloss.Width(got); w != lipgloss.Width(tc.wantText) {
				t.Fatalf("FormatAddress(%q, %d) width = %d, want %d", tc.address, tc.width, w, lipgloss.Width(tc.wantText))
			}
			if tc.width > 0 {
				fullWidth := len(tc.address) + 2
				wantWidth := min(tc.width, fullWidth)
				if w := lipgloss.Width(got); w != wantWidth {
					t.Fatalf("FormatAddress(%q, %d) width = %d, want min(%d, %d) = %d",
						tc.address, tc.width, w, tc.width, fullWidth, wantWidth)
				}
			}

			head := tc.address[:tc.headLen]
			tail := tc.address[len(tc.address)-tc.tailLen:]
			if !strings.HasPrefix(got, addressHeadStyle.Render(head)) {
				t.Fatalf("first group %q is not in the head style: %q", head, got)
			}
			if !strings.HasSuffix(got, addressHeadStyle.Render(tail)) {
				t.Fatalf("last group %q is not in the head style: %q", tail, got)
			}
			if !strings.Contains(got, addressMiddleStyle.Render(tc.wantMiddle)) {
				t.Fatalf("middle %q is not in the middle style: %q", tc.wantMiddle, got)
			}

			headRendered := addressHeadStyle.Render(head)
			tailRendered := addressHeadStyle.Render(tail)
			middleRendered := addressMiddleStyle.Render(tc.wantMiddle)
			if middleRendered != headRendered && middleRendered != tailRendered &&
				strings.Contains(got, addressHeadStyle.Render(tc.wantMiddle)) {
				t.Fatalf("middle %q carries the highlight style: %q", tc.wantMiddle, got)
			}
		})
	}
}

func TestFormatAddressShortAndControlCharacters(t *testing.T) {
	const short = "tb1qshort"
	if got, want := FormatAddress(short, 0), addressHeadStyle.Render(short); got != want {
		t.Fatalf("short address = %q, want %q", got, want)
	}
	if got, want := AddressWidth(short, 0), len(short); got != want {
		t.Fatalf("short address width = %d, want %d", got, want)
	}
	if got := lipgloss.Width(FormatAddress(short, 5)); got != 5 {
		t.Fatalf("narrow short address width = %d, want 5", got)
	}
	if got, want := AddressWidth(short, 5), 5; got != want {
		t.Fatalf("narrow short address AddressWidth = %d, want %d", got, want)
	}

	const clean = "tb1qtest"
	for _, dirty := range []string{"tb1q\ntest", "tb1q\tte\x1bst", "tb1q\x00test"} {
		if got := stripFormatANSI(FormatAddress(dirty, 0)); got != clean {
			t.Fatalf("FormatAddress(%q, 0) = %q, want %q", dirty, got, clean)
		}
	}
}

func TestAddressWidthMatchesFormatAddress(t *testing.T) {
	const (
		p2wpkh = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
		short  = "tb1qshort"
	)
	for _, address := range []string{p2wpkh, short} {
		for _, width := range []int{0, 1, 7, 9, 12, 20, 42, 44, 80} {
			got := AddressWidth(address, width)
			want := lipgloss.Width(FormatAddress(address, width))
			if got != want {
				t.Fatalf("AddressWidth(%q, %d) = %d, formatted width = %d", address, width, got, want)
			}
		}
	}
}

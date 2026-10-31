package styles

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

func TestDarkPaletteIsDefault(t *testing.T) {
	SetDarkBackground(true)
	t.Cleanup(func() { SetDarkBackground(true) })

	cases := []struct {
		name string
		got  color.Color
		want string
	}{
		{"PastelOrange", PastelOrange, "#FFD4A3"},
		{"BitcoinOrange", BitcoinOrange, "#FFB374"},
		{"PrimaryBg", PrimaryBg, "#0F111A"},
		{"TextPrimary", TextPrimary, "#FAFAFA"},
		{"TextMuted", TextMuted, "#718096"},
		{"AddressMiddle", AddressMiddle, "#BAE1FF"},
	}
	for _, tc := range cases {
		if got := Hex(tc.got); !strings.EqualFold(got, strings.ToLower(tc.want)) {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}

	if got, ok := Placeholder.(ansi.IndexedColor); !ok || int(got) != 240 {
		t.Errorf("Placeholder = %v, want ANSI 240", Placeholder)
	}
}

func TestLightPaletteContrast(t *testing.T) {
	SetDarkBackground(false)
	defer SetDarkBackground(true)

	white := lipgloss.Color("#FFFFFF")
	lightGrey := lipgloss.Color("#F5F5F5")

	helperText := []struct {
		name string
		col  color.Color
	}{
		{"TextMuted", TextMuted},
		{"MediumGray", MediumGray},
		{"TextSecondary", TextSecondary},
	}
	for _, tc := range helperText {
		for _, bg := range []color.Color{white, lightGrey} {
			if got := contrastRatio(tc.col, bg); got < 4.5 {
				t.Errorf("%s contrast on %s = %.2f, want >= 4.5", tc.name, Hex(bg), got)
			}
		}
	}

	accents := []struct {
		name string
		col  color.Color
	}{
		{"PastelOrange", PastelOrange},
		{"PastelPink", PastelPink},
		{"PastelPurple", PastelPurple},
		{"PastelBlue", PastelBlue},
		{"PastelGreen", PastelGreen},
		{"PastelYellow", PastelYellow},
		{"BitcoinOrange", BitcoinOrange},
		{"BitcoinGold", BitcoinGold},
		{"Amber", Amber},
		{"Copper", Copper},
		{"SuccessGreen", SuccessGreen},
		{"ErrorRed", ErrorRed},
		{"WarningYellow", WarningYellow},
		{"InfoBlue", InfoBlue},
	}
	for _, tc := range accents {
		if got := contrastRatio(tc.col, white); got < 3.0 {
			t.Errorf("%s contrast on white = %.2f, want >= 3.0", tc.name, got)
		}
	}
}

func contrastRatio(a, b color.Color) float64 {
	ca, _ := colorful.MakeColor(a)
	cb, _ := colorful.MakeColor(b)
	la := relativeLuminance(ca)
	lb := relativeLuminance(cb)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(c colorful.Color) float64 {
	r, g, b := c.LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

package models

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// passwordStrength scores a wallet password from 0 (too short) to 4
// (strong) using length and character variety. It is a hint only; the
// enforced rule stays wallet.MinPasswordLength.
func passwordStrength(pw string) (int, string) {
	n := utf8.RuneCountInString(pw)
	if n < wallet.MinPasswordLength {
		return 0, "Too short"
	}
	var lower, upper, digit, other bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	classes := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			classes++
		}
	}
	score := 1
	if n >= 12 {
		score++
	}
	if n >= 16 {
		score++
	}
	if classes >= 3 || n >= 20 {
		score++
	}
	if distinctRunes(pw) <= 3 { // "aaaaaaaa", "abababab"
		score = 1
	}
	if score > 4 {
		score = 4
	}
	return score, []string{"Too short", "Weak", "Fair", "Good", "Strong"}[score]
}

func distinctRunes(s string) int {
	seen := map[rune]bool{}
	for _, r := range s {
		seen[r] = true
	}
	return len(seen)
}

// renderPasswordStrength draws a four-segment meter with a label. Empty
// input renders nothing.
func renderPasswordStrength(pw string) string {
	if pw == "" {
		return ""
	}
	score, label := passwordStrength(pw)
	bar := strings.Repeat("■", score) + strings.Repeat("□", 4-score)
	style := styles.ErrorText
	switch {
	case score >= 4:
		style = styles.SuccessText
	case score == 3:
		style = styles.SuccessText
	case score == 2:
		style = styles.WarningText
	}
	return style.Render(bar + " " + label)
}

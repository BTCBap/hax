package models

import (
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

const (
	// amountMask is the fixed mask shown in place of a hidden amount.
	amountMask = "••••••"

	// stealthBadgeWidth keeps the header badge in a fixed slot.
	stealthBadgeWidth = 10

	// passphraseBadgeWidth reserves room for " + passphrase" beside a name.
	passphraseBadgeWidth = 13
)

// maskAmount replaces an amount with a right-aligned mask that occupies the
// same number of cells as the real value, so nearby labels never move.
func maskAmount(s string, hidden bool) string {
	if !hidden {
		return s
	}
	width := lipgloss.Width(s)
	if width <= 0 {
		return ""
	}
	mask := amountMask
	for lipgloss.Width(mask) > width {
		r := []rune(mask)
		mask = string(r[:len(r)-1])
	}
	return strings.Repeat(" ", width-lipgloss.Width(mask)) + mask
}

// stealthBadge returns the dim "◌ hidden" badge in a fixed-width slot, or an
// empty string when stealth mode is off.
func stealthBadge(on bool) string {
	if !on {
		return ""
	}
	return lipgloss.NewStyle().Width(stealthBadgeWidth).
		Render(styles.DimText.Render("◌ hidden"))
}

// passphraseBadge returns a dim badge for a wallet that was created or
// restored with a BIP39 passphrase.
func passphraseBadge(has bool) string {
	if !has {
		return ""
	}
	return lipgloss.NewStyle().Width(passphraseBadgeWidth).
		Render(styles.DimText.Render(" + passphrase"))
}

// walletNameWithBadge truncates a wallet name so it and the optional
// passphrase badge fit in maxWidth cells.
func walletNameWithBadge(name string, hasPassphrase bool, maxWidth int) string {
	badge := passphraseBadge(hasPassphrase)
	if hasPassphrase {
		maxWidth -= passphraseBadgeWidth
	}
	if maxWidth < 3 {
		maxWidth = 3
	}
	return truncateRunes(name, maxWidth, "…") + badge
}

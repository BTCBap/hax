package models

import (
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

const (
	// stealthBadgeWidth keeps the header badge in a fixed slot.
	stealthBadgeWidth = 10

	// passphraseBadgeWidth reserves room for " + passphrase" beside a name.
	passphraseBadgeWidth = 13
)

// maskAmount replaces the numeric part of a left-aligned amount with bullets,
// preserving the unit suffix and the original width so nearby text never moves.
func maskAmount(s string, hidden bool) string {
	return maskAmountAligned(s, hidden, false)
}

// maskAmountRight is maskAmount for right-aligned numeric columns.
func maskAmountRight(s string, hidden bool) string {
	return maskAmountAligned(s, hidden, true)
}

func maskAmountAligned(s string, hidden, right bool) string {
	if !hidden {
		return s
	}
	if s == "" {
		return ""
	}
	value, unit := splitAmountUnit(s)
	masked := strings.Repeat("•", lipgloss.Width(value)) + unit
	if width := lipgloss.Width(s); right && lipgloss.Width(masked) < width {
		masked = strings.Repeat(" ", width-lipgloss.Width(masked)) + masked
	}
	return masked
}

// splitAmountUnit separates a formatted amount from its trailing unit suffix,
// keeping the separating space with the suffix.
func splitAmountUnit(s string) (value, unit string) {
	for _, suffix := range []string{" BTC", " sats"} {
		if strings.HasSuffix(s, suffix) {
			return strings.TrimSuffix(s, suffix), suffix
		}
	}
	return s, ""
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

package ui

import (
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/charmbracelet/lipgloss"
)

// FormatBitcoinAddressColored formats a Bitcoin address with the prefix and suffix
// in a highlighted color, and the middle section dimmed
// Example: tb1qABCDEF...XYZ
//
//	^^^^highlighted  ^^^highlighted
//	     ^^^^^^dimmed
func FormatBitcoinAddressColored(address string, prefixLen, suffixLen int) string {
	if len(address) <= prefixLen+suffixLen {
		// Address too short, just highlight it all
		highlightStyle := lipgloss.NewStyle().
			Foreground(styles.PastelOrange). // Pastel orange
			Bold(true)
		return highlightStyle.Render(address)
	}

	// Styles - PASTEL THEME
	highlightStyle := lipgloss.NewStyle().
		Foreground(styles.PastelOrange). // Pastel orange
		Bold(true)

	middleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#BAE1FF")) // Pastel blue

	// Split the address
	prefix := address[:prefixLen]
	suffix := address[len(address)-suffixLen:]
	middle := address[prefixLen : len(address)-suffixLen]

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#B0B0B0")) // Light gray

	return highlightStyle.Render(prefix) +
		separatorStyle.Render(" ") +
		middleStyle.Render(middle) +
		separatorStyle.Render(" ") +
		highlightStyle.Render(suffix)
}

// FormatBitcoinAddressCompact formats address showing only first/last chars
// Example: tb1qABCD...WXYZ
func FormatBitcoinAddressCompact(address string, prefixLen, suffixLen int) string {
	if len(address) <= prefixLen+suffixLen {
		return address
	}

	highlightStyle := lipgloss.NewStyle().
		Foreground(styles.PastelOrange). // Pastel orange
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#BAE1FF")) // Pastel blue

	prefix := address[:prefixLen]
	suffix := address[len(address)-suffixLen:]

	return highlightStyle.Render(prefix) +
		dimStyle.Render("...") +
		highlightStyle.Render(suffix)
}

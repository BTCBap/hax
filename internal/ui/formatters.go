package ui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/BTCBap/hax/internal/ui/styles"
)

const (
	addressGroupSize    = 8
	addressMinGroupSize = 3
)

var (
	addressHeadStyle      lipgloss.Style
	addressMiddleStyle    lipgloss.Style
	addressSeparatorStyle lipgloss.Style
)

func init() {
	rebuildAddressStyles()
	styles.OnRebuild(rebuildAddressStyles)
}

func rebuildAddressStyles() {
	addressHeadStyle = styles.AddressHeadStyle
	addressMiddleStyle = styles.AddressMiddleStyle
	addressSeparatorStyle = styles.AddressSeparatorStyle
}

// sanitizeAddress drops control characters so terminal escapes in an address
// cannot restyle the output.
func sanitizeAddress(address string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, address)
}

// FormatAddress renders an address with the receive screen's layout: the first
// and last eight characters highlighted, the middle plain, and one space
// between each section. A positive width limits the visible cells by shrinking
// the middle first, then the highlighted groups.
func FormatAddress(address string, width int) string {
	address = sanitizeAddress(address)
	n := len(address)
	if n == 0 {
		return ""
	}
	if n <= 2*addressGroupSize {
		if width > 0 && n > width {
			return formatAddressFallback(address, width)
		}
		return addressHeadStyle.Render(address)
	}

	middle := address[addressGroupSize : n-addressGroupSize]
	if width <= 0 || n+2 <= width {
		return formatAddressParts(address, addressGroupSize, addressGroupSize, middle)
	}

	if width >= 2*addressGroupSize+3 {
		keep := width - (2*addressGroupSize + 3)
		return formatAddressParts(address, addressGroupSize, addressGroupSize, middle[:keep]+"…")
	}

	if width >= 2*addressMinGroupSize+3 {
		total := width - 3
		tailLen := total / 2
		headLen := total - tailLen
		return formatAddressParts(address, headLen, tailLen, "…")
	}

	return formatAddressFallback(address, width)
}

// AddressWidth is the visible width FormatAddress uses for the same address
// and width. Callers use it to pad a row without measuring the ANSI output.
func AddressWidth(address string, width int) int {
	address = sanitizeAddress(address)
	n := len(address)
	if n == 0 {
		return 0
	}
	if n <= 2*addressGroupSize {
		if width > 0 && n > width {
			return width
		}
		return n
	}
	if width <= 0 || n+2 <= width {
		return n + 2
	}
	return width
}

func formatAddressParts(address string, headLen, tailLen int, middle string) string {
	head := address[:headLen]
	tail := address[len(address)-tailLen:]

	return addressHeadStyle.Render(head) +
		addressSeparatorStyle.Render(" ") +
		addressMiddleStyle.Render(middle) +
		addressSeparatorStyle.Render(" ") +
		addressHeadStyle.Render(tail)
}

// formatAddressFallback is the old compact form for widths too narrow for two
// highlighted groups with spaces: head, "...", tail, exactly width cells wide.
func formatAddressFallback(address string, width int) string {
	n := len(address)
	if width < 3 {
		if width > n {
			width = n
		}
		return addressHeadStyle.Render(address[:width])
	}

	total := width - 3
	tailLen := total / 2
	headLen := total - tailLen

	return addressHeadStyle.Render(address[:headLen]) +
		addressMiddleStyle.Render("...") +
		addressHeadStyle.Render(address[n-tailLen:])
}

// FormatBitcoinAddressColored formats a Bitcoin address with the prefix and suffix
// in a highlighted color, and the middle section dimmed
// Example: tb1qABCDEF...XYZ
//
//	^^^^highlighted  ^^^highlighted
//	     ^^^^^^dimmed
func FormatBitcoinAddressColored(address string, prefixLen, suffixLen int) string {
	address = sanitizeAddress(address)
	if len(address) <= prefixLen+suffixLen {
		// Address too short, just highlight it all
		return addressHeadStyle.Render(address)
	}

	return formatAddressParts(
		address,
		prefixLen,
		suffixLen,
		address[prefixLen:len(address)-suffixLen],
	)
}

// FormatBitcoinAddressCompact formats address showing only first/last chars
// Example: tb1qABCD...WXYZ
func FormatBitcoinAddressCompact(address string, prefixLen, suffixLen int) string {
	address = sanitizeAddress(address)
	if len(address) <= prefixLen+suffixLen {
		return address
	}

	return addressHeadStyle.Render(address[:prefixLen]) +
		addressMiddleStyle.Render("...") +
		addressHeadStyle.Render(address[len(address)-suffixLen:])
}

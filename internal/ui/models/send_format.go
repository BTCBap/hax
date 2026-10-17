package models

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui/styles"
)

func renderRow(label, value string) string {
	if label == "" {
		return "  " + styles.NormalText.Render(value)
	}
	return styles.DimText.Render(fmt.Sprintf("%-15s", label)) + " " +
		styles.HighlightText.Render(value)
}

func formatBTC(btc float64) string {
	return fmt.Sprintf("%.8f", btc)
}

// formatFeeRate formats a fee rate, showing decimals only if needed
// Examples: 1.0 -> "1", 0.5 -> "0.5", 1.5 -> "1.5"
func formatFeeRate(rate float64) string {
	if rate == float64(int64(rate)) {
		return fmt.Sprintf("%d", int64(rate))
	}
	// Remove trailing zeros but keep at least one decimal
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", rate), "0"), ".")
}

// formatSatsCompact formats satoshis in compact form with k suffix for thousands
// Examples: 1234 -> "1.2k", 50000 -> "50k", 999 -> "999"
func formatSatsCompact(sats api.Satoshi) string {
	if sats < 1000 {
		return fmt.Sprintf("%d", sats)
	}
	if sats < 10000 {
		// Show one decimal place for values 1k-10k
		return fmt.Sprintf("%.1fk", float64(sats)/1000.0)
	}
	// No decimal for 10k+
	return fmt.Sprintf("%.0fk", float64(sats)/1000.0)
}

// formatBTCWithSats formats amount as "0.00012345 BTC (12.3k sats)" with colored styling
// Amount: pastel blue, "BTC": pastel orange, sats: grey
func formatBTCWithSats(sats api.Satoshi) string {
	btc := sats.ToBTC()
	amountStr := styles.AmountStyle.Render(formatBTC(btc))
	btcLabel := styles.CurrencyUnitStyle.Render("BTC")
	satsStr := styles.DimText.Render(fmt.Sprintf("(%s sats)", formatSatsCompact(sats)))
	return fmt.Sprintf("%s %s %s", amountStr, btcLabel, satsStr)
}

// validateBTCAmount validates Bitcoin amount input (max 8 decimal places)
func validateBTCAmount(s string) error {
	if s == "" {
		return nil // Empty is OK while typing
	}

	// Only allow numbers and one decimal point
	if !regexp.MustCompile(`^[0-9]*\.?[0-9]*$`).MatchString(s) {
		return fmt.Errorf("only numbers and decimal point allowed")
	}

	// Check decimal places (max 8 for Bitcoin)
	if strings.Contains(s, ".") {
		parts := strings.Split(s, ".")
		if len(parts) > 2 {
			return fmt.Errorf("only one decimal point allowed")
		}
		if len(parts[1]) > 8 {
			return fmt.Errorf("maximum 8 decimal places (satoshi precision)")
		}
	}

	return nil
}

// validateSatPerVB validates custom fee rate input (allows decimals for sub-1 sat/vB)
func validateSatPerVB(s string) error {
	if s == "" {
		return nil // Empty is OK while typing
	}

	// Allow integers and decimals (e.g., "1", "0.5", "0.1")
	if !regexp.MustCompile(`^[0-9]+\.?[0-9]*$`).MatchString(s) {
		return fmt.Errorf("invalid number format")
	}

	return nil
}

// pluralize returns "s" if count != 1, otherwise empty string
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func pluralVerb(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

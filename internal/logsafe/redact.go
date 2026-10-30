// Package logsafe redacts wallet-sensitive values from diagnostic output.
package logsafe

import "regexp"

var (
	hexPattern      = regexp.MustCompile(`\b[0-9a-fA-F]{16,}\b`)
	bech32Pattern   = regexp.MustCompile(`\b(?:bc1|tb1|bcrt1)[a-z0-9]{8,}\b`)
	base58Pattern   = regexp.MustCompile(`\b[13][1-9A-HJ-NP-Za-km-z]{25,40}\b`)
	derivationPath  = regexp.MustCompile(`\bm(?:/\d+'?)+`)
	urlWithPassword = regexp.MustCompile(`(?i)(://[^:/\s]+:)[^@\s]+@`)
)

// Redact shortens addresses, transaction ids, extended keys, and derivation
// paths so a debug log does not record the wallet's full history.
func Redact(s string) string {
	s = urlWithPassword.ReplaceAllString(s, "$1***@")
	s = bech32Pattern.ReplaceAllStringFunc(s, shorten(10))
	s = base58Pattern.ReplaceAllStringFunc(s, shorten(6))
	s = hexPattern.ReplaceAllStringFunc(s, shorten(8))
	s = derivationPath.ReplaceAllString(s, "m/…")
	return s
}

func shorten(prefix int) func(string) string {
	return func(v string) string {
		if len(v) <= prefix {
			return v
		}
		return v[:prefix] + "…"
	}
}

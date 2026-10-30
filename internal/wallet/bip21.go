package wallet

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/BTCBap/hax/internal/api"
)

// PaymentURI is a parsed BIP21 "bitcoin:" payment request.
type PaymentURI struct {
	Address string
	Amount  api.Satoshi // 0 when the URI has no amount
	Label   string
	Message string
}

// maxURILen bounds what we try to parse from a paste.
const maxURILen = 2048

// IsBIP21 reports whether s looks like a bitcoin: URI.
func IsBIP21(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 8 && strings.EqualFold(s[:8], "bitcoin:")
}

// ParseBIP21 parses a BIP21 payment URI and checks the address against
// network. Per BIP21, unknown "req-" parameters make the URI invalid;
// other unknown parameters (lightning=, pj=, ...) are ignored.
func ParseBIP21(raw, network string) (*PaymentURI, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxURILen {
		return nil, fmt.Errorf("payment URI is too long")
	}
	if !IsBIP21(raw) {
		return nil, fmt.Errorf("not a bitcoin: URI")
	}
	rest := raw[len("bitcoin:"):]
	rest = strings.TrimPrefix(rest, "//") // tolerate bitcoin://addr from some apps
	addrPart, query, _ := strings.Cut(rest, "?")

	addr, err := url.PathUnescape(addrPart)
	if err != nil {
		return nil, fmt.Errorf("invalid address in URI: %w", err)
	}
	if addr == "" {
		return nil, fmt.Errorf("payment URI has no address")
	}
	// QR codes often carry bech32 in upper case for compactness.
	if isUpper(addr) {
		addr = strings.ToLower(addr)
	}
	if err := ValidateAddressForNetwork(addr, network); err != nil {
		return nil, err
	}

	p := &PaymentURI{Address: addr}
	seen := map[string]bool{}
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		key := strings.ToLower(k)
		val, err := url.QueryUnescape(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %q parameter: %w", k, err)
		}
		if seen[key] {
			return nil, fmt.Errorf("payment URI repeats %q", k)
		}
		seen[key] = true
		switch key {
		case "amount":
			sats, err := api.ParseBTC(val)
			if err != nil {
				return nil, fmt.Errorf("invalid amount in URI: %w", err)
			}
			p.Amount = sats
		case "label":
			p.Label = sanitizeURIText(val)
		case "message":
			p.Message = sanitizeURIText(val)
		default:
			if strings.HasPrefix(key, "req-") {
				return nil, fmt.Errorf("payment URI requires %q, which hax does not support", k)
			}
		}
	}
	return p, nil
}

func isUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

// sanitizeURIText drops control and Unicode format characters (terminal
// escapes, bidirectional overrides and isolates) from labels and messages
// and caps their length.
func sanitizeURIText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 255 {
		s = string(r[:255])
	}
	return s
}

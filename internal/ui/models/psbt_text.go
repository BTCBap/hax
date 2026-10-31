package models

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/BTCBap/hax/internal/ur"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

// maxPastedPSBTLen bounds pasted PSBT text so a paste cannot grow without
// limit. It matches the base64 decoding limit with room to spare.
const maxPastedPSBTLen = 16 << 20

// errNotPSBT is the only decode error text. It never includes the input, so
// pasted PSBT data cannot leak into an error message.
const errNotPSBT = "not a valid PSBT (base64, hex or ur:crypto-psbt)"

// stripPSBTWhitespace removes every whitespace rune, including the line
// wraps that terminals and clipboard sources insert into long values.
func stripPSBTWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// looksLikePSBTText reports whether pasted text is plausibly a PSBT in
// base64, hex, or UR form, ignoring whitespace and UR casing.
func looksLikePSBTText(s string) bool {
	clean := stripPSBTWhitespace(s)
	if clean == "" {
		return false
	}
	if strings.HasPrefix(clean, "cHNidP8") {
		return true
	}
	lower := strings.ToLower(clean)
	return strings.HasPrefix(lower, "70736274ff") ||
		strings.HasPrefix(lower, "ur:crypto-psbt/") ||
		strings.HasPrefix(lower, "ur:psbt/")
}

// decodePSBTText normalizes pasted PSBT text to base64. It accepts base64,
// hex, and ur:crypto-psbt (or ur:psbt) encodings, with or without whitespace.
func decodePSBTText(s string) (string, error) {
	clean := stripPSBTWhitespace(s)
	if clean == "" {
		return "", errors.New("no PSBT found in pasted text")
	}
	if len(clean) > maxPastedPSBTLen {
		return "", errors.New("PSBT is too large")
	}

	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "ur:crypto-psbt/") || strings.HasPrefix(lower, "ur:psbt/") {
		psbtBytes, err := ur.DecodePSBTFromUR(clean)
		if err != nil {
			return "", errors.New(errNotPSBT)
		}
		return base64.StdEncoding.EncodeToString(psbtBytes), nil
	}

	if strings.HasPrefix(lower, "70736274ff") {
		raw, err := hex.DecodeString(clean)
		if err != nil {
			return "", errors.New(errNotPSBT)
		}
		return base64.StdEncoding.EncodeToString(raw), nil
	}

	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil || !bytes.HasPrefix(raw, []byte("psbt\xff")) {
		return "", errors.New(errNotPSBT)
	}
	return clean, nil
}

// startPSBTImport decodes pasted text and returns the import command. On
// decode failure it records the error and reports false so the caller can
// keep its current screen open.
func (m *SendModel) startPSBTImport(text string) (bool, tea.Cmd) {
	b64, err := decodePSBTText(text)
	if err != nil {
		m.err = err
		return false, nil
	}
	m.err = nil
	m.loading = true
	return true, m.importPSBTBase64(b64)
}

// importPSBTBase64 parses a base64 PSBT through the wallet and reports it as
// a PSBTFileMsg import, the same path file imports use.
func (m SendModel) importPSBTBase64(b64 string) tea.Cmd {
	walletMgr := m.walletManager

	return func() tea.Msg {
		if walletMgr == nil {
			return PSBTFileMsg{Operation: "import", Err: fmt.Errorf("wallet is not available")}
		}

		result, err := walletMgr.ImportPSBTFromBase64(b64)
		if err != nil {
			return PSBTFileMsg{Operation: "import", Err: fmt.Errorf("invalid PSBT: %w", err)}
		}

		return PSBTFileMsg{
			Operation: "import",
			FilePath:  "",
			Base64:    b64,
			Result:    result,
			SigsAdded: wallet.CountPSBTSigners(result.Packet),
		}
	}
}

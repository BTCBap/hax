package ur

import (
	"fmt"
	"strings"
)

// EncodePSBTAsUR encodes raw PSBT bytes as a UR string.
// Format: ur:crypto-psbt/<bytewords-with-checksum>
// The CBOR payload is simply a byte string wrapping the raw PSBT.
func EncodePSBTAsUR(psbtBytes []byte) string {
	var buf cborBuf
	buf.writeBytes(psbtBytes)
	return EncodeSinglePart("crypto-psbt", buf.data)
}

// DecodePSBTFromUR decodes a ur:crypto-psbt (or ur:psbt) string back to raw PSBT bytes.
func DecodePSBTFromUR(urString string) ([]byte, error) {
	urString = strings.TrimSpace(strings.ToLower(urString))

	// Accept both legacy "crypto-psbt" and modern "psbt" type names
	var encoded string
	switch {
	case strings.HasPrefix(urString, "ur:crypto-psbt/"):
		encoded = strings.TrimPrefix(urString, "ur:crypto-psbt/")
	case strings.HasPrefix(urString, "ur:psbt/"):
		encoded = strings.TrimPrefix(urString, "ur:psbt/")
	default:
		return nil, fmt.Errorf("not a PSBT UR string (expected ur:crypto-psbt/ or ur:psbt/ prefix)")
	}

	// Decode bytewords and verify checksum
	cborData, err := DecodeSinglePart(encoded)
	if err != nil {
		return nil, fmt.Errorf("UR decode: %w", err)
	}

	// Extract PSBT bytes from CBOR byte string
	psbtBytes, err := decodeCBORByteString(cborData)
	if err != nil {
		return nil, fmt.Errorf("CBOR decode: %w", err)
	}

	return psbtBytes, nil
}

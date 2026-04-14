// Package bbqr implements the BBQr encoding format for QR-based data exchange.
// BBQr is used by Coldcard Q for PSBT transfer via animated QR codes.
// Specification: https://github.com/coinkite/BBQr/blob/master/BBQr.md
package bbqr

import (
	"bytes"
	"compress/zlib"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

// File type constants
const (
	FileTypePSBT        = "P"
	FileTypeTransaction = "T"
	FileTypeJSON        = "J"
	FileTypeUnicode     = "U"
	FileTypeBinary      = "B"
)

// Encoding constants
const (
	EncodingHex  = 'H' // Uppercase hex
	EncodingZlib = 'Z' // Zlib + base32
)

// QR alphanumeric capacity at Low error correction level.
// Source: ISO/IEC 18004, Table 7.
var qrAlphanumericCapacity = map[int]int{
	5: 154, 6: 195, 7: 224, 8: 279, 9: 335, 10: 395,
	11: 468, 12: 535, 13: 619, 14: 667, 15: 758,
	16: 854, 17: 938, 18: 1046, 19: 1153, 20: 1249,
	21: 1352, 22: 1460, 23: 1588, 24: 1704, 25: 1853,
	26: 1990, 27: 2132, 28: 2223, 29: 2369,
}

const headerLen = 8 // "B$" + encoding(1) + filetype(1) + total(2) + seq(2)

// Encode encodes data as BBQr frames using zlib compression.
// Returns a slice of strings, each being the full content for one QR code.
func Encode(data []byte, fileType string) ([]string, error) {
	return EncodeWithOptions(data, fileType, EncodingZlib, 0)
}

// EncodeHex encodes data as BBQr frames using hex encoding (simpler, no compression).
func EncodeHex(data []byte, fileType string) ([]string, error) {
	return EncodeWithOptions(data, fileType, EncodingHex, 0)
}

// EncodeWithOptions encodes data with configurable encoding and target QR version.
// If targetVersion is 0, the smallest version that minimizes frame count is chosen.
func EncodeWithOptions(data []byte, fileType string, encoding byte, targetVersion int) ([]string, error) {
	if len(fileType) != 1 {
		return nil, fmt.Errorf("file type must be exactly 1 character, got %q", fileType)
	}

	// Encode the data
	var encoded string
	switch encoding {
	case EncodingHex:
		encoded = strings.ToUpper(hex.EncodeToString(data))
	case EncodingZlib:
		compressed, err := zlibCompress(data)
		if err != nil {
			return nil, fmt.Errorf("zlib compression: %w", err)
		}
		encoded = strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(compressed))
	default:
		return nil, fmt.Errorf("unsupported encoding: %c", encoding)
	}

	// Find optimal QR version and part count
	version, numParts := findOptimalSplit(len(encoded), targetVersion)
	if numParts < 1 {
		return nil, fmt.Errorf("data too large for BBQr encoding")
	}
	_ = version // used implicitly through capacity

	// Split encoded data into parts
	dataPerPart := (len(encoded) + numParts - 1) / numParts
	frames := make([]string, numParts)

	for i := 0; i < numParts; i++ {
		start := i * dataPerPart
		end := start + dataPerPart
		if end > len(encoded) {
			end = len(encoded)
		}

		header := fmt.Sprintf("B$%c%s%s%s",
			encoding,
			fileType,
			toBase36Pair(numParts),
			toBase36Pair(i),
		)
		frames[i] = header + encoded[start:end]
	}

	return frames, nil
}

// findOptimalSplit finds the best QR version and number of parts.
// Returns (version, numParts). Prefers fewer parts (larger QR codes up to reasonable size).
func findOptimalSplit(encodedLen int, targetVersion int) (int, int) {
	if targetVersion > 0 {
		cap, ok := qrAlphanumericCapacity[targetVersion]
		if !ok {
			return 0, 0
		}
		dataPerQR := cap - headerLen
		if dataPerQR <= 0 {
			return 0, 0
		}
		numParts := (encodedLen + dataPerQR - 1) / dataPerQR
		if numParts > 1295 { // max ZZ in base-36
			return 0, 0
		}
		return targetVersion, numParts
	}

	// Try each version from small to large, pick the one with fewest parts
	// that keeps QR codes at a reasonable terminal size (≤ version 18 preferred)
	bestVersion := 0
	bestParts := 0

	for v := 5; v <= 29; v++ {
		cap := qrAlphanumericCapacity[v]
		dataPerQR := cap - headerLen
		if dataPerQR <= 0 {
			continue
		}

		numParts := (encodedLen + dataPerQR - 1) / dataPerQR
		if numParts > 1295 {
			continue
		}

		if bestParts == 0 || numParts < bestParts {
			bestVersion = v
			bestParts = numParts
		}

		// If we can fit in a single QR, use this version
		if numParts == 1 {
			break
		}

		// Once we find a version that fits in ≤3 parts, prefer it
		// to avoid unnecessarily large QR codes
		if numParts <= 3 && v >= 10 {
			break
		}
	}

	return bestVersion, bestParts
}

// toBase36Pair encodes an integer (0-1295) as a 2-character base-36 string.
func toBase36Pair(n int) string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if n < 0 || n > 1295 {
		return "00"
	}
	return string([]byte{digits[n/36], digits[n%36]})
}

func zlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

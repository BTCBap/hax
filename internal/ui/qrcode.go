package ui

import (
	"strings"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCodeHalfBlocks generates a QR code using half-height blocks (more compact).
// Uses Low error correction and no border to minimize size for terminal display.
func GenerateQRCodeHalfBlocks(data string) (string, error) {
	qr, err := qrcode.New(data, qrcode.Low)
	if err != nil {
		return "", err
	}
	qr.DisableBorder = true

	bitmap := qr.Bitmap()
	var result strings.Builder

	// Process two rows at a time using upper/lower half blocks
	for y := 0; y < len(bitmap); y += 2 {
		for x := range bitmap[y] {
			top := bitmap[y][x]
			bottom := false
			if y+1 < len(bitmap) {
				bottom = bitmap[y+1][x]
			}

			// Choose the right character based on top/bottom
			if top && bottom {
				result.WriteString("█") // Full block
			} else if top && !bottom {
				result.WriteString("▀") // Upper half block
			} else if !top && bottom {
				result.WriteString("▄") // Lower half block
			} else {
				result.WriteString(" ") // Empty
			}
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}

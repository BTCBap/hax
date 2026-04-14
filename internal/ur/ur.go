package ur

import (
	"fmt"
	"hash/crc32"
	"strings"
)

// EncodeDescriptorAsUR encodes a DescriptorInfo as a complete UR string.
// Returns a string like "ur:crypto-output/taadm..." that can be rendered as a QR code.
func EncodeDescriptorAsUR(info DescriptorInfo) (string, error) {
	cborData, err := EncodeCryptoOutput(info)
	if err != nil {
		return "", fmt.Errorf("CBOR encoding failed: %w", err)
	}
	return EncodeSinglePart("crypto-output", cborData), nil
}

// EncodeSinglePart wraps CBOR data into a single-part UR string.
// Format: ur:<type>/<bytewords-minimal-with-checksum>
func EncodeSinglePart(urType string, cborData []byte) string {
	// Append CRC32 checksum (IEEE polynomial, big-endian)
	checksum := crc32.ChecksumIEEE(cborData)
	withChecksum := make([]byte, len(cborData)+4)
	copy(withChecksum, cborData)
	withChecksum[len(cborData)] = byte(checksum >> 24)
	withChecksum[len(cborData)+1] = byte(checksum >> 16)
	withChecksum[len(cborData)+2] = byte(checksum >> 8)
	withChecksum[len(cborData)+3] = byte(checksum)

	encoded := EncodeMinimal(withChecksum)
	return fmt.Sprintf("ur:%s/%s", urType, strings.ToLower(encoded))
}

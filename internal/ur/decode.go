package ur

import (
	"fmt"
	"hash/crc32"
)

// bytewordsReverse maps 2-char minimal encoding back to byte value.
var bytewordsReverse map[string]byte

func init() {
	bytewordsReverse = make(map[string]byte, 256)
	for i, word := range bytewordsTable {
		minimal := string([]byte{word[0], word[len(word)-1]})
		bytewordsReverse[minimal] = byte(i)
	}
}

// DecodeMinimal decodes minimal bytewords (first+last letter pairs) back to bytes.
func DecodeMinimal(encoded string) ([]byte, error) {
	if len(encoded)%2 != 0 {
		return nil, fmt.Errorf("minimal bytewords must have even length, got %d", len(encoded))
	}

	result := make([]byte, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		pair := encoded[i : i+2]
		b, ok := bytewordsReverse[pair]
		if !ok {
			return nil, fmt.Errorf("invalid bytewords pair at position %d: %q", i, pair)
		}
		result[i/2] = b
	}
	return result, nil
}

// DecodeSinglePart decodes a single-part UR payload (bytewords with CRC32 checksum).
// Returns the raw CBOR data after verifying the checksum.
func DecodeSinglePart(encoded string) ([]byte, error) {
	data, err := DecodeMinimal(encoded)
	if err != nil {
		return nil, fmt.Errorf("bytewords decode: %w", err)
	}

	if len(data) < 5 {
		return nil, fmt.Errorf("UR payload too short (%d bytes)", len(data))
	}

	// Split payload and checksum (last 4 bytes)
	payload := data[:len(data)-4]
	checksumBytes := data[len(data)-4:]

	expected := uint32(checksumBytes[0])<<24 | uint32(checksumBytes[1])<<16 |
		uint32(checksumBytes[2])<<8 | uint32(checksumBytes[3])
	actual := crc32.ChecksumIEEE(payload)

	if actual != expected {
		return nil, fmt.Errorf("CRC32 checksum mismatch: expected %08x, got %08x", expected, actual)
	}

	return payload, nil
}

// decodeCBORByteString extracts a byte string from CBOR major type 2 encoding.
func decodeCBORByteString(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty CBOR data")
	}

	major := data[0] & 0xe0
	if major != cborBytes {
		return nil, fmt.Errorf("expected CBOR byte string (major type 2), got major type %d", major>>5)
	}

	additional := data[0] & 0x1f
	var length uint64
	var headerLen int

	switch {
	case additional <= 23:
		length = uint64(additional)
		headerLen = 1
	case additional == 24:
		if len(data) < 2 {
			return nil, fmt.Errorf("truncated CBOR header")
		}
		length = uint64(data[1])
		headerLen = 2
	case additional == 25:
		if len(data) < 3 {
			return nil, fmt.Errorf("truncated CBOR header")
		}
		length = uint64(data[1])<<8 | uint64(data[2])
		headerLen = 3
	case additional == 26:
		if len(data) < 5 {
			return nil, fmt.Errorf("truncated CBOR header")
		}
		length = uint64(data[1])<<24 | uint64(data[2])<<16 | uint64(data[3])<<8 | uint64(data[4])
		headerLen = 5
	case additional == 27:
		if len(data) < 9 {
			return nil, fmt.Errorf("truncated CBOR header")
		}
		length = uint64(data[1])<<56 | uint64(data[2])<<48 | uint64(data[3])<<40 | uint64(data[4])<<32 |
			uint64(data[5])<<24 | uint64(data[6])<<16 | uint64(data[7])<<8 | uint64(data[8])
		headerLen = 9
	default:
		return nil, fmt.Errorf("unsupported CBOR additional info: %d", additional)
	}

	if uint64(len(data)-headerLen) < length {
		return nil, fmt.Errorf("CBOR byte string length %d exceeds available data %d", length, len(data)-headerLen)
	}

	return data[headerLen : headerLen+int(length)], nil
}

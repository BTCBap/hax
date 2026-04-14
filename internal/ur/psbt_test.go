package ur

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestEncodePSBTAsUR(t *testing.T) {
	// Simple known input
	psbtBytes := []byte{0x70, 0x73, 0x62, 0x74} // "psbt" magic bytes
	result := EncodePSBTAsUR(psbtBytes)

	if !strings.HasPrefix(result, "ur:crypto-psbt/") {
		t.Errorf("expected ur:crypto-psbt/ prefix, got %s", result)
	}

	// Should be lowercase
	if result != strings.ToLower(result) {
		t.Errorf("expected lowercase UR string, got %s", result)
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"psbt magic", []byte{0x70, 0x73, 0x62, 0x74, 0xff}},
		{"256 bytes", make([]byte, 256)},
	}

	// Fill 256-byte test with sequential values
	for i := range tests[3].data {
		tests[3].data[i] = byte(i)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodePSBTAsUR(tt.data)
			decoded, err := DecodePSBTFromUR(encoded)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if !bytes.Equal(decoded, tt.data) {
				t.Errorf("round-trip mismatch: got %d bytes, want %d bytes", len(decoded), len(tt.data))
			}
		})
	}
}

func TestRoundTripRandomData(t *testing.T) {
	// Test with random data of various sizes
	sizes := []int{1, 10, 100, 500, 1000, 2000}
	for _, size := range sizes {
		data := make([]byte, size)
		if _, err := rand.Read(data); err != nil {
			t.Fatalf("rand.Read failed: %v", err)
		}

		encoded := EncodePSBTAsUR(data)
		decoded, err := DecodePSBTFromUR(encoded)
		if err != nil {
			t.Fatalf("decode failed for %d bytes: %v", size, err)
		}
		if !bytes.Equal(decoded, data) {
			t.Errorf("round-trip mismatch for %d bytes", size)
		}
	}
}

func TestDecodePSBTFromUR_AcceptsBothPrefixes(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	encoded := EncodePSBTAsUR(data)

	// Should work with crypto-psbt prefix
	decoded, err := DecodePSBTFromUR(encoded)
	if err != nil {
		t.Fatalf("crypto-psbt decode failed: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Error("crypto-psbt round-trip mismatch")
	}

	// Should also work with psbt prefix
	modernPrefix := strings.Replace(encoded, "ur:crypto-psbt/", "ur:psbt/", 1)
	decoded, err = DecodePSBTFromUR(modernPrefix)
	if err != nil {
		t.Fatalf("psbt decode failed: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Error("psbt round-trip mismatch")
	}
}

func TestDecodePSBTFromUR_CaseInsensitive(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded := EncodePSBTAsUR(data)

	// Uppercase should also decode
	upper := strings.ToUpper(encoded)
	decoded, err := DecodePSBTFromUR(upper)
	if err != nil {
		t.Fatalf("uppercase decode failed: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Error("uppercase round-trip mismatch")
	}
}

func TestDecodePSBTFromUR_InvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"wrong prefix", "ur:crypto-output/aeadae"},
		{"no prefix", "just-some-text"},
		{"empty payload", "ur:crypto-psbt/"},
		{"odd length payload", "ur:crypto-psbt/abc"},
		{"invalid bytewords", "ur:crypto-psbt/zz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodePSBTFromUR(tt.input)
			if err == nil {
				t.Errorf("expected error for input %q", tt.input)
			}
		})
	}
}

func TestDecodeMinimal_RoundTrip(t *testing.T) {
	// Every byte value should round-trip through encode/decode
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	encoded := EncodeMinimal(allBytes)
	decoded, err := DecodeMinimal(encoded)
	if err != nil {
		t.Fatalf("DecodeMinimal failed: %v", err)
	}
	if !bytes.Equal(decoded, allBytes) {
		t.Error("all-bytes round-trip mismatch")
	}
}

func TestDecodeCBORByteString(t *testing.T) {
	tests := []struct {
		name string
		cbor []byte
		want []byte
	}{
		{"tiny (len<=23)", []byte{0x44, 0x01, 0x02, 0x03, 0x04}, []byte{0x01, 0x02, 0x03, 0x04}},
		{"empty", []byte{0x40}, []byte{}},
		{"1-byte length", append([]byte{0x58, 0x04}, []byte{0x01, 0x02, 0x03, 0x04}...), []byte{0x01, 0x02, 0x03, 0x04}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeCBORByteString(tt.cbor)
			if err != nil {
				t.Fatalf("decodeCBORByteString failed: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("got %x, want %x", got, tt.want)
			}
		})
	}
}

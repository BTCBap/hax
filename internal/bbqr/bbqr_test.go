package bbqr

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestEncodeHex_SingleFrame(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frames, err := EncodeHex(data, FileTypePSBT)
	if err != nil {
		t.Fatalf("EncodeHex failed: %v", err)
	}

	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}

	frame := frames[0]

	// Check header structure
	if !strings.HasPrefix(frame, "B$HP") {
		t.Errorf("expected B$HP prefix, got %s", frame[:4])
	}

	// Header: B$HP0100 (1 total, index 0)
	if frame[:8] != "B$HP0100" {
		t.Errorf("expected header B$HP0100, got %s", frame[:8])
	}

	// Data should be uppercase hex
	if frame[8:] != "DEADBEEF" {
		t.Errorf("expected DEADBEEF, got %s", frame[8:])
	}
}

func TestEncodeZlib_SingleFrame(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frames, err := Encode(data, FileTypePSBT)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}

	frame := frames[0]

	// Check header: B$ZP (zlib, PSBT)
	if !strings.HasPrefix(frame, "B$ZP") {
		t.Errorf("expected B$ZP prefix, got %s", frame[:4])
	}

	// Total should be 01, index 00
	total := frame[4:6]
	index := frame[6:8]
	if total != "01" {
		t.Errorf("expected total 01, got %s", total)
	}
	if index != "00" {
		t.Errorf("expected index 00, got %s", index)
	}
}

func TestEncodeHex_MultiFrame(t *testing.T) {
	// Create data large enough to require multiple frames at a small QR version
	data := make([]byte, 2000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	frames, err := EncodeWithOptions(data, FileTypePSBT, EncodingHex, 10)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(frames) < 2 {
		t.Fatalf("expected multiple frames, got %d", len(frames))
	}

	// Check all frames have correct headers
	for i, frame := range frames {
		if !strings.HasPrefix(frame, "B$HP") {
			t.Errorf("frame %d: expected B$HP prefix", i)
		}

		// Verify frame index in header
		total := frame[4:6]
		idx := frame[6:8]

		expectedTotal := toBase36Pair(len(frames))
		expectedIdx := toBase36Pair(i)

		if total != expectedTotal {
			t.Errorf("frame %d: expected total %s, got %s", i, expectedTotal, total)
		}
		if idx != expectedIdx {
			t.Errorf("frame %d: expected index %s, got %s", i, expectedIdx, idx)
		}
	}

	// Reconstruct data from frames (strip headers, join, hex decode)
	var allHex string
	for _, frame := range frames {
		allHex += frame[8:]
	}
	if len(allHex) != len(data)*2 {
		t.Errorf("total hex length %d, expected %d", len(allHex), len(data)*2)
	}
}

func TestToBase36Pair(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{1, "01"},
		{9, "09"},
		{10, "0A"},
		{35, "0Z"},
		{36, "10"},
		{99, "2R"},
		{1295, "ZZ"},
	}

	for _, tt := range tests {
		got := toBase36Pair(tt.n)
		if got != tt.want {
			t.Errorf("toBase36Pair(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestEncodeWithOptions_InvalidFileType(t *testing.T) {
	_, err := EncodeWithOptions([]byte{0x01}, "PP", EncodingHex, 0)
	if err == nil {
		t.Error("expected error for multi-char file type")
	}
}

func TestEncode_LargeRandom(t *testing.T) {
	// Ensure large data doesn't panic
	data := make([]byte, 5000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	frames, err := Encode(data, FileTypePSBT)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(frames) == 0 {
		t.Error("expected at least 1 frame")
	}

	// All frames should have valid headers
	for i, frame := range frames {
		if len(frame) < 8 {
			t.Errorf("frame %d too short: %d", i, len(frame))
		}
		if !strings.HasPrefix(frame, "B$ZP") {
			t.Errorf("frame %d: bad prefix", i)
		}
	}
}

func TestZlibCompression_SmallerThanHex(t *testing.T) {
	// Repetitive data should compress well
	data := make([]byte, 500)
	for i := range data {
		data[i] = 0xAA
	}

	hexFrames, err := EncodeHex(data, FileTypePSBT)
	if err != nil {
		t.Fatalf("EncodeHex failed: %v", err)
	}

	zlibFrames, err := Encode(data, FileTypePSBT)
	if err != nil {
		t.Fatalf("Encode (zlib) failed: %v", err)
	}

	hexTotal := 0
	for _, f := range hexFrames {
		hexTotal += len(f)
	}

	zlibTotal := 0
	for _, f := range zlibFrames {
		zlibTotal += len(f)
	}

	if zlibTotal >= hexTotal {
		t.Errorf("zlib (%d bytes) should be smaller than hex (%d bytes) for repetitive data", zlibTotal, hexTotal)
	}
}

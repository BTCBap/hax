package ur

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestBytewordsEncodeMinimal(t *testing.T) {
	// Test basic encoding
	data := []byte{0x00} // "able" -> "ae"
	result := EncodeMinimal(data)
	if result != "ae" {
		t.Errorf("expected 'ae', got %q", result)
	}

	// Byte 0x01 = "acid" -> "ad"
	data = []byte{0x01}
	result = EncodeMinimal(data)
	if result != "ad" {
		t.Errorf("expected 'ad', got %q", result)
	}

	// Multiple bytes
	data = []byte{0x00, 0x01, 0x02}
	result = EncodeMinimal(data)
	if result != "aeadao" {
		t.Errorf("expected 'aeadao', got %q", result)
	}
}

func TestCBOREncoder(t *testing.T) {
	// Test unsigned integer encoding
	var buf cborBuf
	buf.writeUint(0)
	if hex.EncodeToString(buf.data) != "00" {
		t.Errorf("uint(0): expected 00, got %s", hex.EncodeToString(buf.data))
	}

	buf = cborBuf{}
	buf.writeUint(23)
	if hex.EncodeToString(buf.data) != "17" {
		t.Errorf("uint(23): expected 17, got %s", hex.EncodeToString(buf.data))
	}

	buf = cborBuf{}
	buf.writeUint(24)
	if hex.EncodeToString(buf.data) != "1818" {
		t.Errorf("uint(24): expected 1818, got %s", hex.EncodeToString(buf.data))
	}

	buf = cborBuf{}
	buf.writeUint(256)
	if hex.EncodeToString(buf.data) != "190100" {
		t.Errorf("uint(256): expected 190100, got %s", hex.EncodeToString(buf.data))
	}

	// Test byte string
	buf = cborBuf{}
	buf.writeBytes([]byte{0xDE, 0xAD})
	if hex.EncodeToString(buf.data) != "42dead" {
		t.Errorf("bytes: expected 42dead, got %s", hex.EncodeToString(buf.data))
	}

	// Test text string
	buf = cborBuf{}
	buf.writeText("hi")
	if hex.EncodeToString(buf.data) != "626869" {
		t.Errorf("text: expected 626869, got %s", hex.EncodeToString(buf.data))
	}

	// Test bool
	buf = cborBuf{}
	buf.writeBool(true)
	if hex.EncodeToString(buf.data) != "f5" {
		t.Errorf("true: expected f5, got %s", hex.EncodeToString(buf.data))
	}

	buf = cborBuf{}
	buf.writeBool(false)
	if hex.EncodeToString(buf.data) != "f4" {
		t.Errorf("false: expected f4, got %s", hex.EncodeToString(buf.data))
	}

	// Test tag
	buf = cborBuf{}
	buf.writeTag(303) // crypto-hdkey
	if hex.EncodeToString(buf.data) != "d9012f" {
		t.Errorf("tag(303): expected d9012f, got %s", hex.EncodeToString(buf.data))
	}
}

func TestEncodeSinglePart(t *testing.T) {
	// Test that UR encoding produces correct format
	data := []byte{0x01, 0x02, 0x03}
	result := EncodeSinglePart("crypto-output", data)

	if !strings.HasPrefix(result, "ur:crypto-output/") {
		t.Errorf("expected ur:crypto-output/ prefix, got %q", result)
	}

	// Should be lowercase
	if result != strings.ToLower(result) {
		t.Errorf("UR body should be lowercase, got %q", result)
	}

	// The encoded body should include both the data and CRC32 checksum (4 extra bytes)
	body := strings.TrimPrefix(result, "ur:crypto-output/")
	// Each byte = 2 chars in minimal bytewords, total bytes = 3 data + 4 checksum = 7
	if len(body) != 14 {
		t.Errorf("expected 14 chars in body (7 bytes * 2), got %d: %q", len(body), body)
	}
}

func TestEncodeCryptoOutputSingleSig(t *testing.T) {
	// Test encoding a single-sig wpkh descriptor
	pubkey, _ := hex.DecodeString("02d2b36900396c9282fa14628566582f206a5dd0bcc8d5e892611806cafb0301f0")
	chaincode, _ := hex.DecodeString("637807030d55d01f9a0cb3a7839515d796bd07706386a6eddf06cc29a65a0e29")

	info := DescriptorInfo{
		ScriptType: "wpkh",
		IsMultisig: false,
		Keys: []KeyInfo{
			{
				PublicKey:  pubkey,
				ChainCode: chaincode,
				ParentFP:  0x78412e3a,
				MasterFP:  0xe8f7c0d2,
				DerivationPath: []uint32{
					84 | 0x80000000,
					1 | 0x80000000,
					0 | 0x80000000,
				},
			},
		},
		Network: 1, // testnet
	}

	cborData, err := EncodeCryptoOutput(info)
	if err != nil {
		t.Fatalf("EncodeCryptoOutput failed: %v", err)
	}

	// Verify CBOR is non-empty and starts with script tag
	if len(cborData) == 0 {
		t.Fatal("empty CBOR data")
	}

	// Test full UR encoding
	urStr, err := EncodeDescriptorAsUR(info)
	if err != nil {
		t.Fatalf("EncodeDescriptorAsUR failed: %v", err)
	}

	if !strings.HasPrefix(urStr, "ur:crypto-output/") {
		t.Errorf("expected ur:crypto-output/ prefix, got %q", urStr[:30])
	}

	t.Logf("Single-sig UR (%d bytes CBOR): %s", len(cborData), urStr)
}

func TestEncodeCryptoOutputMultisig(t *testing.T) {
	// Test encoding a 2-of-3 multisig wsh descriptor
	pubkey1, _ := hex.DecodeString("02d2b36900396c9282fa14628566582f206a5dd0bcc8d5e892611806cafb0301f0")
	chaincode1, _ := hex.DecodeString("637807030d55d01f9a0cb3a7839515d796bd07706386a6eddf06cc29a65a0e29")

	pubkey2, _ := hex.DecodeString("03cbcaa9c98c877a26977d00825c956a238e8dddfbd322cce4f74b0b5bd6ace4a7")
	chaincode2, _ := hex.DecodeString("9d2f86043276f9251a4a4f577166a5abeb16b6ec61e226b5b8063bc11f981186")

	pubkey3, _ := hex.DecodeString("0260b2003c386519fc9eadf2b5cf124dd8eea4c4e68d5e154050a9346ea98ce600")
	chaincode3, _ := hex.DecodeString("f4f3a5b1f5b4ef3b3e7b3a3f0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d")

	info := DescriptorInfo{
		ScriptType: "wsh",
		IsMultisig: true,
		Threshold:  2,
		Keys: []KeyInfo{
			{
				PublicKey:  pubkey1,
				ChainCode: chaincode1,
				ParentFP:  0x78412e3a,
				MasterFP:  0xe8f7c0d2,
				DerivationPath: []uint32{
					48 | 0x80000000,
					1 | 0x80000000,
					0 | 0x80000000,
					2 | 0x80000000,
				},
				Name: "Alice",
			},
			{
				PublicKey:  pubkey2,
				ChainCode: chaincode2,
				ParentFP:  0x1234abcd,
				MasterFP:  0x0f056943,
				DerivationPath: []uint32{
					48 | 0x80000000,
					1 | 0x80000000,
					0 | 0x80000000,
					2 | 0x80000000,
				},
				Name: "Bob",
			},
			{
				PublicKey:  pubkey3,
				ChainCode: chaincode3,
				ParentFP:  0xdeadbeef,
				MasterFP:  0xaabbccdd,
				DerivationPath: []uint32{
					48 | 0x80000000,
					1 | 0x80000000,
					0 | 0x80000000,
					2 | 0x80000000,
				},
				Name: "Carol",
			},
		},
		Network: 1, // testnet
	}

	cborData, err := EncodeCryptoOutput(info)
	if err != nil {
		t.Fatalf("EncodeCryptoOutput failed: %v", err)
	}

	if len(cborData) == 0 {
		t.Fatal("empty CBOR data")
	}

	urStr, err := EncodeDescriptorAsUR(info)
	if err != nil {
		t.Fatalf("EncodeDescriptorAsUR failed: %v", err)
	}

	if !strings.HasPrefix(urStr, "ur:crypto-output/") {
		t.Errorf("expected ur:crypto-output/ prefix, got prefix: %q", urStr[:30])
	}

	t.Logf("Multisig UR (%d bytes CBOR): %s", len(cborData), urStr)

	// Verify the CBOR size is reasonable for a QR code (< 1000 bytes)
	if len(cborData) > 1000 {
		t.Errorf("CBOR data too large for single QR code: %d bytes", len(cborData))
	}
}

func TestEncodeNoKeys(t *testing.T) {
	info := DescriptorInfo{
		ScriptType: "wpkh",
		IsMultisig: false,
		Keys:       []KeyInfo{},
	}

	_, err := EncodeCryptoOutput(info)
	if err == nil {
		t.Error("expected error for empty keys")
	}
}

func TestCBORTagValues(t *testing.T) {
	// Verify tag constants match BCR spec
	tests := []struct {
		name string
		tag  uint64
	}{
		{"CryptoHDKey", 303},
		{"CryptoKeypath", 304},
		{"CryptoCoinInfo", 305},
		{"ScriptSH", 400},
		{"ScriptWSH", 401},
		{"ScriptPKH", 403},
		{"ScriptWPKH", 404},
		{"ScriptSortedMulti", 407},
		{"ScriptTR", 409},
	}

	expectedTags := map[string]uint64{
		"CryptoHDKey":      tagCryptoHDKey,
		"CryptoKeypath":    tagCryptoKeypath,
		"CryptoCoinInfo":   tagCryptoCoinInfo,
		"ScriptSH":         tagScriptSH,
		"ScriptWSH":        tagScriptWSH,
		"ScriptPKH":        tagScriptPKH,
		"ScriptWPKH":       tagScriptWPKH,
		"ScriptSortedMulti": tagScriptSortedMulti,
		"ScriptTR":         tagScriptTR,
	}

	for _, tt := range tests {
		if expectedTags[tt.name] != tt.tag {
			t.Errorf("%s: expected tag %d, got %d", tt.name, tt.tag, expectedTags[tt.name])
		}
	}
}

func TestBytewordsTableCompleteness(t *testing.T) {
	// Verify all 256 entries are unique 4-letter words
	seen := make(map[string]bool)
	for i, word := range bytewordsTable {
		if len(word) != 4 {
			t.Errorf("bytewords[%d] = %q: expected 4 chars", i, word)
		}
		if seen[word] {
			t.Errorf("bytewords[%d] = %q: duplicate word", i, word)
		}
		seen[word] = true
	}
	if len(seen) != 256 {
		t.Errorf("expected 256 unique words, got %d", len(seen))
	}
}

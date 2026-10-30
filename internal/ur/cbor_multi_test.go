package ur

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func multiTagTestInfo() DescriptorInfo {
	pubkey1, _ := hex.DecodeString("02d2b36900396c9282fa14628566582f206a5dd0bcc8d5e892611806cafb0301f0")
	chaincode1, _ := hex.DecodeString("637807030d55d01f9a0cb3a7839515d796bd07706386a6eddf06cc29a65a0e29")
	pubkey2, _ := hex.DecodeString("03cbcaa9c98c877a26977d00825c956a238e8dddfbd322cce4f74b0b5bd6ace4a7")
	chaincode2, _ := hex.DecodeString("9d2f86043276f9251a4a4f577166a5abeb16b6ec61e226b5b8063bc11f981186")
	pubkey3, _ := hex.DecodeString("0260b2003c386519fc9eadf2b5cf124dd8eea4c4e68d5e154050a9346ea98ce600")
	chaincode3, _ := hex.DecodeString("f4f3a5b1f5b4ef3b3e7b3a3f0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d")

	return DescriptorInfo{
		ScriptType: "wsh",
		IsMultisig: true,
		Threshold:  2,
		Keys: []KeyInfo{
			{PublicKey: pubkey1, ChainCode: chaincode1, Name: "Alice"},
			{PublicKey: pubkey2, ChainCode: chaincode2, Name: "Bob"},
			{PublicKey: pubkey3, ChainCode: chaincode3, Name: "Carol"},
		},
		Network: 1,
	}
}

// TestEncodeMultisigOutputMultiTag checks that unsorted multisig uses tag 406
// while sorted multisig keeps tag 407, with otherwise identical bytes.
func TestEncodeMultisigOutputMultiTag(t *testing.T) {
	for _, scriptType := range []string{"wsh", "sh-wsh", "sh"} {
		t.Run(scriptType, func(t *testing.T) {
			info := multiTagTestInfo()
			info.ScriptType = scriptType

			sortedBytes, err := EncodeCryptoOutput(info)
			if err != nil {
				t.Fatal(err)
			}
			info.Unsorted = true
			multiBytes, err := EncodeCryptoOutput(info)
			if err != nil {
				t.Fatal(err)
			}

			multiHead := []byte{0xd9, 0x01, 0x96}  // tag 406
			sortedHead := []byte{0xd9, 0x01, 0x97} // tag 407

			if !bytes.Contains(multiBytes, multiHead) {
				t.Fatalf("unsorted CBOR has no tag 406: %x", multiBytes)
			}
			if bytes.Contains(multiBytes, sortedHead) {
				t.Fatalf("unsorted CBOR uses tag 407: %x", multiBytes)
			}
			if !bytes.Contains(sortedBytes, sortedHead) {
				t.Fatalf("sorted CBOR has no tag 407: %x", sortedBytes)
			}
			if bytes.Contains(sortedBytes, multiHead) {
				t.Fatalf("sorted CBOR uses tag 406: %x", sortedBytes)
			}

			idx := bytes.Index(multiBytes, multiHead)
			replaced := append([]byte(nil), multiBytes...)
			replaced[idx+2] = 0x97
			if !bytes.Equal(replaced, sortedBytes) {
				t.Fatalf("CBOR differs beyond the multi tag:\n multi %x\nsorted %x",
					multiBytes, sortedBytes)
			}

			// EncodeDescriptorAsUR must carry the same CBOR payload.
			urStr, err := EncodeDescriptorAsUR(info)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := DecodeSinglePart(strings.TrimPrefix(urStr, "ur:crypto-output/"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, multiBytes) {
				t.Fatalf("UR payload differs from CBOR output")
			}

			// Keys stay in the given order.
			offset := 0
			for i, key := range info.Keys {
				pos := bytes.Index(multiBytes[offset:], key.PublicKey)
				if pos < 0 {
					t.Fatalf("key %d is missing or out of order", i)
				}
				offset += pos + len(key.PublicKey)
			}
		})
	}
}

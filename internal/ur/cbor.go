package ur

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Minimal CBOR encoder for BCR-2020-010 crypto-output structures.
// Only implements the CBOR types needed: unsigned int, byte string,
// text string, array, map, and tags.
// Reference: RFC 8949

// CBOR major types
const (
	cborUint    = 0 << 5 // Major type 0: unsigned integer
	cborBytes   = 2 << 5 // Major type 2: byte string
	cborText    = 3 << 5 // Major type 3: text string
	cborArray   = 4 << 5 // Major type 4: array
	cborMap     = 5 << 5 // Major type 5: map
	cborTag     = 6 << 5 // Major type 6: semantic tag
	cborSpecial = 7 << 5 // Major type 7: special (bool, null, etc.)
	cborFalse   = cborSpecial | 20
	cborTrue    = cborSpecial | 21
)

// cborBuf is a simple CBOR encoder that builds into a byte slice.
type cborBuf struct {
	data []byte
}

func (c *cborBuf) writeHead(major byte, val uint64) {
	if val <= 23 {
		c.data = append(c.data, major|byte(val))
	} else if val <= math.MaxUint8 {
		c.data = append(c.data, major|24, byte(val))
	} else if val <= math.MaxUint16 {
		c.data = append(c.data, major|25)
		c.data = binary.BigEndian.AppendUint16(c.data, uint16(val))
	} else if val <= math.MaxUint32 {
		c.data = append(c.data, major|26)
		c.data = binary.BigEndian.AppendUint32(c.data, uint32(val))
	} else {
		c.data = append(c.data, major|27)
		c.data = binary.BigEndian.AppendUint64(c.data, val)
	}
}

func (c *cborBuf) writeUint(val uint64) { c.writeHead(cborUint, val) }
func (c *cborBuf) writeBytes(b []byte) {
	c.writeHead(cborBytes, uint64(len(b)))
	c.data = append(c.data, b...)
}
func (c *cborBuf) writeText(s string) {
	c.writeHead(cborText, uint64(len(s)))
	c.data = append(c.data, s...)
}
func (c *cborBuf) writeArrayHeader(n int) { c.writeHead(cborArray, uint64(n)) }
func (c *cborBuf) writeMapHeader(n int)   { c.writeHead(cborMap, uint64(n)) }
func (c *cborBuf) writeTag(tag uint64)    { c.writeHead(cborTag, tag) }
func (c *cborBuf) writeBool(val bool) {
	if val {
		c.data = append(c.data, cborTrue)
	} else {
		c.data = append(c.data, cborFalse)
	}
}

// BCR-2020-010 CBOR tag numbers for crypto-output
const (
	tagCryptoHDKey    = 303 // #6.303 - crypto-hdkey
	tagCryptoKeypath  = 304 // #6.304 - crypto-keypath
	tagCryptoCoinInfo = 305 // #6.305 - crypto-coininfo

	// Script expression tags (BCR-2020-010)
	tagScriptSH          = 400 // sh(...)
	tagScriptWSH         = 401 // wsh(...)
	tagScriptPKH         = 403 // pkh(...)
	tagScriptWPKH        = 404 // wpkh(...)
	tagScriptSortedMulti = 407 // sortedmulti(...)
	tagScriptTR          = 409 // tr(...)
)

// DescriptorInfo holds the information needed to encode a descriptor as UR.
type DescriptorInfo struct {
	ScriptType string // "wpkh", "wsh", "sh-wsh", "sh", "pkh", "tr"
	IsMultisig bool
	Threshold  int // M in M-of-N (only for multisig)
	Keys       []KeyInfo
	Network    int // 0=mainnet, 1=testnet
}

// KeyInfo holds per-key data for UR encoding.
type KeyInfo struct {
	PublicKey      []byte   // 33-byte compressed pubkey
	ChainCode      []byte   // 32-byte chain code
	ParentFP       uint32   // 4-byte parent fingerprint
	MasterFP       uint32   // 4-byte master fingerprint
	DerivationPath []uint32 // Path components (with hardened bit 0x80000000 set where applicable)
	Name           string   // Cosigner label (CBOR key 9)
}

// EncodeCryptoOutput produces the CBOR bytes for a crypto-output.
func EncodeCryptoOutput(info DescriptorInfo) ([]byte, error) {
	if len(info.Keys) == 0 {
		return nil, fmt.Errorf("no keys provided")
	}

	var buf cborBuf

	if info.IsMultisig {
		return encodeMultisigOutput(&buf, info)
	}
	return encodeSingleSigOutput(&buf, info)
}

func encodeSingleSigOutput(buf *cborBuf, info DescriptorInfo) ([]byte, error) {
	// Wrap key in script expression tag
	switch info.ScriptType {
	case "wpkh":
		buf.writeTag(tagScriptWPKH)
	case "sh-wpkh":
		buf.writeTag(tagScriptSH)
		buf.writeTag(tagScriptWPKH)
	case "pkh":
		buf.writeTag(tagScriptPKH)
	case "tr":
		buf.writeTag(tagScriptTR)
	default:
		buf.writeTag(tagScriptWPKH)
	}

	encodeCryptoHDKey(buf, info.Keys[0], info.Network, false)
	return buf.data, nil
}

func encodeMultisigOutput(buf *cborBuf, info DescriptorInfo) ([]byte, error) {
	// Determine outer script wrapper
	switch info.ScriptType {
	case "wsh":
		// wsh(sortedmulti(...))
		buf.writeTag(tagScriptWSH)
	case "sh-wsh":
		// sh(wsh(sortedmulti(...)))
		buf.writeTag(tagScriptSH)
		buf.writeTag(tagScriptWSH)
	case "sh":
		// sh(sortedmulti(...))
		buf.writeTag(tagScriptSH)
	default:
		buf.writeTag(tagScriptWSH)
	}

	// sortedmulti tag wraps the multikey structure
	buf.writeTag(tagScriptSortedMulti)

	// Multikey: CBOR map {1: threshold, 2: [keys]}
	buf.writeMapHeader(2)

	// Key 1: threshold (M)
	buf.writeUint(1)
	buf.writeUint(uint64(info.Threshold))

	// Key 2: array of crypto-hdkeys
	buf.writeUint(2)
	buf.writeArrayHeader(len(info.Keys))
	for _, key := range info.Keys {
		encodeCryptoHDKey(buf, key, info.Network, false)
	}

	return buf.data, nil
}

// encodeCryptoHDKey writes a crypto-hdkey CBOR structure.
// BCR-2020-007 defines the map keys:
//
//	1: is-master (bool)
//	2: is-private (bool)
//	3: key-data (bytes) - 33 bytes for public
//	4: chain-code (bytes) - 32 bytes
//	5: use-info (crypto-coininfo)
//	6: origin (crypto-keypath) - derivation from master
//	7: children (crypto-keypath) - expected child derivation
//	8: parent-fingerprint (uint32)
//	9: name (text) - human-readable label
func encodeCryptoHDKey(buf *cborBuf, key KeyInfo, network int, isMaster bool) {
	// Count map entries
	count := 0
	if len(key.PublicKey) > 0 {
		count++ // key 3
	}
	if len(key.ChainCode) > 0 {
		count++ // key 4
	}
	count++ // key 5 (use-info, always present)
	if len(key.DerivationPath) > 0 {
		count++ // key 6 (origin)
	}
	count++ // key 7 (children - wildcard derivation)
	if key.ParentFP != 0 {
		count++ // key 8
	}
	if key.Name != "" {
		count++ // key 9
	}

	buf.writeTag(tagCryptoHDKey)
	buf.writeMapHeader(count)

	// Key 3: key-data (33-byte compressed public key)
	if len(key.PublicKey) > 0 {
		buf.writeUint(3)
		buf.writeBytes(key.PublicKey)
	}

	// Key 4: chain-code (32 bytes)
	if len(key.ChainCode) > 0 {
		buf.writeUint(4)
		buf.writeBytes(key.ChainCode)
	}

	// Key 5: use-info (crypto-coininfo)
	buf.writeUint(5)
	encodeCryptoCoinInfo(buf, network)

	// Key 6: origin (crypto-keypath from master)
	if len(key.DerivationPath) > 0 {
		buf.writeUint(6)
		encodeCryptoKeypath(buf, key.DerivationPath, key.MasterFP)
	}

	// Key 7: children (wildcard for receive+change)
	// Encode as empty keypath with no source fingerprint
	// This represents the /<0;1>/* suffix
	buf.writeUint(7)
	encodeCryptoKeypathChildren(buf)

	// Key 8: parent-fingerprint
	if key.ParentFP != 0 {
		buf.writeUint(8)
		buf.writeUint(uint64(key.ParentFP))
	}

	// Key 9: name (cosigner label)
	if key.Name != "" {
		buf.writeUint(9)
		buf.writeText(key.Name)
	}
}

// encodeCryptoCoinInfo writes a crypto-coininfo: {1: type, 2: network}
// type 0 = BTC, network 0 = mainnet, 1 = testnet
func encodeCryptoCoinInfo(buf *cborBuf, network int) {
	buf.writeTag(tagCryptoCoinInfo)
	if network == 0 {
		// Mainnet: empty map (defaults are BTC mainnet)
		buf.writeMapHeader(0)
	} else {
		// Testnet: specify network=1
		buf.writeMapHeader(1)
		buf.writeUint(2)
		buf.writeUint(uint64(network))
	}
}

// encodeCryptoKeypath writes a crypto-keypath for derivation origin.
// Map keys: 1=components (array), 2=source-fingerprint (uint32)
// Components alternate: [child-index, is-hardened, child-index, is-hardened, ...]
func encodeCryptoKeypath(buf *cborBuf, path []uint32, sourceFingerprint uint32) {
	buf.writeTag(tagCryptoKeypath)

	mapCount := 1 // components
	if sourceFingerprint != 0 {
		mapCount++
	}
	buf.writeMapHeader(mapCount)

	// Key 1: components array
	buf.writeUint(1)
	buf.writeArrayHeader(len(path) * 2) // each component is [index, hardened]
	for _, component := range path {
		isHardened := component&0x80000000 != 0
		index := component & 0x7FFFFFFF
		buf.writeUint(uint64(index))
		buf.writeBool(isHardened)
	}

	// Key 2: source fingerprint
	if sourceFingerprint != 0 {
		buf.writeUint(2)
		buf.writeUint(uint64(sourceFingerprint))
	}
}

// encodeCryptoKeypathChildren writes a children keypath for wildcard derivation.
// Per BCR-2020-007, wildcard is encoded as [[], false] where [] is an empty CBOR array.
// This encodes "/*" — all child indexes, non-hardened.
func encodeCryptoKeypathChildren(buf *cborBuf) {
	buf.writeTag(tagCryptoKeypath)
	buf.writeMapHeader(1)
	buf.writeUint(1)
	// Components: [[], false] = wildcard non-hardened
	buf.writeArrayHeader(2)
	buf.writeArrayHeader(0) // [] = wildcard marker
	buf.writeBool(false)    // non-hardened
}

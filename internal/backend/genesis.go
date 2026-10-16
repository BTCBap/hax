package backend

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// GenesisProvider is implemented by backends that can report the hash of
// block 0 of the chain they serve. Used to verify the server is on the
// wallet's network before trusting any of its data.
type GenesisProvider interface {
	GenesisHash() (chainhash.Hash, error)
}

// GenesisHash fetches the block 0 header and hashes it locally, so the
// answer does not rely on the server reporting its own chain name.
func (b *ElectrumBackend) GenesisHash() (chainhash.Hash, error) {
	result, err := b.request("blockchain.block.header", []interface{}{0})
	if err != nil {
		return chainhash.Hash{}, err
	}
	var headerHex string
	if err := json.Unmarshal(result, &headerHex); err != nil {
		return chainhash.Hash{}, err
	}
	return hashHeaderHex(headerHex)
}

func hashHeaderHex(headerHex string) (chainhash.Hash, error) {
	raw, err := hex.DecodeString(headerHex)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("bad header hex: %w", err)
	}
	if len(raw) != 80 {
		return chainhash.Hash{}, fmt.Errorf("bad header length %d", len(raw))
	}
	return chainhash.DoubleHashH(raw), nil
}

// GenesisHash returns getblockhash 0 from Bitcoin Core.
func (b *RPCBackend) GenesisHash() (chainhash.Hash, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return chainhash.Hash{}, err
		}
	}
	result, err := b.callRPC("getblockhash", []interface{}{0})
	if err != nil {
		return chainhash.Hash{}, err
	}
	var s string
	if err := json.Unmarshal(result, &s); err != nil {
		return chainhash.Hash{}, err
	}
	h, err := chainhash.NewHashFromStr(s)
	if err != nil {
		return chainhash.Hash{}, err
	}
	return *h, nil
}

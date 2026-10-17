package backend

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/wire"
)

// decodeRawTx decodes a hex transaction and checks that it hashes to
// txid, so a server cannot substitute a different previous transaction.
func decodeRawTx(txid, rawHex string) (*wire.MsgTx, error) {
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction hex: %w", err)
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}
	if got := tx.TxHash().String(); got != txid {
		return nil, fmt.Errorf("server returned transaction %s for %s", got, txid)
	}
	return &tx, nil
}

package backend

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

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
	if got := tx.TxHash().String(); !strings.EqualFold(got, txid) {
		return nil, fmt.Errorf("server returned transaction %s for %s", got, txid)
	}
	return &tx, nil
}

// verifyBroadcastTxID computes the transaction id locally and requires the
// server to report the same one (H3-26). A server cannot make a failed
// broadcast look successful by returning a different txid.
func verifyBroadcastTxID(rawTxHex, reported string) (string, error) {
	raw, err := hex.DecodeString(rawTxHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode transaction hex: %w", err)
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return "", fmt.Errorf("failed to deserialize transaction: %w", err)
	}
	got := tx.TxHash().String()
	if !strings.EqualFold(reported, got) {
		return "", fmt.Errorf("server reported transaction %s but the transaction id is %s", reported, got)
	}
	return got, nil
}

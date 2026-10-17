package wallet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrCoinFrozen is returned when a transaction would spend a frozen coin.
var ErrCoinFrozen = errors.New("coin is frozen")

func outpointKey(txid string, vout uint32) string {
	return fmt.Sprintf("%s:%d", txid, vout)
}

// parseOutpoint validates a "txid:vout" reference.
func parseOutpoint(ref string) (string, uint32, bool) {
	txid, n, ok := strings.Cut(ref, ":")
	if !ok || len(txid) != 64 {
		return "", 0, false
	}
	if _, err := hex.DecodeString(txid); err != nil {
		return "", 0, false
	}
	vout, err := strconv.ParseUint(n, 10, 32)
	if err != nil {
		return "", 0, false
	}
	return strings.ToLower(txid), uint32(vout), true
}

// IsUTXOFrozen reports whether the coin txid:vout is frozen.
func (m *WalletManager) IsUTXOFrozen(txid string, vout uint32) bool {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	return m.data != nil && m.data.FrozenOutputs[outpointKey(txid, vout)]
}

// SetUTXOFrozen freezes or unfreezes the coin txid:vout and saves the
// wallet. Frozen coins are never picked by coin selection, send-max or
// fee bumps, and a transaction that names one explicitly is refused.
func (m *WalletManager) SetUTXOFrozen(txid string, vout uint32, frozen bool) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	setFrozenLocked(m.data, outpointKey(txid, vout), frozen)
	if err := m.saveWalletTracked(m.data.Name, m.data); err != nil {
		return fmt.Errorf("failed to save frozen coins: %w", err)
	}
	return nil
}

// setFrozenLocked updates the frozen set. Callers hold dataMu.
func setFrozenLocked(data *WalletData, key string, frozen bool) {
	if !frozen {
		delete(data.FrozenOutputs, key)
		return
	}
	if data.FrozenOutputs == nil {
		data.FrozenOutputs = make(map[string]bool)
	}
	data.FrozenOutputs[key] = true
}

// spendableUTXOs drops frozen coins.
func (m *WalletManager) spendableUTXOs(utxos []UTXOWithPath) []UTXOWithPath {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	if m.data == nil || len(m.data.FrozenOutputs) == 0 {
		return utxos
	}
	out := make([]UTXOWithPath, 0, len(utxos))
	for _, u := range utxos {
		if !m.data.FrozenOutputs[outpointKey(u.TxID, u.Vout)] {
			out = append(out, u)
		}
	}
	return out
}

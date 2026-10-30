package wallet

import (
	"fmt"

	"github.com/BTCBap/hax/internal/api"
)

// TxRecipient represents a single output in a batch transaction
type TxRecipient struct {
	Address string      // Recipient Bitcoin address
	Amount  api.Satoshi // Amount in satoshis
}

// resolveUTXOs converts api.UTXO to UTXOWithPath by looking up addresses.
// If utxos is empty, fetches all available UTXOs for the account.
// The addresses parameter provides the lookup table for UTXO address matching.
func (m *WalletManager) resolveUTXOs(accountIndex uint32, utxos []api.UTXO, addresses []AddressData) ([]UTXOWithPath, error) {
	if len(utxos) > 0 {
		utxosWithPath := make([]UTXOWithPath, 0, len(utxos))
		for _, utxo := range utxos {
			if m.IsUTXOFrozen(utxo.TxID, utxo.Vout) {
				return nil, fmt.Errorf("%s:%d: %w; unfreeze it first",
					shortTxID(utxo.TxID), utxo.Vout, ErrCoinFrozen)
			}
			tx, err := m.backend.GetTransaction(utxo.TxID)
			if err != nil {
				return nil, fmt.Errorf("failed to get transaction %s: %w", utxo.TxID, err)
			}
			if int(utxo.Vout) >= len(tx.Vout) {
				return nil, fmt.Errorf("invalid vout index %d for transaction %s", utxo.Vout, utxo.TxID)
			}
			// The backend's listunspent value is a claim. Reject it unless
			// the transaction it just returned agrees, so a hostile server
			// cannot skew change and fee (H3-06).
			if tx.Vout[utxo.Vout].Value != utxo.Value {
				return nil, fmt.Errorf("UTXO %s:%d value does not match its transaction", utxo.TxID, utxo.Vout)
			}
			utxoAddr := tx.Vout[utxo.Vout].ScriptpubkeyAddress
			var path string
			found := false
			for _, addr := range addresses {
				if addr.Address == utxoAddr {
					path = addr.Path
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("UTXO %s:%d belongs to address %s which is not in this account",
					utxo.TxID, utxo.Vout, utxoAddr)
			}
			utxosWithPath = append(utxosWithPath, UTXOWithPath{
				UTXO:    utxo,
				Address: utxoAddr,
				Path:    path,
			})
		}
		return utxosWithPath, nil
	}
	// No UTXOs provided, fetch all available
	utxosWithPath, err := m.GetUTXOsWithPath(accountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}
	if len(utxosWithPath) == 0 {
		return nil, fmt.Errorf("no UTXOs available")
	}
	utxosWithPath = m.spendableUTXOs(utxosWithPath)
	if len(utxosWithPath) == 0 {
		return nil, fmt.Errorf("every coin is frozen")
	}
	return utxosWithPath, nil
}

// PreviewBatchTransaction previews a transaction with multiple recipients using the PSBT flow.
// Works uniformly for software, multisig, and hardware wallets.
func (m *WalletManager) PreviewBatchTransaction(accountIndex uint32, recipients []TxRecipient, feeRateSatsPerVByte float64, utxos []api.UTXO) (*PSBTResult, error) {
	return m.CreatePSBTBatchTransaction(accountIndex, recipients, feeRateSatsPerVByte, utxos)
}

// CreatePSBTTransaction creates an unsigned PSBT for a single-recipient transaction.
// Thin wrapper around CreatePSBTBatchTransaction.
func (m *WalletManager) CreatePSBTTransaction(accountIndex uint32, recipientAddr string, amountSats api.Satoshi, feeRateSatsPerVByte float64, utxos []api.UTXO) (*PSBTResult, error) {
	return m.CreatePSBTBatchTransaction(accountIndex, []TxRecipient{
		{Address: recipientAddr, Amount: amountSats},
	}, feeRateSatsPerVByte, utxos)
}

// CreatePSBTBatchTransaction creates an unsigned PSBT for a batch transaction with multiple recipients
// If utxos is nil or empty, automatically selects UTXOs
// If utxos is provided, uses exactly those UTXOs (coin control)
// Returns PSBTResult containing the unsigned PSBT packet and transaction details
func (m *WalletManager) CreatePSBTBatchTransaction(accountIndex uint32, recipients []TxRecipient, feeRateSatsPerVByte float64, utxos []api.UTXO) (*PSBTResult, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}

	if int(accountIndex) >= len(m.data.Accounts) {
		return nil, fmt.Errorf("account not found")
	}

	utxosWithPath, err := m.resolveUTXOs(accountIndex, utxos, m.data.Accounts[accountIndex].Addresses)
	if err != nil {
		return nil, err
	}

	changeAddr, changePath, changeAddrIndex, err := m.GetCurrentChangeAddress(accountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get change address: %w", err)
	}

	network := m.getNetwork()

	// Convert recipients to outputs
	outputs := make([]TxOutput, len(recipients))
	for i, r := range recipients {
		outputs[i] = TxOutput(r)
	}

	scriptType, multisigM, multisigN := m.GetScriptType()

	var selectedUTXOs []UTXOWithPath
	if len(utxos) > 0 {
		selectedUTXOs = utxosWithPath
	} else {
		selectedUTXOs, _, err = SelectUTXOs(utxosWithPath, nil, outputs, feeRateSatsPerVByte, scriptType, scriptType, multisigM, multisigN)
		if err != nil {
			return nil, fmt.Errorf("failed to select UTXOs: %w", err)
		}
	}

	witnessScripts, err := m.multisigWitnessScripts(selectedUTXOs)
	if err != nil {
		return nil, err
	}

	result, err := CreatePSBT(selectedUTXOs, outputs, changeAddr, feeRateSatsPerVByte, network, witnessScripts, scriptType, multisigM, multisigN)
	if err != nil {
		return nil, fmt.Errorf("failed to create PSBT: %w", err)
	}

	changeOutputIdx := -1
	if result.HasChange {
		changeOutputIdx = len(recipients)
	}
	if err := m.addSignerData(result, changePath, changeAddrIndex, changeOutputIdx); err != nil {
		return nil, err
	}

	return result, nil
}

// multisigWitnessScripts returns the witness script for each input of a
// multisig wallet, or nil for single-sig.
func (m *WalletManager) multisigWitnessScripts(utxos []UTXOWithPath) ([][]byte, error) {
	if m.data.Multisig == nil {
		return nil, nil
	}
	scripts := make([][]byte, len(utxos))
	for i, utxo := range utxos {
		change, index, err := parseMultisigPath(utxo.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse path for UTXO %d: %w", i, err)
		}
		_, witnessScript, err := m.GetMultisigAddress(change, index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive witness script for UTXO %d: %w", i, err)
		}
		scripts[i] = witnessScript
	}
	return scripts, nil
}

// addSignerData adds what signers need to a freshly built PSBT: key
// derivations for inputs and change (changeOutputIdx -1 when there is
// none), multisig scripts and xpubs, and previous transactions.
func (m *WalletManager) addSignerData(result *PSBTResult, changePath string, changeAddrIndex uint32, changeOutputIdx int) error {
	if m.IsMultisig() && m.data.Multisig != nil {
		// Metadata errors are not fatal: hax signs from its own
		// paths, and only some external signers need these fields.
		_ = m.addBip32DerivationToInputs(result.Packet, result.SelectedUTXOs)
		if changeOutputIdx >= 0 {
			_ = m.addMultisigChangeOutputMetadata(result.Packet, changeAddrIndex, changeOutputIdx)
		}
		// Global xpubs let devices like Coldcard detect the multisig.
		_ = m.addGlobalXpubs(result.Packet)
	} else {
		m.addSingleSigSignerData(result.Packet, result.SelectedUTXOs, changePath, changeOutputIdx)
	}

	// After the signer data: whether a P2SH input is segwit depends on
	// its redeem script.
	if err := m.attachPrevTxs(result); err != nil {
		return fmt.Errorf("failed to attach previous transactions: %w", err)
	}
	return nil
}

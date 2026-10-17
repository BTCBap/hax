package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BTCBap/hax/internal/api"
)

// GetTransactionLabel returns the label for a transaction (BIP-329)
func (m *WalletManager) GetTransactionLabel(txid string) string {
	if m.data == nil || m.data.TransactionLabels == nil {
		return ""
	}
	return m.data.TransactionLabels[txid]
}

// SetTransactionLabel sets a label for a transaction (BIP-329)
func (m *WalletManager) SetTransactionLabel(txid string, label string) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	defer m.dataMu.Unlock()

	// Initialize map if needed
	if m.data.TransactionLabels == nil {
		m.data.TransactionLabels = make(map[string]string)
	}

	// Set or delete label
	if label == "" {
		delete(m.data.TransactionLabels, txid)
	} else {
		// Truncate to 255 characters per BIP-329 recommendation
		if len(label) > 255 {
			label = label[:255]
		}
		m.data.TransactionLabels[txid] = label
	}

	// Save wallet (lock held via defer)
	if err := m.saveWalletTracked(m.data.Name, m.data); err != nil {
		return fmt.Errorf("failed to save label: %w", err)
	}

	return nil
}

// ErrNoLabels is returned when there is nothing to export.
var ErrNoLabels = errors.New("no labels to export")

// GetAddressLabel returns the label of one of the wallet's addresses.
func (m *WalletManager) GetAddressLabel(address string) string {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	if m.data == nil {
		return ""
	}
	for _, acc := range m.data.Accounts {
		if l, ok := acc.Labels[address]; ok {
			return l
		}
	}
	return ""
}

// SetAddressLabel labels one of the wallet's own addresses (BIP-329
// "addr"). Labels live in the account's Labels map inside the encrypted
// wallet file. An empty label removes it.
func (m *WalletManager) SetAddressLabel(address, label string) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	acc := m.accountForAddressLocked(address)
	if acc == nil {
		return fmt.Errorf("%s is not an address of this wallet", address)
	}
	setLabel(&acc.Labels, address, label)
	if err := m.saveWalletTracked(m.data.Name, m.data); err != nil {
		return fmt.Errorf("failed to save label: %w", err)
	}
	return nil
}

// accountForAddressLocked finds the account that derived address.
// Callers hold dataMu.
func (m *WalletManager) accountForAddressLocked(address string) *AccountData {
	if m.data == nil {
		return nil
	}
	for i := range m.data.Accounts {
		for _, a := range m.data.Accounts[i].Addresses {
			if a.Address == address {
				return &m.data.Accounts[i]
			}
		}
	}
	return nil
}

// setLabel sets or (for an empty label) deletes a label, capping it at
// 255 characters without splitting a UTF-8 sequence.
func setLabel(labels *map[string]string, key, label string) {
	label = strings.TrimSpace(label)
	if label == "" {
		if *labels != nil {
			delete(*labels, key)
		}
		return
	}
	if r := []rune(label); len(r) > 255 {
		label = string(r[:255])
	}
	if *labels == nil {
		*labels = make(map[string]string)
	}
	(*labels)[key] = label
}

// ExportLabelsBIP329 exports transaction and address labels, and frozen
// coins as outputs with spendable:false, in BIP-329 JSON Lines format,
// sorted for stable output.
func (m *WalletManager) ExportLabelsBIP329() (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()

	var recs []bip329Record
	for txid, label := range m.data.TransactionLabels {
		recs = append(recs, bip329Record{Type: "tx", Ref: txid, Label: strPtr(label)})
	}
	for _, acc := range m.data.Accounts {
		for addr, label := range acc.Labels {
			recs = append(recs, bip329Record{Type: "addr", Ref: addr, Label: strPtr(label)})
		}
	}
	notSpendable := false
	for ref := range m.data.FrozenOutputs {
		recs = append(recs, bip329Record{Type: "output", Ref: ref, Spendable: &notSpendable})
	}
	if len(recs) == 0 {
		return "", ErrNoLabels
	}
	order := map[string]int{"tx": 0, "addr": 1, "output": 2}
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Type != recs[j].Type {
			return order[recs[i].Type] < order[recs[j].Type]
		}
		return recs[i].Ref < recs[j].Ref
	})

	var output strings.Builder
	for _, r := range recs {
		jsonBytes, err := json.Marshal(r)
		if err != nil {
			return "", fmt.Errorf("failed to marshal label: %w", err)
		}
		output.Write(jsonBytes)
		output.WriteByte('\n')
	}

	return output.String(), nil
}

// bip329Record is one line of a BIP-329 export. Label and Spendable are
// optional; an import leaves the value alone when they are absent.
type bip329Record struct {
	Type      string  `json:"type"`
	Ref       string  `json:"ref"`
	Label     *string `json:"label,omitempty"`
	Spendable *bool   `json:"spendable,omitempty"`
}

func strPtr(s string) *string { return &s }

// ImportLabelsBIP329 imports BIP-329 JSON Lines: transaction labels,
// labels of this wallet's addresses, and the spendable flag of outputs
// (spendable:false freezes a coin). Other records are skipped.
func (m *WalletManager) ImportLabelsBIP329(data string) (int, error) {
	if !m.unlocked {
		return 0, fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	defer m.dataMu.Unlock()

	// Initialize map if nil
	if m.data.TransactionLabels == nil {
		m.data.TransactionLabels = make(map[string]string)
	}

	imported := 0
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var r bip329Record
		if err := json.Unmarshal([]byte(line), &r); err != nil || r.Ref == "" {
			// Skip invalid lines
			continue
		}

		switch r.Type {
		case "tx":
			if r.Label != nil {
				m.data.TransactionLabels[r.Ref] = *r.Label
				imported++
			}
		case "addr":
			// Only addresses this wallet derived; others are skipped.
			if acc := m.accountForAddressLocked(r.Ref); acc != nil && r.Label != nil {
				setLabel(&acc.Labels, r.Ref, *r.Label)
				imported++
			}
		case "output":
			// spendable:false freezes the coin, true unfreezes it.
			if txid, vout, ok := parseOutpoint(r.Ref); ok && r.Spendable != nil {
				setFrozenLocked(m.data, outpointKey(txid, vout), !*r.Spendable)
				imported++
			}
		}
	}

	// Save wallet with new labels
	if imported > 0 {
		if err := m.saveWalletTracked(m.data.Name, m.data); err != nil {
			return imported, fmt.Errorf("failed to save imported labels: %w", err)
		}
	}

	return imported, nil
}

// ExportKoinly exports transaction history in Koinly CSV format for tax reporting
// Format: Date,Sent Amount,Sent Currency,Received Amount,Received Currency,Fee Amount,Fee Currency,Label,Description,TxHash
func (m *WalletManager) ExportKoinly(transactions []api.Transaction) (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	if len(transactions) == 0 {
		return "", fmt.Errorf("no transactions to export")
	}

	// Build address map for our wallet
	ourAddresses := make(map[string]bool)
	for _, account := range m.data.Accounts {
		for _, addr := range account.Addresses {
			ourAddresses[addr.Address] = true
		}
	}

	// CSV header
	var sb strings.Builder
	sb.WriteString("Date,Sent Amount,Sent Currency,Received Amount,Received Currency,Fee Amount,Fee Currency,Label,Description,TxHash\n")

	// Process each transaction
	for _, tx := range transactions {
		// Skip unconfirmed transactions for tax purposes (or include with note?)
		timestamp := ""
		if tx.Status.Confirmed && tx.Status.BlockTime > 0 {
			t := time.Unix(tx.Status.BlockTime, 0).UTC()
			timestamp = t.Format("2006-01-02 15:04:05")
		} else {
			// Use current time for unconfirmed
			t := time.Now().UTC()
			timestamp = t.Format("2006-01-02 15:04:05")
		}

		// Analyze transaction
		var totalIn, totalOut api.Satoshi
		for _, vin := range tx.Vin {
			if vin.Prevout != nil && ourAddresses[vin.Prevout.ScriptpubkeyAddress] {
				totalIn += vin.Prevout.Value
			}
		}
		for _, vout := range tx.Vout {
			if ourAddresses[vout.ScriptpubkeyAddress] {
				totalOut += vout.Value
			}
		}

		netAmount := totalOut - totalIn
		sentAmount := ""
		sentCurrency := ""
		receivedAmount := ""
		receivedCurrency := ""
		feeAmount := ""
		feeCurrency := ""
		label := ""

		if netAmount < 0 {
			// Outgoing transaction - we sent BTC
			absAmount := -netAmount
			sentAmount = fmt.Sprintf("%.8f", absAmount.ToBTC())
			sentCurrency = "BTC"

			// Calculate fee (only for outgoing)
			fee := tx.Fee
			if fee > 0 {
				feeAmount = fmt.Sprintf("%.8f", fee.ToBTC())
				feeCurrency = "BTC"
			}

			label = "withdrawal"
		} else if netAmount > 0 {
			// Incoming transaction
			receivedAmount = fmt.Sprintf("%.8f", netAmount.ToBTC())
			receivedCurrency = "BTC"
			label = "deposit"
		} else if totalIn > 0 && totalOut > 0 {
			// Self-send (consolidation or internal transfer)
			fee := tx.Fee
			if fee > 0 {
				feeAmount = fmt.Sprintf("%.8f", fee.ToBTC())
				feeCurrency = "BTC"
			}
			label = "cost" // Fee-only for self-sends
		}

		// Get label from wallet if set
		description := ""
		if m.data.TransactionLabels != nil {
			if walletLabel, ok := m.data.TransactionLabels[tx.TxID]; ok {
				description = walletLabel
			}
		}

		// Write CSV row
		// Escape commas in description
		description = strings.ReplaceAll(description, ",", " ")
		description = strings.ReplaceAll(description, "\"", "'")

		fmt.Fprintf(&sb, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			timestamp,
			sentAmount,
			sentCurrency,
			receivedAmount,
			receivedCurrency,
			feeAmount,
			feeCurrency,
			label,
			description,
			tx.TxID)
	}

	return sb.String(), nil
}

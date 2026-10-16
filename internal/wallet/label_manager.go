package wallet

import (
	"encoding/json"
	"fmt"
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

// ExportLabelsBIP329 exports transaction labels in BIP-329 JSON Lines format
func (m *WalletManager) ExportLabelsBIP329() (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	if len(m.data.TransactionLabels) == 0 {
		return "", fmt.Errorf("no transaction labels to export")
	}

	var output string
	for txid, label := range m.data.TransactionLabels {
		// Create BIP-329 record for each transaction label
		record := map[string]string{
			"type":  "tx",
			"ref":   txid,
			"label": label,
		}

		jsonBytes, err := json.Marshal(record)
		if err != nil {
			return "", fmt.Errorf("failed to marshal label: %w", err)
		}

		output += string(jsonBytes) + "\n"
	}

	return output, nil
}

// ImportLabelsBIP329 imports transaction labels from BIP-329 JSON Lines format
// Each line should be a JSON object with "type", "ref", and "label" fields
// Only "tx" type labels are imported (transaction labels)
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

		var record map[string]string
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			// Skip invalid lines
			continue
		}

		// BIP-329 requires type, ref, and label fields
		labelType, hasType := record["type"]
		ref, hasRef := record["ref"]
		label, hasLabel := record["label"]

		if !hasType || !hasRef || !hasLabel {
			continue
		}

		// Only import transaction labels (type="tx")
		if labelType != "tx" {
			continue
		}

		// Import the label (overwrites existing)
		m.data.TransactionLabels[ref] = label
		imported++
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

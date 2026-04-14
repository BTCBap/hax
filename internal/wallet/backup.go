package wallet

import (
	"encoding/json"
	"fmt"
	"time"
)

// WalletBackup is the unified backup format for all wallet types.
// It bundles the output descriptor, wallet config, and labels into
// a single JSON file. For multisig wallets, cosigner metadata is
// included. Mnemonics and seed phrases are never included.
type WalletBackup struct {
	Version    int                 `json:"version"`
	Format     string              `json:"format"`
	ExportedAt string              `json:"exported_at"`
	Wallet     BackupWalletInfo    `json:"wallet"`
	Descriptor string              `json:"descriptor"`
	Multisig   *BackupMultisigInfo `json:"multisig,omitempty"`
	Hardware   *BackupHardwareInfo `json:"hardware,omitempty"`
	Labels     *BackupLabels       `json:"labels,omitempty"`
}

// BackupWalletInfo holds wallet-level metadata.
type BackupWalletInfo struct {
	Name        string `json:"name"`
	Network     string `json:"network"`
	CreatedAt   string `json:"created_at"`
	WalletType  string `json:"wallet_type"`
	AddressType string `json:"address_type"`
}

// BackupMultisigInfo holds multisig-specific configuration.
type BackupMultisigInfo struct {
	M          int              `json:"required_signatures"`
	N          int              `json:"total_cosigners"`
	ScriptType string           `json:"script_type"`
	Cosigners  []BackupCosigner `json:"cosigners"`
}

// BackupCosigner holds per-cosigner public metadata.
// Secrets (mnemonics) are never included.
type BackupCosigner struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	KeyOrigin   string `json:"key_origin"`
	Xpub        string `json:"xpub"`
	DeviceType  string `json:"device_type,omitempty"`
}

// BackupHardwareInfo holds hardware wallet-specific metadata.
type BackupHardwareInfo struct {
	DeviceType  string `json:"device_type"`
	Fingerprint string `json:"fingerprint"`
	KeyOrigin   string `json:"key_origin"`
	Xpub        string `json:"xpub"`
}

// BackupLabels holds transaction and address labels.
type BackupLabels struct {
	Transactions []BackupLabel `json:"transactions,omitempty"`
	Addresses    []BackupLabel `json:"addresses,omitempty"`
}

// BackupLabel is a BIP-329-style label record.
type BackupLabel struct {
	Type  string `json:"type"`
	Ref   string `json:"ref"`
	Label string `json:"label"`
}

// MultisigExport is the JSON format for multisig wallet export.
// Compatible with Sparrow's "File > Import > Caravan Multisig" import.
// Cosigner names are preserved in extendedPublicKeys[].name.
type MultisigExport struct {
	Name                 string                `json:"name"`
	AddressType          string                `json:"addressType"`
	Network              string                `json:"network"`
	Client               MultisigExportClient  `json:"client"`
	Quorum               MultisigExportQuorum  `json:"quorum"`
	ExtendedPublicKeys   []MultisigExportKey   `json:"extendedPublicKeys"`
	StartingAddressIndex int                   `json:"startingAddressIndex"`
}

type MultisigExportClient struct {
	Type string `json:"type"`
}

type MultisigExportQuorum struct {
	RequiredSigners int `json:"requiredSigners"`
	TotalSigners    int `json:"totalSigners"`
}

type MultisigExportKey struct {
	Name      string `json:"name"`
	BIP32Path string `json:"bip32Path"`
	Xpub      string `json:"xpub"`
	XFP       string `json:"xfp"`
	Method    string `json:"method"`
}

// ExportMultisigConfig generates a JSON file for multisig wallet import into
// Sparrow and other compatible wallets. Cosigner labels are preserved.
func (m *WalletManager) ExportMultisigConfig() ([]byte, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}
	if !m.IsMultisig() {
		return nil, fmt.Errorf("not a multisig wallet")
	}

	config := m.data.Multisig

	// Map script type to addressType
	var addressType string
	switch MultisigScriptType(config.ScriptType) {
	case MultisigLegacy:
		addressType = "P2SH"
	case MultisigNestedSegwit:
		addressType = "P2SH-P2WSH"
	case MultisigNativeSegwit:
		addressType = "P2WSH"
	default:
		addressType = "P2WSH"
	}

	network := m.data.Network
	if network == "" {
		network = "testnet"
	}

	entries := make([]MultisigExportKey, len(config.Cosigners))
	for i, cs := range config.Cosigners {
		name := cs.Name
		if name == "" {
			name = fmt.Sprintf("Cosigner %d", i+1)
		}
		// Truncate to 16 chars (Sparrow limit)
		if len(name) > 16 {
			name = name[:16]
		}

		// Extract BIP32 path from key origin
		bip32Path := ""
		if cs.KeyOrigin != "" {
			bip32Path = keyOriginToBIP32Path(cs.KeyOrigin)
		}
		if bip32Path == "" {
			// Default BIP48 path
			scriptIdx := config.ScriptType
			coinType := 1 // testnet
			if m.data.Network == "mainnet" {
				coinType = 0
			}
			bip32Path = fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptIdx)
		}

		// Map device type to method
		method := "unknown"
		if cs.DeviceType != "" {
			switch cs.DeviceType {
			case "coldcard":
				method = "coldcard"
			case "trezor":
				method = "trezor"
			case "ledger":
				method = "ledger"
			case "local":
				method = "unknown"
			default:
				method = cs.DeviceType
			}
		}

		entries[i] = MultisigExportKey{
			Name:      name,
			BIP32Path: bip32Path,
			Xpub:      cs.Xpub,
			XFP:       cs.Fingerprint,
			Method:    method,
		}
	}

	export := MultisigExport{
		Name:        m.data.Name,
		AddressType: addressType,
		Network:     network,
		Client:      MultisigExportClient{Type: "public"},
		Quorum: MultisigExportQuorum{
			RequiredSigners: config.M,
			TotalSigners:    config.N,
		},
		ExtendedPublicKeys:   entries,
		StartingAddressIndex: 0,
	}

	return json.MarshalIndent(export, "", "  ")
}

// keyOriginToBIP32Path converts "fingerprint/48h/1h/0h/2h" to "m/48'/1'/0'/2'"
func keyOriginToBIP32Path(origin string) string {
	// Use the existing helper from hdwallet.go
	return keyOriginToPath(origin)
}

// ExportUnifiedBackup generates a JSON backup combining the output descriptor,
// wallet configuration, and all labels. For multisig wallets, cosigner metadata
// is included. Mnemonics and seed phrases are never included in the backup.
func (m *WalletManager) ExportUnifiedBackup() ([]byte, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	accounts := m.data.Accounts
	if len(accounts) == 0 {
		return nil, fmt.Errorf("wallet has no accounts")
	}

	// Generate output descriptor
	descriptor, err := m.ExportDescriptor(accounts[0].Index)
	if err != nil {
		return nil, fmt.Errorf("failed to generate descriptor: %w", err)
	}

	// Determine wallet type and address type
	walletType := "single_sig"
	addressType := accounts[0].AddressType
	if m.IsMultisig() {
		walletType = "multisig"
	} else if m.IsHardwareWallet() {
		walletType = "hardware"
		hwConfig := m.data.HardwareWallet
		if hwConfig != nil {
			addressType = hwConfig.AddressType
		}
	}

	backup := WalletBackup{
		Version:    1,
		Format:     "haxwallet-backup",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Wallet: BackupWalletInfo{
			Name:        m.data.Name,
			Network:     m.data.Network,
			CreatedAt:   time.Unix(m.data.CreatedAt, 0).UTC().Format(time.RFC3339),
			WalletType:  walletType,
			AddressType: addressType,
		},
		Descriptor: descriptor,
	}

	// Add multisig-specific data
	if m.IsMultisig() {
		config := m.data.Multisig
		scriptType := "unknown"
		switch MultisigScriptType(config.ScriptType) {
		case MultisigLegacy:
			scriptType = "legacy"
		case MultisigNestedSegwit:
			scriptType = "nested_segwit"
		case MultisigNativeSegwit:
			scriptType = "native_segwit"
		}

		cosigners := make([]BackupCosigner, len(config.Cosigners))
		for i, cs := range config.Cosigners {
			cosigners[i] = BackupCosigner{
				Name:        cs.Name,
				Fingerprint: cs.Fingerprint,
				KeyOrigin:   cs.KeyOrigin,
				Xpub:        cs.Xpub,
				DeviceType:  cs.DeviceType,
			}
		}

		backup.Multisig = &BackupMultisigInfo{
			M:          config.M,
			N:          config.N,
			ScriptType: scriptType,
			Cosigners:  cosigners,
		}
	}

	// Add hardware wallet-specific data
	if m.IsHardwareWallet() {
		hw := m.data.HardwareWallet
		backup.Hardware = &BackupHardwareInfo{
			DeviceType:  hw.DeviceType,
			Fingerprint: hw.Fingerprint,
			KeyOrigin:   hw.KeyOrigin,
			Xpub:        hw.Xpub,
		}
	}

	// Collect labels
	backup.Labels = collectLabels(m.data)

	return json.MarshalIndent(backup, "", "  ")
}

// collectLabels gathers transaction and address labels from wallet data.
// Returns nil if no labels exist.
func collectLabels(data *WalletData) *BackupLabels {
	hasTxLabels := len(data.TransactionLabels) > 0
	hasAddrLabels := false
	for _, acct := range data.Accounts {
		if len(acct.Labels) > 0 {
			hasAddrLabels = true
			break
		}
	}

	if !hasTxLabels && !hasAddrLabels {
		return nil
	}

	labels := &BackupLabels{}

	for txid, label := range data.TransactionLabels {
		labels.Transactions = append(labels.Transactions, BackupLabel{
			Type:  "tx",
			Ref:   txid,
			Label: label,
		})
	}

	for _, acct := range data.Accounts {
		for addr, label := range acct.Labels {
			labels.Addresses = append(labels.Addresses, BackupLabel{
				Type:  "addr",
				Ref:   addr,
				Label: label,
			})
		}
	}

	return labels
}

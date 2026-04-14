package wallet

import (
	"fmt"
	"strings"
)

// ImportCosignerSeed attaches a seed phrase to an existing cosigner for local signing
// The cosigner is matched by fingerprint or by deriving the xpub from the seed
// Returns the index of the matched cosigner, or error if no match
func (m *WalletManager) ImportCosignerSeed(mnemonic string) (int, error) {
	if !m.unlocked {
		return -1, fmt.Errorf("wallet is locked")
	}
	if !m.IsMultisig() {
		return -1, fmt.Errorf("not a multisig wallet")
	}

	if !ValidateMnemonic(mnemonic) {
		return -1, fmt.Errorf("invalid mnemonic phrase")
	}

	seed, err := MnemonicToSeed(mnemonic, "")
	if err != nil {
		return -1, fmt.Errorf("failed to derive seed: %w", err)
	}

	isTestnet := m.data.Network == "testnet"
	hdWallet, err := NewHDWallet(seed, isTestnet)
	if err != nil {
		return -1, fmt.Errorf("failed to create HD wallet: %w", err)
	}

	fingerprint, err := hdWallet.GetMasterFingerprintHex()
	if err != nil {
		return -1, fmt.Errorf("failed to get fingerprint: %w", err)
	}

	coinType := uint32(0)
	if isTestnet {
		coinType = 1
	}

	// Try different derivation paths (handles BIP84 xpubs pasted into BIP48 multisig)
	type derivationAttempt struct {
		xpub string
		path string
	}
	var attempts []derivationAttempt
	scriptType := MultisigScriptType(m.data.Multisig.ScriptType)
	bip48Path := BIP48Path{
		CoinType:   coinType,
		Account:    0,
		ScriptType: scriptType,
	}
	if bip48Xpub, err := hdWallet.GetMultisigAccountXpub(bip48Path); err == nil {
		attempts = append(attempts, derivationAttempt{
			xpub: bip48Xpub,
			path: fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType),
		})
	}

	// 2. Try BIP84 (native segwit single-sig - common mistake)
	if bip84Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeSegWitNative); err == nil {
		attempts = append(attempts, derivationAttempt{
			xpub: bip84Xpub,
			path: fmt.Sprintf("m/84'/%d'/0'", coinType),
		})
	}

	// 3. Try BIP49 (nested segwit single-sig)
	if bip49Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeSegWitNested); err == nil {
		attempts = append(attempts, derivationAttempt{
			xpub: bip49Xpub,
			path: fmt.Sprintf("m/49'/%d'/0'", coinType),
		})
	}

	// 4. Try BIP44 (legacy single-sig)
	if bip44Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeLegacy); err == nil {
		attempts = append(attempts, derivationAttempt{
			xpub: bip44Xpub,
			path: fmt.Sprintf("m/44'/%d'/0'", coinType),
		})
	}

	// Find ALL matching cosigners by fingerprint OR by xpub (trying all derivation paths)
	// Important: same seed may match multiple cosigners (e.g., user used same seed for all)
	m.dataMu.Lock()
	defer m.dataMu.Unlock()

	matchedCount := 0
	firstMatchIndex := -1

	for i := range m.data.Multisig.Cosigners {
		cs := &m.data.Multisig.Cosigners[i]

		// Skip if already has a mnemonic (already imported)
		if cs.Mnemonic != "" {
			continue
		}

		// Match by fingerprint (if available)
		fingerprintMatch := cs.Fingerprint != "" && cs.Fingerprint == fingerprint

		// Try each derivation path for xpub matching
		var matchedPath string
		for _, attempt := range attempts {
			if cs.Xpub == attempt.xpub {
				matchedPath = attempt.path
				break
			}
		}
		xpubMatch := matchedPath != ""

		if fingerprintMatch || xpubMatch {
			// Found a match - update the cosigner
			cs.Mnemonic = mnemonic
			cs.IsMine = true
			cs.DeviceType = "local"

			// Store the actual derivation path used (important for signing!)
			if matchedPath != "" {
				cs.ActualDerivation = matchedPath
			} else {
				// Matched by fingerprint, assume BIP48 (correct path)
				cs.ActualDerivation = fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType)
			}

			// Also update the fingerprint if it was missing
			if cs.Fingerprint == "" {
				cs.Fingerprint = fingerprint
			}

			// Build key origin if missing (use the actual derivation path)
			if cs.KeyOrigin == "" {
				pathWithoutM := cs.ActualDerivation[2:] // Remove "m/"
				pathWithoutM = strings.ReplaceAll(pathWithoutM, "'", "h") // Use 'h' notation
				cs.KeyOrigin = fmt.Sprintf("%s/%s", fingerprint, pathWithoutM)
			}

			matchedCount++
			if firstMatchIndex == -1 {
				firstMatchIndex = i
			}
			// Continue to check other cosigners - don't return early!
		}
	}

	// Save if we matched any cosigners
	if matchedCount > 0 {
		if err := m.saveWalletTracked(m.data.Name, m.data, m.password); err != nil {
			return -1, fmt.Errorf("failed to save wallet: %w", err)
		}
		return firstMatchIndex, nil
	}

	// No match found - list what we tried and what we're looking for
	var triedPaths []string
	for _, attempt := range attempts {
		xp := attempt.xpub
		if len(xp) > 20 {
			xp = xp[:10] + "..." + xp[len(xp)-10:]
		}
		triedPaths = append(triedPaths, fmt.Sprintf("  %s: %s", attempt.path, xp))
	}

	var cosignerInfo []string
	for i, cs := range m.data.Multisig.Cosigners {
		fp := cs.Fingerprint
		if fp == "" {
			fp = "(none)"
		}
		xp := cs.Xpub
		if len(xp) > 20 {
			xp = xp[:10] + "..." + xp[len(xp)-10:]
		}
		cosignerInfo = append(cosignerInfo, fmt.Sprintf("  Cosigner %d: fp=%s, xpub=%s", i+1, fp, xp))
	}

	return -1, fmt.Errorf("seed (fp=%s) does not match any cosigner.\n\nTried derivation paths:\n%s\n\nCosigners in wallet:\n%s",
		fingerprint, strings.Join(triedPaths, "\n"), strings.Join(cosignerInfo, "\n"))
}

// GetCosigners returns the list of cosigners for the current multisig wallet
func (m *WalletManager) GetCosigners() []MultisigCosignerConfig {
	if !m.unlocked || !m.IsMultisig() {
		return nil
	}
	return m.data.Multisig.Cosigners
}

// UpdateCosigner updates a cosigner's name and/or fingerprint
// Pass empty string to leave a field unchanged
func (m *WalletManager) UpdateCosigner(index int, name, fingerprint string) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}
	if !m.IsMultisig() {
		return fmt.Errorf("not a multisig wallet")
	}

	m.dataMu.Lock()
	defer m.dataMu.Unlock()

	if index < 0 || index >= len(m.data.Multisig.Cosigners) {
		return fmt.Errorf("invalid cosigner index: %d", index)
	}

	cs := &m.data.Multisig.Cosigners[index]

	if name != "" {
		cs.Name = name
	}

	if fingerprint != "" {
		// Validate fingerprint format (8 hex chars)
		if len(fingerprint) != 8 {
			return fmt.Errorf("fingerprint must be 8 hex characters")
		}
		for _, c := range fingerprint {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return fmt.Errorf("fingerprint must be hexadecimal")
			}
		}
		cs.Fingerprint = strings.ToLower(fingerprint)

		// Update key origin if we now have a fingerprint
		if cs.KeyOrigin == "" {
			scriptType := MultisigScriptType(m.data.Multisig.ScriptType)
			coinType := uint32(1) // testnet
			if m.data.Network == "mainnet" {
				coinType = 0
			}
			cs.KeyOrigin = fmt.Sprintf("%s/48h/%dh/0h/%dh", cs.Fingerprint, coinType, scriptType)
		}
	}

	// Save the wallet
	if err := m.saveWalletTracked(m.data.Name, m.data, m.password); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// LocalWalletMatch represents a local wallet that matches a cosigner
type LocalWalletMatch struct {
	WalletName     string // Name of the local wallet file
	CosignerIndex  int    // Which cosigner it matches
	DerivationPath string // The derivation path that matched (BIP84, BIP48, etc.)
	Fingerprint    string // Master fingerprint from the seed
}

// ScanLocalWalletsForCosigners scans other wallets in ~/.haxwallet and tries to
// match their seeds to cosigners in the current multisig wallet.
// This helps when user created multisig from single-sig wallet xpubs.
// Returns list of matches found. Does NOT import seeds - call ImportCosignerSeed for that.
func (m *WalletManager) ScanLocalWalletsForCosigners(password string) ([]LocalWalletMatch, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}
	if !m.IsMultisig() {
		return nil, fmt.Errorf("not a multisig wallet")
	}

	// Get list of all wallets
	wallets, err := m.storage.ListWallets()
	if err != nil {
		return nil, fmt.Errorf("failed to list wallets: %w", err)
	}

	currentWalletName := m.data.Name
	isTestnet := m.data.Network == "testnet"
	coinType := uint32(0)
	if isTestnet {
		coinType = 1
	}
	scriptType := MultisigScriptType(m.data.Multisig.ScriptType)

	var matches []LocalWalletMatch

	// Try each wallet
	for _, walletName := range wallets {
		// Skip the current multisig wallet
		if walletName == currentWalletName {
			continue
		}

		// Try to load the wallet with the provided password
		walletData, err := m.storage.LoadWallet(walletName, password)
		if err != nil {
			// Wrong password or corrupt - skip
			continue
		}

		// Skip multisig wallets (we only want single-sig with seeds)
		if walletData.Multisig != nil {
			continue
		}

		// Skip wallets without mnemonic
		if walletData.Mnemonic == "" {
			continue
		}

		// Derive xpubs at various paths from this seed
		seed, err := MnemonicToSeed(walletData.Mnemonic, "")
		if err != nil {
			continue
		}

		hdWallet, err := NewHDWallet(seed, isTestnet)
		if err != nil {
			continue
		}

		fingerprint, err := hdWallet.GetMasterFingerprintHex()
		if err != nil {
			continue
		}

		// Build list of xpubs to try
		type attempt struct {
			xpub string
			path string
		}
		var attempts []attempt

		// BIP48 (correct for multisig)
		if bip48Xpub, err := hdWallet.GetMultisigAccountXpub(BIP48Path{
			CoinType:   coinType,
			Account:    0,
			ScriptType: scriptType,
		}); err == nil {
			attempts = append(attempts, attempt{bip48Xpub, fmt.Sprintf("m/48'/%d'/0'/%d'", coinType, scriptType)})
		}

		// BIP84 (native segwit single-sig)
		if bip84Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeSegWitNative); err == nil {
			attempts = append(attempts, attempt{bip84Xpub, fmt.Sprintf("m/84'/%d'/0'", coinType)})
		}

		// BIP49 (nested segwit)
		if bip49Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeSegWitNested); err == nil {
			attempts = append(attempts, attempt{bip49Xpub, fmt.Sprintf("m/49'/%d'/0'", coinType)})
		}

		// BIP44 (legacy)
		if bip44Xpub, err := hdWallet.GetExtendedPublicKey(0, AddressTypeLegacy); err == nil {
			attempts = append(attempts, attempt{bip44Xpub, fmt.Sprintf("m/44'/%d'/0'", coinType)})
		}

		// Check each cosigner
		for i, cs := range m.data.Multisig.Cosigners {
			// Skip already matched cosigners
			if cs.IsMine && cs.Mnemonic != "" {
				continue
			}

			// Try each derivation path
			for _, att := range attempts {
				if cs.Xpub == att.xpub {
					matches = append(matches, LocalWalletMatch{
						WalletName:     walletName,
						CosignerIndex:  i,
						DerivationPath: att.path,
						Fingerprint:    fingerprint,
					})
					break // Found match for this cosigner, move to next
				}
			}

			// Also check fingerprint match if available
			if cs.Fingerprint != "" && cs.Fingerprint == fingerprint {
				// Found by fingerprint but maybe different path - find which one
				alreadyAdded := false
				for _, match := range matches {
					if match.CosignerIndex == i && match.WalletName == walletName {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					// We matched by fingerprint, determine path
					for _, att := range attempts {
						if cs.Xpub == att.xpub {
							matches = append(matches, LocalWalletMatch{
								WalletName:     walletName,
								CosignerIndex:  i,
								DerivationPath: att.path,
								Fingerprint:    fingerprint,
							})
							break
						}
					}
				}
			}
		}
	}

	return matches, nil
}

// ImportCosignerFromLocalWallet imports a seed from another local wallet to a cosigner
// and sets the cosigner's name to the source wallet name
func (m *WalletManager) ImportCosignerFromLocalWallet(walletName, password string) (int, error) {
	if !m.unlocked {
		return -1, fmt.Errorf("wallet is locked")
	}
	if !m.IsMultisig() {
		return -1, fmt.Errorf("not a multisig wallet")
	}

	// Load the source wallet
	walletData, err := m.storage.LoadWallet(walletName, password)
	if err != nil {
		return -1, fmt.Errorf("failed to load wallet '%s': %w", walletName, err)
	}

	if walletData.Mnemonic == "" {
		return -1, fmt.Errorf("wallet '%s' has no seed phrase", walletName)
	}

	// Use ImportCosignerSeed which handles multi-path matching
	index, err := m.ImportCosignerSeed(walletData.Mnemonic)
	if err != nil {
		return -1, err
	}

	// Update cosigner name to source wallet name
	if index >= 0 && index < len(m.data.Multisig.Cosigners) {
		m.dataMu.Lock()
		m.data.Multisig.Cosigners[index].Name = walletName
		m.dataMu.Unlock()

		// Save updated name (ignore error - seed import succeeded, name is non-critical)
		_ = m.saveWalletTracked(m.data.Name, m.data, m.password)
	}

	return index, nil
}

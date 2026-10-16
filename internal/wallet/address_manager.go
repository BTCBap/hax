package wallet

import (
	"fmt"
	"log"
	"strings"

	"github.com/BTCBap/hax/internal/api"
)

// resolvePathPrefix returns the expected path prefix for addresses in an account.
// Uses the custom derivation path if set, otherwise derives from address type.
// For multisig wallets, returns "m/48'".
func resolvePathPrefix(account *AccountData, walletData *WalletData) string {
	if walletData.Multisig != nil {
		return "m/48'"
	}
	if account.CustomDerivationPath != "" {
		return account.CustomDerivationPath
	}
	if walletData.HardwareWallet != nil {
		if path := keyOriginToPath(walletData.HardwareWallet.KeyOrigin); path != "" {
			return path
		}
	}
	switch account.AddressType {
	case "segwit_nested":
		return "m/49'"
	case "segwit_native":
		return "m/84'"
	case "taproot":
		return "m/86'"
	default:
		return "m/44'"
	}
}

// deriveAddress derives an address for the given account, index, and chain (receive/change).
// Should be called WITHOUT dataMu held to avoid blocking other goroutines during derivation.
// Reads m.data.Multisig, m.data.HardwareWallet, and m.data.Network without lock;
// these fields are immutable once the wallet is unlocked.
func (m *WalletManager) deriveAddress(accountIndex, addressIndex uint32, isChange bool, addrTypeStr string, customPath string) (string, string, error) {
	change := uint32(0)
	if isChange {
		change = 1
	}
	isTestnet := m.data != nil && m.data.Network == "testnet"

	if m.data.Multisig != nil {
		addr, _, err := m.GetMultisigAddress(isChange, addressIndex)
		if err != nil {
			return "", "", fmt.Errorf("failed to derive multisig address: %w", err)
		}
		scriptType := m.data.Multisig.ScriptType
		pathStr := fmt.Sprintf("m/48'/1'/0'/%d'/%d/%d", scriptType, change, addressIndex)
		return addr, pathStr, nil
	}

	if m.data.HardwareWallet != nil {
		hwConfig := m.data.HardwareWallet
		addrType := ParseAddressTypeString(hwConfig.AddressType)
		network := m.getNetwork()
		addr, err := DeriveAddressFromXpub(hwConfig.Xpub, change, addressIndex, addrType, network)
		if err != nil {
			return "", "", fmt.Errorf("failed to derive address from xpub: %w", err)
		}
		// Use KeyOrigin for path if available, otherwise construct from address type
		accountPath := ResolveAccountPath(customPath, addrType, isTestnet, 0)
		if accountPath == "" || (customPath == "" && hwConfig.KeyOrigin != "") {
			if koPath := keyOriginToPath(hwConfig.KeyOrigin); koPath != "" {
				accountPath = koPath
			}
		}
		pathStr := BuildFullPath(accountPath, change, addressIndex)
		return addr, pathStr, nil
	}

	// Software wallet
	if m.wallet == nil {
		return "", "", fmt.Errorf("HD wallet not initialized")
	}

	addrType := ParseAddressTypeString(addrTypeStr)
	if addrTypeStr != "legacy" && addrTypeStr != "segwit_nested" &&
		addrTypeStr != "segwit_native" && addrTypeStr != "taproot" {
		addrType = AddressTypeLegacy
	}

	// If custom path is set, parse it and use those components
	if customPath != "" {
		purpose, coinType, account, err := ParseAccountPath(customPath)
		if err != nil {
			return "", "", fmt.Errorf("invalid custom derivation path %q: %w", customPath, err)
		}
		path := BIP44Path{
			Purpose:      purpose,
			CoinType:     coinType,
			Account:      account,
			Change:       change,
			AddressIndex: addressIndex,
		}
		addr, err := m.wallet.GetAddress(path, addrType)
		if err != nil {
			return "", "", fmt.Errorf("failed to derive address at custom path: %w", err)
		}
		return addr, path.String(), nil
	}

	path := PathForAddressType(addrType, isTestnet, accountIndex, change, addressIndex)
	addr, err := m.wallet.GetAddress(path, addrType)
	if err != nil {
		return "", "", fmt.Errorf("failed to derive address: %w", err)
	}
	return addr, path.String(), nil
}

// registerAddress stores an address in the account data and updates the cache.
// MUST be called WITH dataMu write lock held. Does NOT perform I/O — callers
// must call persistAndSubscribe after releasing the write lock.
func (m *WalletManager) registerAddress(accountIndex uint32, addr AddressData) {
	account := &m.data.Accounts[accountIndex]
	account.Addresses = append(account.Addresses, addr)

	if m.cache != nil {
		scripthash, _ := m.backend.AddressToScripthash(addr.Address)
		m.cache.SetAddress(addr.Address, &AddressCacheEntry{
			Address:    addr.Address,
			Scripthash: scripthash,
		}, scripthash)
	}
}

// persistAndSubscribe saves the wallet to disk and subscribes to Electrum
// notifications for the given address. MUST be called WITHOUT dataMu write
// lock held — acquires RLock internally for the save.
func (m *WalletManager) persistAndSubscribe(address string) {
	m.dataMu.RLock()
	err := m.saveWalletTracked(m.data.Name, m.data)
	m.dataMu.RUnlock()
	if err != nil {
		log.Printf("[WALLET] persistAndSubscribe: SaveWallet failed: %v", err)
	}

	m.backend.SubscribeAddress(address)
}

// GetNewAddress generates a new receiving address for an account
func (m *WalletManager) GetNewAddress(accountIndex uint32) (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.Unlock()
		return "", fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]

	// Find next receive address index
	addressIndex := uint32(0)
	for _, addr := range account.Addresses {
		if !addr.Change && addr.Index >= addressIndex {
			addressIndex = addr.Index + 1
		}
	}

	addrTypeStr := account.AddressType
	customPath := account.CustomDerivationPath
	m.dataMu.Unlock()

	// Derive address without lock held
	address, pathStr, err := m.deriveAddress(accountIndex, addressIndex, false, addrTypeStr, customPath)
	if err != nil {
		return "", err
	}

	// Register address under lock (check for duplicate after reacquiring)
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for _, a := range m.data.Accounts[accountIndex].Addresses {
			if !a.Change && a.Index == addressIndex {
				// Already registered by concurrent caller
				m.dataMu.Unlock()
				return address, nil
			}
		}
	}
	m.registerAddress(accountIndex, AddressData{
		Index:   addressIndex,
		Address: address,
		Path:    pathStr,
		Change:  false,
		Used:    false,
	})
	m.dataMu.Unlock()

	// I/O outside write lock
	m.persistAndSubscribe(address)

	// Update cache indexes
	if m.cache != nil {
		m.cache.SetNextReceiveIndex(addressIndex + 1)
	}

	return address, nil
}

// GetNewChangeAddress generates a new change address for an account
func (m *WalletManager) GetNewChangeAddress(accountIndex uint32) (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.Unlock()
		return "", fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]

	// Find next change address index
	addressIndex := uint32(0)
	for _, addr := range account.Addresses {
		if addr.Change && addr.Index >= addressIndex {
			addressIndex = addr.Index + 1
		}
	}

	addrTypeStr := account.AddressType
	customPath := account.CustomDerivationPath
	m.dataMu.Unlock()

	// Derive address without lock held
	address, pathStr, err := m.deriveAddress(accountIndex, addressIndex, true, addrTypeStr, customPath)
	if err != nil {
		return "", err
	}

	// Register address under lock (check for duplicate after reacquiring)
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for _, a := range m.data.Accounts[accountIndex].Addresses {
			if a.Change && a.Index == addressIndex {
				// Already registered by concurrent caller
				m.dataMu.Unlock()
				return address, nil
			}
		}
	}
	m.registerAddress(accountIndex, AddressData{
		Index:   addressIndex,
		Address: address,
		Path:    pathStr,
		Change:  true,
		Used:    false,
	})
	m.dataMu.Unlock()

	// I/O outside write lock
	m.persistAndSubscribe(address)

	// Update cache indexes
	if m.cache != nil {
		m.cache.SetNextChangeIndex(addressIndex + 1)
	}

	return address, nil
}

// GetCurrentChangeAddress returns the current unused change address
// If no unused change address exists, it generates a new one
// Returns (address, path, index, error)
func (m *WalletManager) GetCurrentChangeAddress(accountIndex uint32) (string, string, uint32, error) {
	if !m.unlocked {
		return "", "", 0, fmt.Errorf("wallet is locked")
	}

	// First try to find an existing unused change address (read-only)
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return "", "", 0, fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]

	// Find the first unused change address (lowest index)
	for i := 0; i < len(account.Addresses); i++ {
		addr := &account.Addresses[i]
		if addr.Change && !addr.Used {
			result := AddressData{Address: addr.Address, Path: addr.Path, Index: addr.Index}
			m.dataMu.RUnlock()
			return result.Address, result.Path, result.Index, nil
		}
	}
	m.dataMu.RUnlock()

	// No unused change address found, generate a new one
	// GetNewChangeAddress acquires its own lock
	address, err := m.GetNewChangeAddress(accountIndex)
	if err != nil {
		return "", "", 0, err
	}

	// Find the newly created address
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	account = &m.data.Accounts[accountIndex]
	for i := len(account.Addresses) - 1; i >= 0; i-- {
		addr := &account.Addresses[i]
		if addr.Address == address {
			return addr.Address, addr.Path, addr.Index, nil
		}
	}

	return "", "", 0, fmt.Errorf("failed to retrieve newly created change address")
}

// GetCurrentReceiveAddress returns the current unused receive address
// If no unused address exists, it generates a new one
// Cross-checks the address cache to skip externally-used addresses.
// Fast (no network calls) — use VerifyReceiveAddressCache for network verification.
// Returns (address, path, index, error)
func (m *WalletManager) GetCurrentReceiveAddress(accountIndex uint32) (string, string, uint32, error) {
	if !m.unlocked {
		return "", "", 0, fmt.Errorf("wallet is locked")
	}

	addr, path, index, found, err := m.findUnusedReceiveAddress(accountIndex)
	if err != nil {
		return "", "", 0, err
	}

	if !found {
		return m.generateAndReturnNewAddress(accountIndex)
	}

	return addr, path, index, nil
}

// VerifyReceiveAddressCache checks receive address candidates against the network
// and updates the cache for any that are used on-chain. Call this async before
// GetCurrentReceiveAddress to ensure the cache is warm.
func (m *WalletManager) VerifyReceiveAddressCache(accountIndex uint32) {
	if m.backend == nil || m.cache == nil {
		return
	}

	for attempts := 0; attempts < 10; attempts++ {
		addr, _, _, found, err := m.findUnusedReceiveAddress(accountIndex)
		if err != nil || !found {
			return
		}

		// Already verified — cache has an entry for this address
		if entry := m.cache.GetAddress(addr); entry != nil {
			return
		}

		info, infoErr := m.backend.GetAddressInfo(addr)
		if infoErr != nil {
			return
		}
		if info.ChainStats.FundedTxoCount > 0 || info.MempoolStats.FundedTxoCount > 0 {
			balance := info.ChainStats.FundedTxoSum - info.ChainStats.SpentTxoSum
			m.cache.UpdateAddressHistory(addr, true, balance)
			continue
		}
		// Mark as verified-unused so we skip the network call next time
		m.cache.UpdateAddressHistory(addr, false, 0)
		return
	}
}

// findUnusedReceiveAddress searches wallet data for the lowest-index unused receive address.
// Cross-checks the address cache to skip addresses with on-chain history (catches
// externally-used addresses after background sync has populated the cache).
func (m *WalletManager) findUnusedReceiveAddress(accountIndex uint32) (string, string, uint32, bool, error) {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()

	if int(accountIndex) >= len(m.data.Accounts) {
		return "", "", 0, false, fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]
	expectedPathPrefix := resolvePathPrefix(account, m.data)

	var lowestUnused *AddressData
	for i := 0; i < len(account.Addresses); i++ {
		addr := &account.Addresses[i]
		if !addr.Change && !addr.Used && strings.HasPrefix(addr.Path, expectedPathPrefix) {
			// Also check the cache — addr.Used may be stale if the address
			// was spent from another wallet before sync updates the flag
			if m.cache != nil {
				if entry := m.cache.GetAddress(addr.Address); entry != nil && entry.HasHistory {
					continue
				}
			}
			if lowestUnused == nil || addr.Index < lowestUnused.Index {
				lowestUnused = addr
			}
		}
	}

	if lowestUnused == nil {
		return "", "", 0, false, nil
	}
	return lowestUnused.Address, lowestUnused.Path, lowestUnused.Index, true, nil
}

// generateAndReturnNewAddress creates a new address and returns its details
func (m *WalletManager) generateAndReturnNewAddress(accountIndex uint32) (string, string, uint32, error) {
	address, err := m.GetNewAddress(accountIndex)
	if err != nil {
		return "", "", 0, err
	}

	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	if int(accountIndex) < len(m.data.Accounts) {
		account := &m.data.Accounts[accountIndex]
		for i := len(account.Addresses) - 1; i >= 0; i-- {
			if account.Addresses[i].Address == address {
				return address, account.Addresses[i].Path, account.Addresses[i].Index, nil
			}
		}
	}
	return address, "", 0, nil
}

// GetAddressByIndex returns an address at a specific index
// If the address doesn't exist, it derives it
func (m *WalletManager) GetAddressByIndex(accountIndex uint32, addressIndex uint32) (string, string, error) {
	if !m.unlocked {
		return "", "", fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.Unlock()
		return "", "", fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]
	expectedPathPrefix := resolvePathPrefix(account, m.data)

	for _, addr := range account.Addresses {
		if !addr.Change && addr.Index == addressIndex && strings.HasPrefix(addr.Path, expectedPathPrefix) {
			m.dataMu.Unlock()
			return addr.Address, addr.Path, nil
		}
	}

	addrTypeStr := account.AddressType
	customPath := account.CustomDerivationPath
	m.dataMu.Unlock()

	// Derive address without lock held
	address, pathStr, err := m.deriveAddress(accountIndex, addressIndex, false, addrTypeStr, customPath)
	if err != nil {
		return "", "", err
	}

	// Register address under lock (check for duplicate after reacquiring)
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for _, a := range m.data.Accounts[accountIndex].Addresses {
			if !a.Change && a.Index == addressIndex {
				m.dataMu.Unlock()
				return address, pathStr, nil
			}
		}
	}
	m.registerAddress(accountIndex, AddressData{
		Index:   addressIndex,
		Address: address,
		Path:    pathStr,
		Change:  false,
		Used:    false,
	})
	m.dataMu.Unlock()

	// I/O outside write lock
	m.persistAndSubscribe(address)

	return address, pathStr, nil
}

// getChangeAddressByIndex returns a change address at a specific index
// If the address doesn't exist, it derives it (similar to GetAddressByIndex but for change)
func (m *WalletManager) getChangeAddressByIndex(accountIndex uint32, addressIndex uint32) (string, error) {
	if !m.unlocked {
		return "", fmt.Errorf("wallet is locked")
	}

	m.dataMu.Lock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.Unlock()
		return "", fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]

	// Check for existing change address at this index
	for _, addr := range account.Addresses {
		if addr.Change && addr.Index == addressIndex {
			m.dataMu.Unlock()
			return addr.Address, nil
		}
	}

	addrTypeStr := account.AddressType
	customPath := account.CustomDerivationPath
	m.dataMu.Unlock()

	// Derive address without lock held
	address, pathStr, err := m.deriveAddress(accountIndex, addressIndex, true, addrTypeStr, customPath)
	if err != nil {
		return "", err
	}

	// Register address under lock (check for duplicate after reacquiring)
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for _, a := range m.data.Accounts[accountIndex].Addresses {
			if a.Change && a.Index == addressIndex {
				m.dataMu.Unlock()
				return address, nil
			}
		}
	}
	m.registerAddress(accountIndex, AddressData{
		Index:   addressIndex,
		Address: address,
		Path:    pathStr,
		Change:  true,
		Used:    false,
	})
	m.dataMu.Unlock()

	// I/O outside write lock
	m.persistAndSubscribe(address)

	return address, nil
}

// GetAllReceiveAddresses returns all receive addresses for an account
func (m *WalletManager) GetAllReceiveAddresses(accountIndex uint32) ([]AddressData, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	if int(accountIndex) >= len(m.data.Accounts) {
		return nil, fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]
	var receiveAddresses []AddressData

	for _, addr := range account.Addresses {
		if !addr.Change {
			receiveAddresses = append(receiveAddresses, addr)
		}
	}

	return receiveAddresses, nil
}

// GetAddressFromCache returns cached address info, or nil if not cached
func (m *WalletManager) GetAddressFromCache(address string) *AddressCacheEntry {
	if m.cache == nil {
		return nil
	}
	return m.cache.GetAddress(address)
}

// GetCachedReceiveAddresses returns all cached receive addresses sorted by index
func (m *WalletManager) GetCachedReceiveAddresses() []*AddressCacheEntry {
	if m.cache == nil {
		return nil
	}
	addresses := m.cache.GetReceiveAddresses()

	// Sort by index
	for i := 0; i < len(addresses)-1; i++ {
		for j := i + 1; j < len(addresses); j++ {
			if addresses[i].Index > addresses[j].Index {
				addresses[i], addresses[j] = addresses[j], addresses[i]
			}
		}
	}

	return addresses
}

// GetNextUnusedReceiveIndex returns the index of the next unused receive address
func (m *WalletManager) GetNextUnusedReceiveIndex() uint32 {
	if m.cache == nil {
		return 0
	}
	return m.cache.GetNextReceiveIndex()
}

// IsAddressScanned returns true if addresses have been scanned
func (m *WalletManager) IsAddressScanned() bool {
	if m.cache == nil {
		return false
	}
	return m.cache.IsAddressScanned()
}

// SyncAddressUsage scans addresses and marks them as used based on blockchain history
// Uses ChainStats.FundedTxoCount to determine if an address has ever received funds
// Implements gap limit of 20 to avoid scanning too many unused addresses
func (m *WalletManager) SyncAddressUsage(accountIndex uint32) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}

	// Validate account with lock
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return fmt.Errorf("account not found")
	}
	m.dataMu.RUnlock()

	gapLimit := m.getGapLimit()

	// Sync receive addresses
	consecutiveUnused := 0
	maxIndex := uint32(gapLimit * 5)
	for currentIndex := uint32(0); currentIndex < maxIndex; currentIndex++ {
		// Get or derive address at current index (has its own locking)
		address, _, err := m.GetAddressByIndex(accountIndex, currentIndex)
		if err != nil {
			break
		}

		// Sync this address by string (avoids holding pointer across network call)
		used, err := m.syncAddressByString(accountIndex, address, false)
		if err != nil {
			// Continue on error
			continue
		}

		if !used {
			consecutiveUnused++
		} else {
			consecutiveUnused = 0
		}

		if consecutiveUnused >= gapLimit {
			break
		}
	}

	// Change addresses also need gap limit discovery for UTXO detection
	consecutiveUnused = 0
	for currentIndex := uint32(0); currentIndex < maxIndex; currentIndex++ {
		address, err := m.getChangeAddressByIndex(accountIndex, currentIndex)
		if err != nil {
			break
		}

		used, err := m.syncAddressByString(accountIndex, address, true)
		if err != nil {
			continue
		}

		if !used {
			consecutiveUnused++
		} else {
			consecutiveUnused = 0
		}

		if consecutiveUnused >= gapLimit {
			break
		}
	}

	m.dataMu.RLock()
	name := m.data.Name
	data := m.data
	err := m.saveWalletTracked(name, data)
	m.dataMu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to save wallet after sync: %w", err)
	}

	return nil
}

// syncAddressByString syncs a single address and returns whether it's used.
func (m *WalletManager) syncAddressByString(accountIndex uint32, address string, isChange bool) (bool, error) {
	var hasHistory bool
	var balance api.Satoshi

	var err error
	hasHistory, err = m.backend.GetAddressHistory(address)
	if err != nil {
		return false, fmt.Errorf("failed to get address history: %w", err)
	}
	if hasHistory {
		balance, _ = m.backend.GetAddressBalance(address)
	}

	// Update address data with lock
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for i := range m.data.Accounts[accountIndex].Addresses {
			addr := &m.data.Accounts[accountIndex].Addresses[i]
			if addr.Address == address && addr.Change == isChange {
				if hasHistory {
					addr.Used = true
				}
				break
			}
		}
	}
	m.dataMu.Unlock()

	// Update cache entry
	if m.cache != nil {
		m.cache.UpdateAddressHistory(address, hasHistory, balance)
	}

	return hasHistory, nil
}

// MarkAddressAsUsed marks a specific address as used after verifying on-chain
// Used after broadcasting transactions to mark change addresses
func (m *WalletManager) MarkAddressAsUsed(accountIndex uint32, address string) error {
	if !m.unlocked {
		return fmt.Errorf("wallet is locked")
	}

	// Check account exists (under lock)
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return fmt.Errorf("account not found")
	}

	// Check address exists in account
	found := false
	for _, addr := range m.data.Accounts[accountIndex].Addresses {
		if addr.Address == address {
			found = true
			break
		}
	}
	m.dataMu.RUnlock()

	if !found {
		return fmt.Errorf("address not found: %s", address)
	}

	// Verify the address has been funded on-chain or in mempool (outside lock)
	shouldMarkUsed := false
	addressInfo, err := m.backend.GetAddressInfo(address)
	if err != nil {
		// If API fails, mark as used anyway (conservative approach)
		// It will be verified on next wallet unlock
		shouldMarkUsed = true
	} else {
		// Mark as used if it has received funds (confirmed or pending in mempool)
		if addressInfo.ChainStats.FundedTxoCount > 0 || addressInfo.MempoolStats.FundedTxoCount > 0 {
			shouldMarkUsed = true
		}
	}

	// Mutate under write lock
	m.dataMu.Lock()
	if int(accountIndex) < len(m.data.Accounts) {
		for i := range m.data.Accounts[accountIndex].Addresses {
			if m.data.Accounts[accountIndex].Addresses[i].Address == address {
				if shouldMarkUsed {
					m.data.Accounts[accountIndex].Addresses[i].Used = true
				}
				break
			}
		}
	}
	m.dataMu.Unlock()

	// Save under read lock (allows concurrent readers, blocks during marshal)
	m.dataMu.RLock()
	err = m.saveWalletTracked(m.data.Name, m.data)
	m.dataMu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}

	return nil
}

// GetAddressStatus returns the status of an address (SPENT, USED, or UNUSED)
// SPENT: Has transactions where it was used as an input (sent from)
// USED: Has received funds but never sent
// UNUSED: No transaction history
func (m *WalletManager) GetAddressStatus(address string) string {
	// Check cache for SPENT status and address info
	if m.cache != nil {
		// Check ALL transactions for spent inputs (not just cached addresses)
		// Use display accessor to include persistent (pre-sync) data
		for _, tx := range m.cache.GetTransactionsForDisplay() {
			for _, vin := range tx.Vin {
				if vin.Prevout != nil && vin.Prevout.ScriptpubkeyAddress == address {
					return "SPENT"
				}
			}
		}

		// Check if address is in cache with history
		if entry := m.cache.GetAddress(address); entry != nil {
			if entry.HasHistory {
				return "USED"
			}
			return "UNUSED"
		}
	}

	// Fall back to wallet data if not in cache
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()

	if m.data == nil {
		return "UNKNOWN"
	}

	for _, account := range m.data.Accounts {
		for _, addr := range account.Addresses {
			if addr.Address == address {
				if addr.Used {
					return "USED"
				}
				return "UNUSED"
			}
		}
	}

	return "UNKNOWN"
}

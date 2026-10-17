package wallet

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BTCBap/hax/internal/api"
)

// RefreshOptions controls which parts of the cache refresh to perform
type RefreshOptions struct {
	Quiet            bool // Don't show status messages (for periodic background refreshes)
	SkipAddressScan  bool // Skip SyncAddressUsage (faster when addresses haven't changed)
	SkipFeeEstimates bool // Skip GetFeeEstimates (not needed after send)
}

// UTXOWithPath extends api.UTXO with derivation path information for signing
type UTXOWithPath struct {
	api.UTXO
	Address string // The address this UTXO belongs to
	Path    string // BIP44 derivation path for this address
}

// migrateMultisigPaths fixes old "multisig/X/Y" paths to proper BIP48 format
func (m *WalletManager) migrateMultisigPaths() {
	if m.data == nil || m.data.Multisig == nil {
		return
	}

	scriptType := m.data.Multisig.ScriptType
	migrated := false

	for i := range m.data.Accounts {
		account := &m.data.Accounts[i]
		for j := range account.Addresses {
			addr := &account.Addresses[j]
			// Check for old format: "multisig/0/0" or "multisig/1/0"
			if strings.HasPrefix(addr.Path, "multisig/") {
				changeVal := 0
				if addr.Change {
					changeVal = 1
				}
				// Convert to proper BIP48: m/48'/1'/0'/script'/change/index
				addr.Path = fmt.Sprintf("m/48'/1'/0'/%d'/%d/%d", scriptType, changeVal, addr.Index)
				migrated = true
			}
		}
	}

	// Save wallet if we migrated any paths
	if migrated && m.hasKey() {
		_ = m.saveWalletTracked(m.data.Name, m.data) // best-effort cache persist; retried on next save
	}
}

// loadPersistentCache loads cached data from wallet file for instant display
func (m *WalletManager) loadPersistentCache() {
	if m.data == nil || m.data.Cache == nil || m.cache == nil {
		return
	}

	pc := m.data.Cache

	// Convert persistent transactions to runtime format
	txs := make([]api.Transaction, 0, len(pc.Transactions))
	for _, ct := range pc.Transactions {
		txs = append(txs, api.Transaction{
			TxID: ct.TxID,
			Status: api.Status{
				BlockHeight: ct.BlockHeight,
				BlockTime:   ct.BlockTime,
				Confirmed:   ct.Confirmed,
			},
		})
	}

	// Convert persistent UTXOs to runtime format
	utxos := make([]api.UTXO, 0, len(pc.UTXOs))
	for _, cu := range pc.UTXOs {
		utxos = append(utxos, api.UTXO{
			TxID:  cu.TxID,
			Vout:  cu.Vout,
			Value: cu.Value,
			Status: api.Status{
				BlockHeight: cu.BlockHeight,
				Confirmed:   cu.Confirmed,
			},
		})
	}

	// Calculate balance from cached UTXOs
	var balance api.Satoshi
	for _, utxo := range utxos {
		balance += utxo.Value
	}

	// Populate runtime cache for instant display only
	// NOTE: We intentionally do NOT set the *UpdatedAt timestamps here.
	// This ensures GetUTXOs() and other methods will fetch fresh data from
	// the network instead of returning potentially stale cached data.
	// The data here is only used for initial UI display via syncStateFromCache().
	// LoadPersistentData does NOT set *UpdatedAt - forces fresh fetch when GetUTXOs() is called
	m.cache.LoadPersistentData(txs, utxos, balance, pc.LastSyncHeight)
}

// syncStateFromCache updates the unified WalletState from the legacy cache
// Called after loading persistent cache for instant UI display
// NOTE: This uses potentially stale data from the previous session
// The actual sync will update with fresh data from the network
func (m *WalletManager) syncStateFromCache() {
	if m.cache == nil || m.state == nil {
		return
	}

	snap := m.cache.Snapshot()

	// Update unified state with cached data for instant display
	m.state.UpdateBalance(snap.Balance)
	m.state.UpdateTransactions(snap.Transactions)
	m.state.UpdateUTXOs(snap.UTXOs)
	if snap.FeeEstimates != nil {
		m.state.UpdateFeeEstimates(snap.FeeEstimates)
	}
	m.state.UpdateAddressIndexes(snap.NextReceiveIndex, snap.NextChangeIndex)
	// NOTE: Don't set status to Synced here - this is cached data from previous session
	// The backgroundSync will set Synced after fetching fresh data from network
	if snap.BlockHeight > 0 {
		m.state.SetStatusWithHeight(SyncStatusConnecting, snap.BlockHeight)
	}
}

// scanAddressesWithContext scans wallet addresses with context cancellation support
// Uses gap limit (20) - scans until 20 consecutive unused addresses are found
func (m *WalletManager) scanAddressesWithContext(ctx context.Context) {
	if m.cache == nil {
		return
	}
	// For single-sig wallets, we need m.wallet; for multisig, we use cosigner xpubs
	if m.wallet == nil && !m.IsMultisig() {
		return
	}

	gapLimit := m.getGapLimit()

	// Scan receive addresses (change=false)
	if ctx.Err() != nil {
		return
	}
	m.scanAddressChainWithContext(ctx, 0, false, gapLimit)

	// Scan change addresses
	if ctx.Err() != nil {
		return
	}
	m.scanAddressChainWithContext(ctx, 0, true, gapLimit)

	// Check for cancellation before finalizing
	if ctx.Err() != nil {
		return
	}

	m.cache.MarkAddressesScanned()

	// Save wallet to persist discovered addresses (hold lock during save)
	m.dataMu.RLock()
	if m.data != nil && m.hasKey() {
		_ = m.saveWalletTracked(m.data.Name, m.data) // best-effort cache persist; retried on next save
	}
	m.dataMu.RUnlock()
}

// scanAddressChainWithContext scans a specific address chain with context cancellation support
func (m *WalletManager) scanAddressChainWithContext(ctx context.Context, account uint32, isChange bool, gapLimit int) {
	consecutiveUnused := 0
	index := uint32(0)

	// Get the account's address type string and custom path for deriveAddress
	var addrTypeStr string
	var customPath string
	m.dataMu.RLock()
	if int(account) < len(m.data.Accounts) {
		addrTypeStr = m.data.Accounts[account].AddressType
		customPath = m.data.Accounts[account].CustomDerivationPath
	}
	m.dataMu.RUnlock()

	for consecutiveUnused < gapLimit {
		// Check for cancellation at start of each iteration
		if ctx.Err() != nil {
			return
		}

		addr, pathStr, err := m.deriveAddress(account, index, isChange, addrTypeStr, customPath)
		if err != nil {
			break
		}

		// Check if address has history
		hasHistory, _ := m.backend.GetAddressHistory(addr)
		var balance api.Satoshi
		if hasHistory {
			balance, _ = m.backend.GetAddressBalance(addr)
		}
		scripthash, _ := m.backend.AddressToScripthash(addr)
		_, _ = m.backend.SubscribeAddress(addr)

		// Cache the address info
		entry := &AddressCacheEntry{
			Address:     addr,
			Scripthash:  scripthash,
			HasHistory:  hasHistory,
			Balance:     balance,
			Index:       index,
			IsChange:    isChange,
			LastChecked: time.Now(),
		}

		m.cache.SetAddress(addr, entry, scripthash)

		// Also add to account.Addresses if not already there (for GetUTXOs to work)
		// Lock dataMu to protect m.data.Accounts modifications (runs in goroutine)
		m.dataMu.Lock()
		if int(account) < len(m.data.Accounts) {
			accountData := &m.data.Accounts[account]
			found := false
			for _, existingAddr := range accountData.Addresses {
				if existingAddr.Address == addr {
					found = true
					// Update Used flag if it changed
					if existingAddr.Used != hasHistory {
						for i := range accountData.Addresses {
							if accountData.Addresses[i].Address == addr {
								accountData.Addresses[i].Used = hasHistory
								break
							}
						}
					}
					break
				}
			}
			if !found {
				accountData.Addresses = append(accountData.Addresses, AddressData{
					Index:   index,
					Address: addr,
					Path:    pathStr,
					Change:  isChange,
					Used:    hasHistory,
				})
			}
		}
		m.dataMu.Unlock()

		m.cache.mu.Lock()
		// Update next unused index
		if !hasHistory {
			if !isChange && m.cache.NextReceiveIndex == 0 && index == 0 {
				// First address is unused, keep NextReceiveIndex at 0
			} else if !isChange && !m.cache.Addresses[addr].HasHistory && index < m.cache.NextReceiveIndex {
				// Don't update if we already have a lower unused index
			} else if !isChange && consecutiveUnused == 0 {
				m.cache.NextReceiveIndex = index
			}
			if isChange && consecutiveUnused == 0 {
				m.cache.NextChangeIndex = index
			}
		}
		m.cache.mu.Unlock()

		if hasHistory {
			consecutiveUnused = 0
		} else {
			consecutiveUnused++
		}

		_ = pathStr

		index++
	}

	m.cache.mu.Lock()
	if !isChange {
		for i := uint32(0); i < index; i++ {
			found := false
			for _, entry := range m.cache.Addresses {
				if !entry.IsChange && entry.Index == i && !entry.HasHistory {
					m.cache.NextReceiveIndex = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	} else {
		// Find first unused change
		for i := uint32(0); i < index; i++ {
			found := false
			for _, entry := range m.cache.Addresses {
				if entry.IsChange && entry.Index == i && !entry.HasHistory {
					m.cache.NextChangeIndex = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	m.cache.mu.Unlock()
}

// StartBackgroundSync starts background data synchronization
func (m *WalletManager) StartBackgroundSync() {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()

	// Stop any existing sync
	if m.syncStopChan != nil {
		close(m.syncStopChan)
		// Note: We don't wait here because we're about to start a new sync
		// and we hold the lock. The old goroutine will exit on channel close.
	}

	stop := make(chan struct{})
	m.syncStopChan = stop

	// Start sync goroutine with WaitGroup tracking. The goroutine gets its
	// own copy of the stop channel: StopBackgroundSync nils the field, and
	// a nil channel would block the select forever (hanging Lock/Stop
	// when it arrives during the initial sync).
	m.syncWg.Add(1)
	go func() {
		defer m.syncWg.Done()
		m.backgroundSync(stop)
	}()
}

// StopBackgroundSync stops the background sync and waits for it to complete
func (m *WalletManager) StopBackgroundSync() {
	m.syncMu.Lock()
	if m.syncStopChan != nil {
		close(m.syncStopChan)
		m.syncStopChan = nil
	}
	m.syncMu.Unlock() // Unlock before waiting to avoid blocking StartBackgroundSync

	// Wait for the background sync goroutine to complete
	m.syncWg.Wait()
}

// backgroundSync performs periodic data synchronization
// Note: Electrum notification channels (scripthash, headers) are handled by the UI layer
// in root.go via StartListeningWithSubscriptions/ContinueListening, which shows toast
// notifications and triggers RefreshCache. This avoids channel read conflicts.
func (m *WalletManager) backgroundSync(stop <-chan struct{}) {
	// Set status to connecting before initial sync
	m.state.SetStatus(SyncStatusConnecting, "Connecting to server...")

	// Initial sync immediately (show syncing status)
	m.refreshCacheInternal(RefreshOptions{})

	// Periodic refresh every 30 seconds (quiet - don't change status)
	// This is a fallback for non-Electrum backends or if notifications are missed
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if m.unlocked {
				// Quiet refresh - stays synced, doesn't show "Syncing..."
				m.refreshCacheInternal(RefreshOptions{Quiet: true})
			}
		}
	}
}

// RefreshCache updates all cached wallet data (shows "Syncing..." status)
// Used when triggered by new block/transaction notifications
func (m *WalletManager) RefreshCache() {
	m.refreshCacheInternal(RefreshOptions{})
}

// RefreshCacheWithOptions updates cached wallet data with configurable behavior
// Use SkipAddressScan/SkipFeeEstimates for faster post-send refreshes
func (m *WalletManager) RefreshCacheWithOptions(opts RefreshOptions) {
	m.refreshCacheInternal(opts)
}

// refreshCacheInternal updates all cached wallet data
// Uses RefreshOptions to control behavior:
//   - Quiet: don't change status (for periodic background refreshes)
//   - SkipAddressScan: skip SyncAddressUsage (faster post-send refresh)
//   - SkipFeeEstimates: skip GetFeeEstimates (not needed post-send)
func (m *WalletManager) refreshCacheInternal(opts RefreshOptions) {
	if !m.unlocked || m.cache == nil {
		return
	}

	// Dedup guard: skip if another refresh is already running
	if !atomic.CompareAndSwapInt32(&m.refreshing, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&m.refreshing, 0)

	m.cache.SetSyncing(true)

	// Helper to set status only if not quiet
	setStatus := func(msg string) {
		if !opts.Quiet {
			m.state.SetStatus(SyncStatusSyncing, msg)
		}
	}

	defer func() {
		m.cache.SetSyncing(false)

		// Persist cache to disk after sync
		m.persistCache()
	}()

	// Connect backend if needed
	setStatus("Connecting to server...")
	if m.backend != nil {
		if err := m.backend.Connect(); err != nil {
			m.state.SetError(fmt.Errorf("connection failed: %w", err))
			return
		}
		if err := m.ensureNetworkVerified(); err != nil {
			m.state.SetError(err)
			return
		}
	}

	// Get block height first
	setStatus("Fetching block height...")
	if height, err := m.backend.GetBlockHeight(); err == nil {
		m.cache.SetBlockHeight(height)
	}

	// Sync address usage to ensure we have all addresses
	if !opts.SkipAddressScan {
		setStatus("Scanning addresses...")
		if err := m.SyncAddressUsage(0); err != nil {
			log.Printf("[SYNC] address scan failed: %v", err)
		}
	}

	// Setup creates the first account right after the wallet is opened;
	// until then there is nothing to fetch.
	m.dataMu.RLock()
	hasAccount := m.data != nil && len(m.data.Accounts) > 0
	m.dataMu.RUnlock()
	if !hasAccount {
		m.state.MarkSynced(m.cache.GetBlockHeight())
		return
	}

	// Fetch UTXOs first (needed for accurate balance)
	setStatus("Fetching UTXOs...")
	utxos, err := m.fetchUTXOsFromNetwork(0)
	if err != nil {
		// Keep the last good balance and history; report the failure
		// instead of showing a zero balance as "synced".
		m.state.SetError(err)
		return
	}
	m.cache.SetUTXOs(utxos)
	m.state.UpdateUTXOs(utxos)

	// Calculate balance from UTXOs (more accurate than separate fetch)
	setStatus("Calculating balance...")
	var balance api.Satoshi
	for _, utxo := range utxos {
		balance += utxo.Value
	}
	m.cache.SetBalance(balance)
	m.state.UpdateBalance(balance)

	// Fetch transactions
	setStatus("Fetching transactions...")
	txs, err := m.GetTransactions(0)
	if err != nil {
		m.state.SetError(err)
		return
	}
	m.cache.SetTransactions(txs)
	m.state.UpdateTransactions(txs)

	// Fetch fee estimates
	if !opts.SkipFeeEstimates {
		setStatus("Fetching fee estimates...")
		fees, err := m.GetFeeEstimates()
		if err == nil {
			m.cache.SetFeeEstimates(fees)
			// Update unified state
			m.state.UpdateFeeEstimates(fees)
		}
	}

	// Update address indexes in unified state
	nextReceive, nextChange := m.cache.GetNextIndexes()
	m.state.UpdateAddressIndexes(nextReceive, nextChange)

	// Mark as synced with block height
	m.state.MarkSynced(m.cache.GetBlockHeight())
}

// persistCache saves the current cache to the wallet file for instant load on next unlock
func (m *WalletManager) persistCache() {
	if m.data == nil || m.cache == nil || !m.hasKey() {
		return
	}

	ps := m.cache.GetPersistSnapshot()

	// Convert cached transactions to persistent format
	cachedTxs := make([]CachedTransaction, 0, len(ps.Transactions))
	for _, tx := range ps.Transactions {
		cachedTxs = append(cachedTxs, CachedTransaction{
			TxID:        tx.TxID,
			BlockHeight: tx.Status.BlockHeight,
			BlockTime:   tx.Status.BlockTime,
			Confirmed:   tx.Status.Confirmed,
		})
	}

	// Convert cached UTXOs to persistent format
	cachedUTXOs := make([]CachedUTXO, 0, len(ps.UTXOs))
	for _, utxo := range ps.UTXOs {
		cachedUTXOs = append(cachedUTXOs, CachedUTXO{
			TxID:        utxo.TxID,
			Vout:        utxo.Vout,
			Value:       utxo.Value,
			BlockHeight: utxo.Status.BlockHeight,
			Confirmed:   utxo.Status.Confirmed,
		})
	}

	// Lock dataMu to safely update m.data.Cache and read m.data fields
	m.dataMu.Lock()
	// Update persistent cache in wallet data
	m.data.Cache = &WalletDataCache{
		Transactions:   cachedTxs,
		UTXOs:          cachedUTXOs,
		LastSyncHeight: ps.BlockHeight,
		LastSyncTime:   time.Now().Unix(),
	}
	// Save synchronously while holding the lock to avoid data races
	// (saving to local file is fast, so blocking is acceptable)
	walletName := m.data.Name
	walletData := m.data
	_ = m.saveWalletTracked(walletName, walletData) // best-effort cache persist; retried on next save
	m.dataMu.Unlock()
}

// GetTotalBalanceChange does a quick balance refresh and returns the change
// This is used for notifications to show the net effect on wallet balance
// Uses QuickRefreshBalance for speed (skips address scanning and transaction fetching)
func (m *WalletManager) GetTotalBalanceChange() (balanceChange api.Satoshi, err error) {
	if m.cache == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	// Get old total balance
	oldBalance := m.cache.GetBalance()

	// Quick refresh - only fetches UTXOs and calculates balance
	// This is much faster than full RefreshCache (skips address scan and tx fetch)
	if err := m.QuickRefreshBalance(); err != nil {
		return 0, err
	}

	// Get new total balance
	newBalance := m.cache.GetBalance()

	balanceChange = newBalance - oldBalance
	return balanceChange, nil
}

// QuickRefreshBalance fetches only UTXOs and updates balance
// Much faster than full RefreshCache - skips address scanning and transaction fetching
// Use this for transaction notifications where we just need the balance change
func (m *WalletManager) QuickRefreshBalance() error {
	if !m.unlocked || m.cache == nil || m.backend == nil {
		return fmt.Errorf("wallet not ready")
	}

	// Connect if needed (usually already connected)
	if err := m.backend.Connect(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	if err := m.ensureNetworkVerified(); err != nil {
		return err
	}

	// Get block height (fast)
	if height, err := m.backend.GetBlockHeight(); err == nil {
		m.cache.SetBlockHeight(height)
	}

	// Fetch UTXOs (this is the main data we need for balance)
	utxos, err := m.fetchUTXOsFromNetwork(0)
	if err != nil {
		return fmt.Errorf("failed to fetch UTXOs: %w", err)
	}

	// Update cache and state
	m.cache.SetUTXOs(utxos)
	m.state.UpdateUTXOs(utxos)

	// Calculate balance from UTXOs
	var balance api.Satoshi
	for _, utxo := range utxos {
		balance += utxo.Value
	}
	blockHeight := m.cache.GetBlockHeight()
	m.cache.SetBalanceWithHeight(balance, blockHeight)
	m.state.UpdateBalance(balance)

	// Mark as synced (balance is current even if transactions aren't fully updated)
	m.state.MarkSynced(blockHeight)

	return nil
}

// GetCachedBalance returns the cached balance (instant, no network call)
func (m *WalletManager) GetCachedBalance() (api.Satoshi, bool) {
	if m.cache == nil {
		return 0, false
	}
	m.cache.mu.RLock()
	defer m.cache.mu.RUnlock()
	if m.cache.BalanceUpdatedAt.IsZero() {
		return 0, false
	}
	return m.cache.TotalBalance, true
}

// GetCachedAddressBalance returns the cached balance for a specific address (instant, no network call)
// Returns balance and whether it was found in cache
func (m *WalletManager) GetCachedAddressBalance(address string) (api.Satoshi, bool) {
	// First try cache
	if m.cache != nil {
		if bal, ok := m.cache.GetAddressBalance(address); ok {
			return bal, true
		}
	}

	// Fall back to calculating from UTXOs (use display accessors to include persistent data)
	if m.cache != nil {
		utxos := m.cache.GetUTXOsForDisplay()
		txs := m.cache.GetTransactionsForDisplay()

		var balance api.Satoshi
		for _, utxo := range utxos {
			for _, tx := range txs {
				if tx.TxID == utxo.TxID && int(utxo.Vout) < len(tx.Vout) {
					if tx.Vout[utxo.Vout].ScriptpubkeyAddress == address {
						balance += utxo.Value
					}
				}
			}
		}
		if balance > 0 {
			return balance, true
		}
	}

	return 0, false
}

// GetCachedTransactions returns cached transactions (instant, no network call)
func (m *WalletManager) GetCachedTransactions() ([]api.Transaction, bool) {
	if m.cache == nil {
		return nil, false
	}
	return m.cache.GetTransactions()
}

// GetCachedUTXOs returns cached UTXOs (instant, no network call)
func (m *WalletManager) GetCachedUTXOs() ([]api.UTXO, bool) {
	if m.cache == nil {
		return nil, false
	}
	return m.cache.GetUTXOs()
}

// GetCachedFeeEstimates returns cached fee estimates (instant, no network call)
func (m *WalletManager) GetCachedFeeEstimates() (*api.FeeEstimates, bool) {
	if m.cache == nil {
		return nil, false
	}
	return m.cache.GetFeeEstimates()
}

// IsSyncing returns whether a sync is currently in progress
func (m *WalletManager) IsSyncing() bool {
	if m.cache == nil {
		return false
	}
	return m.cache.IsCacheSyncing()
}

// GetBalance returns the total balance for an account
// Scans both receive and change addresses with a gap limit of 20 using the Used flag
func (m *WalletManager) GetBalance(accountIndex uint32) (api.Satoshi, error) {
	if !m.unlocked {
		return 0, fmt.Errorf("wallet is locked")
	}

	// Copy addresses while holding lock to avoid races
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return 0, fmt.Errorf("account not found")
	}
	account := &m.data.Accounts[accountIndex]
	addresses := make([]AddressData, len(account.Addresses))
	copy(addresses, account.Addresses)
	m.dataMu.RUnlock()

	var totalBalance api.Satoshi

	// Scan receive addresses with gap limit based on Used flag
	gapLimit := m.getGapLimit()
	consecutiveUnused := 0
	currentIndex := uint32(0)

	for {
		// Get or derive address at current index (this has its own locking)
		address, _, err := m.GetAddressByIndex(accountIndex, currentIndex)
		if err != nil {
			break
		}

		// Find this address in our copied addresses
		var addrData *AddressData
		for i := range addresses {
			if addresses[i].Address == address && !addresses[i].Change {
				addrData = &addresses[i]
				break
			}
		}

		if addrData == nil {
			// Address not in our snapshot - re-copy to get latest
			m.dataMu.RLock()
			if int(accountIndex) < len(m.data.Accounts) {
				addresses = make([]AddressData, len(m.data.Accounts[accountIndex].Addresses))
				copy(addresses, m.data.Accounts[accountIndex].Addresses)
			}
			m.dataMu.RUnlock()

			// Try again with updated snapshot
			for i := range addresses {
				if addresses[i].Address == address && !addresses[i].Change {
					addrData = &addresses[i]
					break
				}
			}
			if addrData == nil {
				break
			}
		}

		// Get balance for this address
		balance, err := m.backend.GetAddressBalance(address)
		if err != nil {
			// Network error, stop scanning
			break
		}

		if balance > 0 {
			totalBalance += balance
		}

		// Use the Used flag to determine gap limit
		if !addrData.Used {
			consecutiveUnused++
		} else {
			consecutiveUnused = 0
		}

		// Stop if we've hit the gap limit
		if consecutiveUnused >= gapLimit {
			break
		}

		currentIndex++

		// Safety limit
		if currentIndex > uint32(gapLimit*5) { // #nosec G115 -- gap limit is a small positive config value
			break
		}
	}

	// Re-copy addresses for change address scan (they may have been updated)
	m.dataMu.RLock()
	if int(accountIndex) < len(m.data.Accounts) {
		addresses = make([]AddressData, len(m.data.Accounts[accountIndex].Addresses))
		copy(addresses, m.data.Accounts[accountIndex].Addresses)
	}
	m.dataMu.RUnlock()

	// Scan all change addresses (no gap limit)
	for i := range addresses {
		if addresses[i].Change {
			balance, err := m.backend.GetAddressBalance(addresses[i].Address)
			if err != nil {
				continue
			}
			if balance > 0 {
				totalBalance += balance
			}
		}
	}

	return totalBalance, nil
}

// GetAddressBalance returns the balance for a specific address
func (m *WalletManager) GetAddressBalance(address string) (api.Satoshi, error) {
	return m.backend.GetAddressBalance(address)
}

// GetTransactions returns all transactions for an account
func (m *WalletManager) GetTransactions(accountIndex uint32) ([]api.Transaction, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	// Copy addresses while holding lock to avoid race with address derivation
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return nil, fmt.Errorf("account not found")
	}
	account := &m.data.Accounts[accountIndex]
	addresses := make([]AddressData, len(account.Addresses))
	copy(addresses, account.Addresses)
	m.dataMu.RUnlock()

	if len(addresses) == 0 {
		return nil, nil
	}

	// Context with timeout to prevent hanging forever
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Bounded parallel fetch - limit concurrent requests to avoid overwhelming server
	const maxConcurrent = 10
	semaphore := make(chan struct{}, maxConcurrent)

	type fetchResult struct {
		txs []api.Transaction
		err error
	}
	results := make(chan fetchResult, len(addresses))

	var wg sync.WaitGroup
	for _, addr := range addresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()

			// Check context before acquiring semaphore
			select {
			case <-ctx.Done():
				results <- fetchResult{err: ctx.Err()}
				return
			case semaphore <- struct{}{}: // acquire
			}
			defer func() { <-semaphore }() // release

			txs, err := m.backend.GetAddressTransactions(address)
			if err != nil {
				results <- fetchResult{err: fmt.Errorf("fetch txs for %s: %w", address[:8], err)}
				return
			}
			results <- fetchResult{txs: txs}
		}(addr.Address)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and deduplicate
	allTxs := make([]api.Transaction, 0, len(addresses)*2)
	seenTxs := make(map[string]bool)
	var errs []error

	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		for _, tx := range result.txs {
			if !seenTxs[tx.TxID] {
				allTxs = append(allTxs, tx)
				seenTxs[tx.TxID] = true
			}
		}
	}

	// A partial list would make history silently shrink, so any failure
	// fails the fetch and callers keep what they already have.
	if len(errs) > 0 {
		return nil, fmt.Errorf("fetching transactions failed for %d of %d addresses: %w", len(errs), len(addresses), errs[0])
	}

	return allTxs, nil
}

// GetUTXOs returns all UTXOs for an account
// Uses cached UTXOs if available for instant response
func (m *WalletManager) GetUTXOs(accountIndex uint32) ([]api.UTXO, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	if int(accountIndex) >= len(m.data.Accounts) {
		return nil, fmt.Errorf("account not found")
	}

	// Try cached UTXOs first for instant response
	if m.cache != nil {
		if utxos, ok := m.cache.GetUTXOs(); ok && len(utxos) > 0 {
			return utxos, nil
		}
	}

	// No cache - fetch from network
	return m.fetchUTXOsFromNetwork(accountIndex)
}

// fetchUTXOsFromNetwork fetches UTXOs directly from the backend
func (m *WalletManager) fetchUTXOsFromNetwork(accountIndex uint32) ([]api.UTXO, error) {
	// Copy addresses while holding lock to avoid race with address derivation
	m.dataMu.RLock()
	if int(accountIndex) >= len(m.data.Accounts) {
		m.dataMu.RUnlock()
		return nil, fmt.Errorf("account not found")
	}
	account := &m.data.Accounts[accountIndex]

	// Collect addresses to check - from account.Addresses first
	addressesToCheck := make(map[string]bool)
	for _, addr := range account.Addresses {
		addressesToCheck[addr.Address] = true
	}
	m.dataMu.RUnlock()

	// Also include addresses from cache (in case scan found more)
	if m.cache != nil {
		for _, addr := range m.cache.GetAllAddressKeys() {
			addressesToCheck[addr] = true
		}
	}

	// Convert to slice for parallel processing
	addresses := make([]string, 0, len(addressesToCheck))
	for addr := range addressesToCheck {
		addresses = append(addresses, addr)
	}

	if len(addresses) == 0 {
		return nil, nil
	}

	// Bounded parallel fetch - limit concurrent requests
	const maxConcurrent = 10
	semaphore := make(chan struct{}, maxConcurrent)
	type utxoResult struct {
		utxos []api.UTXO
		err   error
	}
	results := make(chan utxoResult, len(addresses))

	var wg sync.WaitGroup
	for _, addr := range addresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire
			defer func() { <-semaphore }() // release

			utxos, err := m.backend.GetAddressUTXOs(address)
			results <- utxoResult{utxos: utxos, err: err}
		}(addr)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	// A partial UTXO set means a wrong balance, so any failed address
	// fails the whole fetch and the previous data is kept.
	allUTXOs := make([]api.UTXO, 0, len(addresses))
	var firstErr error
	failed := 0
	for r := range results {
		if r.err != nil {
			failed++
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		allUTXOs = append(allUTXOs, r.utxos...)
	}
	if firstErr != nil {
		return nil, fmt.Errorf("fetching UTXOs failed for %d of %d addresses: %w", failed, len(addresses), firstErr)
	}

	// Update cache
	if m.cache != nil {
		m.cache.SetUTXOs(allUTXOs)
	}

	return allUTXOs, nil
}

// GetUTXOsWithPath returns all UTXOs for an account with their derivation paths
// This is needed for transaction signing
func (m *WalletManager) GetUTXOsWithPath(accountIndex uint32) ([]UTXOWithPath, error) {
	if !m.unlocked {
		return nil, fmt.Errorf("wallet is locked")
	}

	if int(accountIndex) >= len(m.data.Accounts) {
		return nil, fmt.Errorf("account not found")
	}

	account := &m.data.Accounts[accountIndex]
	var allUTXOs []UTXOWithPath

	// Get UTXOs for all addresses, preserving address and path info
	for _, addr := range account.Addresses {
		utxos, err := m.backend.GetAddressUTXOs(addr.Address)
		if err != nil {
			continue
		}

		// Wrap each UTXO with its address and path
		for _, utxo := range utxos {
			allUTXOs = append(allUTXOs, UTXOWithPath{
				UTXO:    utxo,
				Address: addr.Address,
				Path:    addr.Path,
			})
		}
	}

	return allUTXOs, nil
}

// GetFeeEstimates returns current fee rate estimates
func (m *WalletManager) GetFeeEstimates() (*api.FeeEstimates, error) {
	return m.backend.GetFeeEstimates()
}

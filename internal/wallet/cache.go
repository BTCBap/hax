package wallet

import (
	"sync"
	"time"

	"github.com/BTCBap/hax/internal/api"
)

// AddressCacheEntry holds cached data for a single address
type AddressCacheEntry struct {
	Address     string
	Scripthash  string
	HasHistory  bool        // True if address has any transaction history (is "used")
	Balance     api.Satoshi // Current balance in satoshis
	Index       uint32
	IsChange    bool
	LastChecked time.Time
}

// WalletCache holds cached blockchain data to avoid repeated fetches.
// All access must go through thread-safe methods.
type WalletCache struct {
	mu sync.RWMutex

	// Cached data
	TotalBalance api.Satoshi
	Transactions []api.Transaction
	UTXOs        []api.UTXO
	FeeEstimates *api.FeeEstimates
	BlockHeight  int64

	// Address tracking - key is address string
	Addresses           map[string]*AddressCacheEntry
	ScripthashToAddress map[string]string // scripthash -> address reverse lookup
	NextReceiveIndex    uint32            // First unused receive address index
	NextChangeIndex     uint32            // First unused change address index

	// Timestamps for staleness checks
	BalanceUpdatedAt      time.Time
	TransactionsUpdatedAt time.Time
	UTXOsUpdatedAt        time.Time
	FeesUpdatedAt         time.Time
	AddressesUpdatedAt    time.Time

	// Sync state
	LastSyncAt     time.Time
	SyncInProgress bool
	SyncError      error
}

// NewWalletCache creates an initialized WalletCache.
func NewWalletCache() *WalletCache {
	return &WalletCache{
		Addresses:           make(map[string]*AddressCacheEntry),
		ScripthashToAddress: make(map[string]string),
	}
}

// --- Balance ---

// GetBalance returns the cached total balance.
func (c *WalletCache) GetBalance() api.Satoshi {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TotalBalance
}

// SetBalance updates the balance and its timestamp atomically.
func (c *WalletCache) SetBalance(balance api.Satoshi) {
	c.mu.Lock()
	c.TotalBalance = balance
	c.BalanceUpdatedAt = time.Now()
	c.mu.Unlock()
}

// SetBalanceWithHeight updates balance, timestamp, and block height atomically.
func (c *WalletCache) SetBalanceWithHeight(balance api.Satoshi, blockHeight int64) {
	c.mu.Lock()
	c.TotalBalance = balance
	c.BalanceUpdatedAt = time.Now()
	c.BlockHeight = blockHeight
	c.mu.Unlock()
}

// --- Transactions ---

// GetTransactions returns a copy of cached transactions and whether they exist.
func (c *WalletCache) GetTransactions() ([]api.Transaction, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.TransactionsUpdatedAt.IsZero() {
		return nil, false
	}
	txs := make([]api.Transaction, len(c.Transactions))
	copy(txs, c.Transactions)
	return txs, true
}

// GetTransactionsForDisplay returns cached transactions including persistent (pre-sync) data.
// Unlike GetTransactions, this does not gate on timestamps — use for display-only contexts.
func (c *WalletCache) GetTransactionsForDisplay() []api.Transaction {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.Transactions) == 0 {
		return nil
	}
	txs := make([]api.Transaction, len(c.Transactions))
	copy(txs, c.Transactions)
	return txs
}

// SetTransactions updates the cached transactions and timestamp.
func (c *WalletCache) SetTransactions(txs []api.Transaction) {
	c.mu.Lock()
	c.Transactions = txs
	c.TransactionsUpdatedAt = time.Now()
	c.mu.Unlock()
}

// --- UTXOs ---

// GetUTXOs returns a copy of cached UTXOs and whether they exist.
func (c *WalletCache) GetUTXOs() ([]api.UTXO, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.UTXOsUpdatedAt.IsZero() {
		return nil, false
	}
	utxos := make([]api.UTXO, len(c.UTXOs))
	copy(utxos, c.UTXOs)
	return utxos, true
}

// GetUTXOsForDisplay returns cached UTXOs including persistent (pre-sync) data.
// Unlike GetUTXOs, this does not gate on timestamps — use for display-only contexts.
func (c *WalletCache) GetUTXOsForDisplay() []api.UTXO {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.UTXOs) == 0 {
		return nil
	}
	utxos := make([]api.UTXO, len(c.UTXOs))
	copy(utxos, c.UTXOs)
	return utxos
}

// SetUTXOs updates the cached UTXOs and timestamp.
func (c *WalletCache) SetUTXOs(utxos []api.UTXO) {
	c.mu.Lock()
	c.UTXOs = utxos
	c.UTXOsUpdatedAt = time.Now()
	c.mu.Unlock()
}

// --- Fee Estimates ---

// GetFeeEstimates returns the cached fee estimates and whether they exist.
func (c *WalletCache) GetFeeEstimates() (*api.FeeEstimates, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.FeesUpdatedAt.IsZero() {
		return nil, false
	}
	return c.FeeEstimates, true
}

// SetFeeEstimates updates the cached fee estimates and timestamp.
func (c *WalletCache) SetFeeEstimates(fees *api.FeeEstimates) {
	c.mu.Lock()
	c.FeeEstimates = fees
	c.FeesUpdatedAt = time.Now()
	c.mu.Unlock()
}

// --- Block Height ---

// GetBlockHeight returns the cached block height.
func (c *WalletCache) GetBlockHeight() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BlockHeight
}

// SetBlockHeight updates the cached block height.
func (c *WalletCache) SetBlockHeight(height int64) {
	c.mu.Lock()
	c.BlockHeight = height
	c.mu.Unlock()
}

// --- Address Management ---

// GetAddress returns the cached entry for an address, or nil if not found.
func (c *WalletCache) GetAddress(addr string) *AddressCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Addresses[addr]
}

// SetAddress stores an address entry and optional scripthash reverse lookup.
func (c *WalletCache) SetAddress(addr string, entry *AddressCacheEntry, scripthash string) {
	c.mu.Lock()
	c.Addresses[addr] = entry
	if scripthash != "" {
		c.ScripthashToAddress[scripthash] = addr
	}
	c.mu.Unlock()
}

// UpdateAddressHistory updates the history/balance for an existing address entry.
func (c *WalletCache) UpdateAddressHistory(addr string, hasHistory bool, balance api.Satoshi) {
	c.mu.Lock()
	if entry, ok := c.Addresses[addr]; ok {
		entry.HasHistory = hasHistory
		entry.Balance = balance
		entry.LastChecked = time.Now()
	} else {
		// Create entry if it doesn't exist (e.g., pre-existing addresses from wallet file)
		c.Addresses[addr] = &AddressCacheEntry{
			Address:     addr,
			HasHistory:  hasHistory,
			Balance:     balance,
			LastChecked: time.Now(),
		}
	}
	c.mu.Unlock()
}

// GetAddressBalance returns the cached balance for an address.
func (c *WalletCache) GetAddressBalance(addr string) (api.Satoshi, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.Addresses[addr]
	if !ok {
		return 0, false
	}
	return entry.Balance, true
}

// HasAddressHistory returns whether the address has transaction history.
func (c *WalletCache) HasAddressHistory(addr string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.Addresses[addr]
	if !ok {
		return false
	}
	return entry.HasHistory
}

// GetAllAddressKeys returns all address strings in the cache.
func (c *WalletCache) GetAllAddressKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, 0, len(c.Addresses))
	for k := range c.Addresses {
		result = append(result, k)
	}
	return result
}

// GetReceiveAddresses returns all non-change address cache entries.
func (c *WalletCache) GetReceiveAddresses() []*AddressCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var result []*AddressCacheEntry
	for _, entry := range c.Addresses {
		if !entry.IsChange {
			result = append(result, entry)
		}
	}
	return result
}

// LookupByScripthash returns the address for a scripthash, or empty string.
func (c *WalletCache) LookupByScripthash(scripthash string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ScripthashToAddress[scripthash]
}

// --- Address Indexes ---

// GetNextIndexes returns the next receive and change address indexes.
func (c *WalletCache) GetNextIndexes() (receive, change uint32) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NextReceiveIndex, c.NextChangeIndex
}

// GetNextReceiveIndex returns the next unused receive address index.
func (c *WalletCache) GetNextReceiveIndex() uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NextReceiveIndex
}

// SetNextReceiveIndex updates the next receive address index if higher.
func (c *WalletCache) SetNextReceiveIndex(idx uint32) {
	c.mu.Lock()
	if idx > c.NextReceiveIndex {
		c.NextReceiveIndex = idx
	}
	c.mu.Unlock()
}

// SetNextChangeIndex updates the next change address index if higher.
func (c *WalletCache) SetNextChangeIndex(idx uint32) {
	c.mu.Lock()
	if idx > c.NextChangeIndex {
		c.NextChangeIndex = idx
	}
	c.mu.Unlock()
}

// --- Sync State ---

// IsCacheSyncing returns whether a sync is currently in progress.
func (c *WalletCache) IsCacheSyncing() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SyncInProgress
}

// SetSyncing sets or clears the sync-in-progress flag.
// When clearing (false), also updates LastSyncAt.
func (c *WalletCache) SetSyncing(syncing bool) {
	c.mu.Lock()
	c.SyncInProgress = syncing
	if !syncing {
		c.LastSyncAt = time.Now()
	}
	c.mu.Unlock()
}

// --- Timestamp Checks ---

// IsAddressScanned returns true if addresses have been scanned at least once.
func (c *WalletCache) IsAddressScanned() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.AddressesUpdatedAt.IsZero()
}

// MarkAddressesScanned sets the address scan timestamp to now.
func (c *WalletCache) MarkAddressesScanned() {
	c.mu.Lock()
	c.AddressesUpdatedAt = time.Now()
	c.mu.Unlock()
}

// --- Compound Operations ---

// CacheSnapshot holds a point-in-time snapshot of cache data for state sync.
type CacheSnapshot struct {
	Balance          api.Satoshi
	Transactions     []api.Transaction
	UTXOs            []api.UTXO
	BlockHeight      int64
	FeeEstimates     *api.FeeEstimates
	NextReceiveIndex uint32
	NextChangeIndex  uint32
}

// Snapshot returns a point-in-time copy of key cache fields under one lock.
func (c *WalletCache) Snapshot() CacheSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	txs := make([]api.Transaction, len(c.Transactions))
	copy(txs, c.Transactions)
	utxos := make([]api.UTXO, len(c.UTXOs))
	copy(utxos, c.UTXOs)
	return CacheSnapshot{
		Balance:          c.TotalBalance,
		Transactions:     txs,
		UTXOs:            utxos,
		BlockHeight:      c.BlockHeight,
		FeeEstimates:     c.FeeEstimates,
		NextReceiveIndex: c.NextReceiveIndex,
		NextChangeIndex:  c.NextChangeIndex,
	}
}

// LoadPersistentData populates cache from persistent wallet data for instant display.
// Does NOT set timestamps — forces fresh fetch when Get methods are called.
func (c *WalletCache) LoadPersistentData(txs []api.Transaction, utxos []api.UTXO, balance api.Satoshi, blockHeight int64) {
	c.mu.Lock()
	c.Transactions = txs
	c.UTXOs = utxos
	c.TotalBalance = balance
	c.BlockHeight = blockHeight
	c.mu.Unlock()
}

// PersistSnapshot holds data needed for cache persistence.
type PersistSnapshot struct {
	Transactions []api.Transaction
	UTXOs        []api.UTXO
	BlockHeight  int64
}

// GetPersistSnapshot returns a snapshot of data for persistence under one lock.
func (c *WalletCache) GetPersistSnapshot() PersistSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	txs := make([]api.Transaction, len(c.Transactions))
	copy(txs, c.Transactions)
	utxos := make([]api.UTXO, len(c.UTXOs))
	copy(utxos, c.UTXOs)
	return PersistSnapshot{
		Transactions: txs,
		UTXOs:        utxos,
		BlockHeight:  c.BlockHeight,
	}
}

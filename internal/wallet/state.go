package wallet

import (
	"sync"
	"time"

	"github.com/BTCBap/hax/internal/api"
)

// SyncStatus represents the current wallet sync state
type SyncStatus int

const (
	SyncStatusDisconnected SyncStatus = iota
	SyncStatusConnecting
	SyncStatusConnected
	SyncStatusSyncing
	SyncStatusSynced
	SyncStatusError
)

// String returns a human-readable status string
func (s SyncStatus) String() string {
	switch s {
	case SyncStatusDisconnected:
		return "Disconnected"
	case SyncStatusConnecting:
		return "Connecting..."
	case SyncStatusConnected:
		return "Connected"
	case SyncStatusSyncing:
		return "Syncing..."
	case SyncStatusSynced:
		return "Synced"
	case SyncStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// UnifiedAddressEntry combines cache and persistent address data
type UnifiedAddressEntry struct {
	Address     string
	Scripthash  string
	Path        string
	Index       uint32
	IsChange    bool
	Used        bool        // Has any transaction history
	Balance     api.Satoshi // Current balance
	LastChecked time.Time
}

// WalletState is the single source of truth for all wallet data
// All screens read from this struct via Snapshot(); only WalletManager writes to it
type WalletState struct {
	mu sync.RWMutex

	// Sync status for UI display
	Status      SyncStatus
	StatusMsg   string // Human-readable status ("Synced at height 12345")
	BlockHeight int64
	LastError   error
	LastSyncAt  time.Time

	// Core wallet data
	Balance      api.Satoshi
	Transactions []api.Transaction
	UTXOs        []api.UTXO
	FeeEstimates *api.FeeEstimates

	// Address data
	Addresses        map[string]*UnifiedAddressEntry
	NextReceiveIndex uint32
	NextChangeIndex  uint32

	// Version counter for change detection
	// Incremented on every state change
	Version uint64
}

// NewWalletState creates a new WalletState instance
func NewWalletState() *WalletState {
	return &WalletState{
		Status:    SyncStatusDisconnected,
		Addresses: make(map[string]*UnifiedAddressEntry),
	}
}

// ResetWalletData drops everything that belongs to the open wallet
// (balance, history, UTXOs, addresses, sync status) so the next wallet
// starts empty. Chain-wide data, the block height and fee estimates, is kept.
func (s *WalletState) ResetWalletData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SyncStatusDisconnected
	s.StatusMsg = ""
	s.LastError = nil
	s.LastSyncAt = time.Time{}
	s.Balance = 0
	s.Transactions = nil
	s.UTXOs = nil
	s.Addresses = make(map[string]*UnifiedAddressEntry)
	s.NextReceiveIndex = 0
	s.NextChangeIndex = 0
	s.Version++
}

// WalletStateSnapshot is an immutable snapshot passed to UI
// Safe to read without locks after creation
type WalletStateSnapshot struct {
	Status           SyncStatus
	StatusMsg        string
	BlockHeight      int64
	Balance          api.Satoshi
	Transactions     []api.Transaction
	UTXOs            []api.UTXO
	FeeEstimates     *api.FeeEstimates
	NextReceiveIndex uint32
	NextChangeIndex  uint32
	Version          uint64
	LastSyncAt       time.Time
	LastError        error
}

// Snapshot returns an immutable copy of current state
// Safe to pass to UI without holding locks
func (s *WalletState) Snapshot() WalletStateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy slices to prevent mutation
	txs := make([]api.Transaction, len(s.Transactions))
	copy(txs, s.Transactions)

	utxos := make([]api.UTXO, len(s.UTXOs))
	copy(utxos, s.UTXOs)

	return WalletStateSnapshot{
		Status:           s.Status,
		StatusMsg:        s.StatusMsg,
		BlockHeight:      s.BlockHeight,
		Balance:          s.Balance,
		Transactions:     txs,
		UTXOs:            utxos,
		FeeEstimates:     s.FeeEstimates,
		NextReceiveIndex: s.NextReceiveIndex,
		NextChangeIndex:  s.NextChangeIndex,
		Version:          s.Version,
		LastSyncAt:       s.LastSyncAt,
		LastError:        s.LastError,
	}
}

// SetStatus updates the sync status and message
func (s *WalletState) SetStatus(status SyncStatus, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.StatusMsg = msg
	s.Version++
}

// SetStatusWithHeight updates status with block height info
func (s *WalletState) SetStatusWithHeight(status SyncStatus, height int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.BlockHeight = height
	s.StatusMsg = "" // Will be formatted by UI
	s.Version++
}

// SetError sets an error status
func (s *WalletState) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SyncStatusError
	s.LastError = err
	s.StatusMsg = err.Error()
	s.Version++
}

// UpdateBalance updates the balance
func (s *WalletState) UpdateBalance(balance api.Satoshi) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Balance = balance
	s.Version++
}

// UpdateTransactions updates the transaction list
func (s *WalletState) UpdateTransactions(txs []api.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Transactions = txs
	s.Version++
}

// UpdateUTXOs updates the UTXO list
func (s *WalletState) UpdateUTXOs(utxos []api.UTXO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UTXOs = utxos
	s.Version++
}

// UpdateFeeEstimates updates fee estimates
func (s *WalletState) UpdateFeeEstimates(fees *api.FeeEstimates) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FeeEstimates = fees
	s.Version++
}

// UpdateAddressIndexes updates the next unused address indexes
func (s *WalletState) UpdateAddressIndexes(receiveIdx, changeIdx uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NextReceiveIndex = receiveIdx
	s.NextChangeIndex = changeIdx
	s.Version++
}

// UpdateBlockHeight updates the block height without changing sync status
func (s *WalletState) UpdateBlockHeight(height int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if height > s.BlockHeight {
		s.BlockHeight = height
		s.Version++
	}
}

// MarkSynced marks the wallet as synced with the given block height
func (s *WalletState) MarkSynced(height int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SyncStatusSynced
	s.BlockHeight = height
	s.LastSyncAt = time.Now()
	s.LastError = nil
	s.Version++
}

// GetVersion returns the current version (for change detection)
func (s *WalletState) GetVersion() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Version
}

// GetStatus returns the current sync status
func (s *WalletState) GetStatus() SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

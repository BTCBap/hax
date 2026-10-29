package wallet

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/BTCBap/hax/internal/api"
)

// flakyBackend serves one UTXO and one transaction per address until down
// is set, then fails every address lookup like a dropped connection.
type flakyBackend struct {
	mockBroadcastBackend
	down atomic.Bool
}

var errDropped = errors.New("connection reset by peer")

func (b *flakyBackend) ConfiguredNetwork() string { return NetworkTestnet }

func (b *flakyBackend) GetAddressUTXOs(address string) ([]api.UTXO, error) {
	if b.down.Load() {
		return nil, errDropped
	}
	return []api.UTXO{{TxID: "tx-" + address, Value: 1000, Status: api.Status{Confirmed: true}}}, nil
}

func (b *flakyBackend) GetAddressTransactions(address string) ([]api.Transaction, error) {
	if b.down.Load() {
		return nil, errDropped
	}
	return []api.Transaction{{TxID: "tx-" + address, Status: api.Status{Confirmed: true}}}, nil
}

func unlockedWithBackend(t *testing.T, b *flakyBackend) *WalletManager {
	t.Helper()
	m, err := NewWalletManager(t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	mn := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if err := m.RestoreWalletOnNetwork("w", "correct horse battery", mn, "", NetworkTestnet); err != nil {
		t.Fatal(err)
	}
	if err := m.UnlockWallet("w", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateAccount("Main Account", AddressTypeSegWitNative, ""); err != nil {
		t.Fatal(err)
	}
	m.StopBackgroundSync()
	m.RefreshCache()
	// Wait for the initial sync, then drive refreshes by hand.
	m.StopBackgroundSync()
	t.Cleanup(m.LockWallet)
	return m
}

func TestRefresh_DisconnectKeepsLastGoodData(t *testing.T) {
	b := &flakyBackend{}
	m := unlockedWithBackend(t, b)

	good := m.GetState()
	if good.Status != SyncStatusSynced || good.Balance == 0 || len(good.Transactions) == 0 {
		t.Fatalf("initial sync: status=%v balance=%d txs=%d", good.Status, good.Balance, len(good.Transactions))
	}

	b.down.Store(true)
	m.RefreshCache()
	got := m.GetState()
	if got.Status != SyncStatusError {
		t.Fatalf("status after dropped connection = %v, want Error", got.Status)
	}
	if got.Balance != good.Balance || len(got.UTXOs) != len(good.UTXOs) || len(got.Transactions) != len(good.Transactions) {
		t.Fatalf("stale data replaced: balance %d->%d utxos %d->%d txs %d->%d",
			good.Balance, got.Balance, len(good.UTXOs), len(got.UTXOs), len(good.Transactions), len(got.Transactions))
	}
	if !got.LastSyncAt.Equal(good.LastSyncAt) {
		t.Fatal("a failed refresh must not bump the last-synced time")
	}

	// A quick balance check during the outage reports no change.
	if change, err := m.GetTotalBalanceChange(); err == nil || change != 0 {
		t.Fatalf("balance change during outage = %d, %v; want 0 and an error", change, err)
	}
	if m.GetState().Balance != good.Balance {
		t.Fatal("quick refresh zeroed the balance")
	}

	// Recovery: the next refresh is synced again.
	b.down.Store(false)
	m.RefreshCache()
	if s := m.GetState(); s.Status != SyncStatusSynced || s.Balance != good.Balance {
		t.Fatalf("after reconnect: status=%v balance=%d", s.Status, s.Balance)
	}
}

func TestFetchUTXOs_PartialFailureIsAnError(t *testing.T) {
	b := &flakyBackend{}
	m := unlockedWithBackend(t, b)
	b.down.Store(true)
	if utxos, err := m.fetchUTXOsFromNetwork(0); err == nil || utxos != nil {
		t.Fatalf("got %d utxos, err %v; want an error", len(utxos), err)
	}
	if _, err := m.GetTransactions(0); err == nil {
		t.Fatal("transaction fetch during outage should fail")
	}
}

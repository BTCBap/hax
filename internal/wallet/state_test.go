package wallet

import (
	"testing"

	"github.com/BTCBap/hax/internal/api"
)

func TestResetWalletData_ClearsWalletKeepsChain(t *testing.T) {
	s := NewWalletState()
	s.UpdateBalance(5000)
	s.UpdateTransactions([]api.Transaction{{TxID: "a"}})
	s.UpdateUTXOs([]api.UTXO{{TxID: "a"}})
	s.UpdateAddressIndexes(3, 2)
	s.MarkSynced(100)
	s.UpdateFeeEstimates(&api.FeeEstimates{})
	s.ResetWalletData()
	snap := s.Snapshot()
	if snap.Balance != 0 || len(snap.Transactions) != 0 || len(snap.UTXOs) != 0 ||
		snap.NextReceiveIndex != 0 || !snap.LastSyncAt.IsZero() || snap.Status != SyncStatusDisconnected {
		t.Fatalf("wallet data not cleared: %+v", snap)
	}
	if snap.BlockHeight != 100 || snap.FeeEstimates == nil {
		t.Fatal("chain data should be kept")
	}
}

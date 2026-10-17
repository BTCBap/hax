//go:build emulator

package wallet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// regtestWalletBackend is Bitcoin Core over RPC for a testnet4 wallet:
// addresses are re-encoded for regtest (same scripts).
type regtestWalletBackend struct {
	*backend.RPCBackend
	t *testing.T
}

func (b regtestWalletBackend) GetAddressUTXOs(addr string) ([]api.UTXO, error) {
	return b.RPCBackend.GetAddressUTXOs(regtestAddr(b.t, addr))
}

// GenesisHash claims testnet4 so the wallet's network check passes.
func (b regtestWalletBackend) GenesisHash() (chainhash.Hash, error) {
	return *TestNet4Params.GenesisHash, nil
}

// mempoolSend submits a signed PSBT's transaction without mining it.
func (n *regtestNode) mempoolSend(m *WalletManager, res *PSBTResult) string {
	n.t.Helper()
	if err := m.SignPSBTTransaction(res); err != nil {
		n.t.Fatal(err)
	}
	txHex, err := m.FinalizePSBTTransaction(res)
	if err != nil {
		n.t.Fatal(err)
	}
	return n.must("sendrawtransaction", txHex)
}

func (n *regtestNode) inMempool(txid string) bool {
	_, err := n.call("getmempoolentry", txid)
	return err == nil
}

// ancestorFeeRate is Core's view of a mempool transaction's package
// feerate including its unconfirmed ancestors, in sat/vB.
func (n *regtestNode) ancestorFeeRate(txid string) float64 {
	n.t.Helper()
	var e struct {
		AncestorSize int64 `json:"ancestorsize"`
		Fees         struct {
			Ancestor float64 `json:"ancestor"`
		} `json:"fees"`
	}
	n.json(&e, "getmempoolentry", txid)
	return e.Fees.Ancestor * 1e8 / float64(e.AncestorSize)
}

// TestRegtestFeeBump runs RBF and CPFP against Bitcoin Core without
// -txindex: Core must accept the replacement under its BIP125 rules, and
// the children must lift the package to the target feerate. Needs only
// HAX_BITCOIND.
func TestRegtestFeeBump(t *testing.T) {
	node := startRegtest(t)
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			b := regtestWalletBackend{node.rpcBackend(), t}
			m := newBumpTestManager(t, tt.addr, b)

			addr, err := m.GetNewAddress(0)
			if err != nil {
				t.Fatal(err)
			}
			funding := node.fundAddress(addr)
			m.state.UpdateTransactions([]api.Transaction{{
				TxID: funding.TxID, Status: funding.Status}})

			// A payment at 2 sat/vB, left unconfirmed.
			dest := externalAddress(t, &TestNet4Params)
			res, err := m.CreatePSBTBatchTransaction(0,
				[]TxRecipient{{Address: dest, Amount: 30_000}}, 2, nil)
			if err != nil {
				t.Fatal(err)
			}
			origID := node.mempoolSend(m, res)

			info, err := m.AnalyzeFeeBump(origID)
			if err != nil {
				t.Fatal(err)
			}
			if !info.CanRBF || !info.CanCPFP {
				t.Fatalf("want RBF and CPFP: %+v", info)
			}

			// RBF: Core enforces BIP125 on sendrawtransaction.
			res, err = m.BuildRBF(origID, 10)
			if err != nil {
				t.Fatal(err)
			}
			replID := node.mempoolSend(m, res)
			if node.inMempool(origID) || !node.inMempool(replID) {
				t.Fatal("replacement didn't replace the original")
			}
			if rate := node.ancestorFeeRate(replID); rate < 10 {
				t.Fatalf("replacement pays %.2f sat/vB", rate)
			}

			// CPFP on the replacement's change. scantxoutset can't see
			// it; gettxout with the mempool does.
			res, err = m.BuildCPFP(replID, 30)
			if err != nil {
				t.Fatal(err)
			}
			childID := node.mempoolSend(m, res)
			if rate := node.ancestorFeeRate(childID); rate < 30 {
				t.Fatalf("package pays %.2f sat/vB, want 30", rate)
			}

			// CPFP on an incoming payment: its inputs aren't ours, so
			// without -txindex the fee comes from getmempoolentry.
			inID := node.must("-rpcwallet=miner", "sendtoaddress",
				regtestAddr(t, addr), "0.0005")
			info, err = m.AnalyzeFeeBump(inID)
			if err != nil {
				t.Fatal(err)
			}
			if info.CanRBF || !info.CanCPFP {
				t.Fatalf("incoming: %+v", info)
			}
			res, err = m.BuildCPFP(inID, info.FeeRate+20)
			if err != nil {
				t.Fatal(err)
			}
			childID = node.mempoolSend(m, res)
			if rate := node.ancestorFeeRate(childID); rate < info.FeeRate+20 {
				t.Fatalf("incoming package pays %.2f sat/vB", rate)
			}
			node.must("-rpcwallet=miner", "-generate", "1")
		})
	}
}

// deviceSend has the emulator sign a PSBT, finalizes it and submits it
// to the mempool.
func (n *regtestNode) deviceSend(m *WalletManager, h *HWIClient, dev HardwareDevice, res *PSBTResult) string {
	n.t.Helper()
	unsigned, err := m.ExportPSBTBase64(res)
	if err != nil {
		n.t.Fatal(err)
	}
	signed, err := h.SignPSBT(dev, unsigned)
	if err != nil {
		n.t.Fatalf("device signing: %v", err)
	}
	imported, err := m.ImportPSBTFromBase64(signed)
	if err != nil {
		n.t.Fatal(err)
	}
	txHex, err := m.FinalizePSBTTransaction(imported)
	if err != nil {
		n.t.Fatal(err)
	}
	return n.must("sendrawtransaction", txHex)
}

// TestEmulatorFeeBump: the device signs a payment, its RBF replacement
// and a CPFP child, and Core accepts each.
func TestEmulatorFeeBump(t *testing.T) {
	h, dev := emulatorDevice(t)
	node := startRegtest(t)
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			origin := fmt.Sprintf("m/%dh/1h/0h", tt.purpose)
			xpub, err := h.GetXpub(dev, origin)
			if err != nil {
				t.Fatal(err)
			}
			m, err := NewWalletManager(t.TempDir(), regtestWalletBackend{node.rpcBackend(), t})
			if err != nil {
				t.Fatal(err)
			}
			keyOrigin := signerTestFingerprint + strings.TrimPrefix(origin, "m")
			if err := m.CreateHardwareWalletOnNetwork("hw", "emulator-pw", xpub,
				signerTestFingerprint, keyOrigin, tt.addr, string(dev.Type), NetworkTestnet); err != nil {
				t.Fatal(err)
			}
			if err := m.UnlockWallet("hw", "emulator-pw"); err != nil {
				t.Fatal(err)
			}
			m.StopBackgroundSync()
			t.Cleanup(m.LockWallet)

			addr, err := m.GetNewAddress(0)
			if err != nil {
				t.Fatal(err)
			}
			funding := node.fundAddress(addr)
			m.state.UpdateTransactions([]api.Transaction{{
				TxID: funding.TxID, Status: funding.Status}})

			dest := externalAddress(t, &TestNet4Params)
			res, err := m.CreatePSBTBatchTransaction(0,
				[]TxRecipient{{Address: dest, Amount: 30_000}}, 2, nil)
			if err != nil {
				t.Fatal(err)
			}
			origID := node.deviceSend(m, h, dev, res)

			res, err = m.BuildRBF(origID, 10)
			if err != nil {
				t.Fatal(err)
			}
			replID := node.deviceSend(m, h, dev, res)
			if node.inMempool(origID) {
				t.Fatal("original still in the mempool")
			}

			res, err = m.BuildCPFP(replID, 30)
			if err != nil {
				t.Fatal(err)
			}
			childID := node.deviceSend(m, h, dev, res)
			if rate := node.ancestorFeeRate(childID); rate < 30 {
				t.Fatalf("package pays %.2f sat/vB", rate)
			}
			node.must("-rpcwallet=miner", "-generate", "1")
		})
	}
}

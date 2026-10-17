//go:build emulator

// End-to-end hardware wallet check against a device emulator and a
// throwaway Bitcoin Core regtest node. Not part of CI; see
// docs/hardware-wallet-compatibility.md for setup. Run with:
//
//	HAX_BITCOIND=/path/to/bitcoind HAX_HWI=/path/to/hwi \
//	  go test -tags emulator -run TestEmulator -v ./internal/wallet/
//
// The emulator must be loaded with testMnemonic and no passphrase.
package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/backend"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

type regtestNode struct {
	t    *testing.T
	cli  string
	args []string
	port int
}

func startRegtest(t *testing.T) *regtestNode {
	t.Helper()
	bitcoind := os.Getenv("HAX_BITCOIND")
	if bitcoind == "" {
		t.Skip("HAX_BITCOIND not set")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	dir := t.TempDir()
	common := []string{"-regtest", "-datadir=" + dir, "-rpcuser=hax", "-rpcpassword=hax",
		"-rpcport=" + strconv.Itoa(port)}
	// No -txindex: previous transactions must be found by block hash.
	args := append(common, "-daemon=0", "-listen=0", "-fallbackfee=0.0001", "-blockfilterindex=1")
	cmd := exec.Command(bitcoind, args...) // #nosec G204 G702 -- test-only, path from the developer
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	n := &regtestNode{t: t, cli: filepath.Join(filepath.Dir(bitcoind), "bitcoin-cli"), args: common, port: port}
	t.Cleanup(func() {
		_, _ = n.call("stop")
		_ = cmd.Wait()
	})
	for i := 0; i < 100; i++ {
		if _, err := n.call("getblockcount"); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	n.must("createwallet", "miner")
	n.must("-rpcwallet=miner", "-generate", "101")
	return n
}

func (n *regtestNode) call(args ...string) (string, error) {
	cmd := exec.Command(n.cli, append(append([]string{}, n.args...), args...)...) // #nosec G204 G702 -- test-only
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bitcoin-cli %v: %v: %s", args, err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func (n *regtestNode) must(args ...string) string {
	n.t.Helper()
	out, err := n.call(args...)
	if err != nil {
		n.t.Fatal(err)
	}
	return out
}

func (n *regtestNode) json(v interface{}, args ...string) {
	n.t.Helper()
	if err := json.Unmarshal([]byte(n.must(args...)), v); err != nil {
		n.t.Fatal(err)
	}
}

func (n *regtestNode) rpcBackend() *backend.RPCBackend {
	b, err := backend.NewRPCBackend(&backend.BackendConfig{
		Type: backend.BackendTypeRPC, Network: "testnet",
		RPC: &backend.RPCConfig{Host: "127.0.0.1", Port: n.port, Username: "hax", Password: "hax"},
	})
	if err != nil {
		n.t.Fatal(err)
	}
	return b
}

// regtestAddr re-encodes a testnet address for regtest (same script).
func regtestAddr(t *testing.T, addr string) string {
	t.Helper()
	a, err := btcutil.DecodeAddress(addr, &TestNet4Params)
	if err != nil {
		t.Fatal(err)
	}
	pk, _ := txscript.PayToAddrScript(a)
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pk, &chaincfg.RegressionNetParams)
	if err != nil || len(addrs) != 1 {
		t.Fatalf("re-encode %s: %v", addr, err)
	}
	return addrs[0].EncodeAddress()
}

// fundAddress pays 0.001 to addr, mines it, and returns the UTXO.
func (n *regtestNode) fundAddress(addr string) api.UTXO {
	n.t.Helper()
	txid := n.must("-rpcwallet=miner", "sendtoaddress", regtestAddr(n.t, addr), "0.001")
	n.must("-rpcwallet=miner", "-generate", "1")
	var tx struct {
		BlockHeight int64 `json:"blockheight"`
		Decoded     struct {
			Vout []struct {
				N            uint32  `json:"n"`
				Value        float64 `json:"value"`
				ScriptPubKey struct {
					Address string `json:"address"`
				} `json:"scriptPubKey"`
			} `json:"vout"`
		} `json:"decoded"`
	}
	n.json(&tx, "-rpcwallet=miner", "gettransaction", txid, "true", "true")
	for _, o := range tx.Decoded.Vout {
		if o.ScriptPubKey.Address == regtestAddr(n.t, addr) {
			return api.UTXO{TxID: txid, Vout: o.N, Value: api.Satoshi(o.Value*1e8 + 0.5),
				Status: api.Status{Confirmed: true, BlockHeight: tx.BlockHeight}}
		}
	}
	n.t.Fatal("funding output not found")
	return api.UTXO{}
}

// accept checks a finalized transaction with testmempoolaccept and mines it.
func (n *regtestNode) accept(txHex string) {
	n.t.Helper()
	var res []struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reject-reason"`
	}
	n.json(&res, "testmempoolaccept", `["`+txHex+`"]`)
	if len(res) != 1 || !res[0].Allowed {
		n.t.Fatalf("Core rejected the transaction: %+v", res)
	}
	n.must("sendrawtransaction", txHex)
	n.must("-rpcwallet=miner", "-generate", "1")
}

func (n *regtestNode) analyzeNext(b64 string) string {
	n.t.Helper()
	var a struct {
		Next string `json:"next"`
	}
	n.json(&a, "analyzepsbt", b64)
	return a.Next
}

func emulatorDevice(t *testing.T) (*HWIClient, HardwareDevice) {
	t.Helper()
	hwi := os.Getenv("HAX_HWI")
	if hwi == "" {
		t.Skip("HAX_HWI not set")
	}
	h := &HWIClient{hwiPath: hwi, testnet: true, available: true}
	// HWI 3 only lists emulators when asked to; HWI 2 has no flag.
	out, err := h.runCommand("--emulators", "enumerate")
	if err != nil {
		if out, err = h.runCommand("enumerate"); err != nil {
			t.Fatal(err)
		}
	}
	var devs []HardwareDevice
	if err := json.Unmarshal([]byte(out), &devs); err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if d.Fingerprint == signerTestFingerprint {
			return h, d
		}
	}
	t.Fatalf("no device with fingerprint %s (load the emulator with the test mnemonic): %+v", signerTestFingerprint, devs)
	return nil, HardwareDevice{}
}

// TestEmulatorSingleSig: for each address type, a watch-only wallet from
// the device xpub builds a PSBT, the device signs it through HWI, hax
// finalizes it and Core accepts it.
func TestEmulatorSingleSig(t *testing.T) {
	h, dev := emulatorDevice(t)
	node := startRegtest(t)
	for _, tt := range signerTestTypes {
		t.Run(tt.name, func(t *testing.T) {
			origin := fmt.Sprintf("m/%dh/1h/0h", tt.purpose)
			xpub, err := h.GetXpub(dev, origin)
			if err != nil {
				t.Fatal(err)
			}
			fb := newFundedBackend(&TestNet4Params)
			m, err := NewWalletManager(t.TempDir(), fb)
			if err != nil {
				t.Fatal(err)
			}
			keyOrigin := signerTestFingerprint + strings.TrimPrefix(origin, "m")
			if err := m.CreateHardwareWalletOnNetwork("hw", "emulator-pw", xpub, signerTestFingerprint,
				keyOrigin, tt.addr, string(dev.Type), NetworkTestnet); err != nil {
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
			utxo := node.fundAddress(addr)
			// The previous transaction comes from the real node.
			prev, err := node.rpcBackend().GetRawTransaction(utxo.TxID, utxo.Status.BlockHeight)
			if err != nil {
				t.Fatalf("GetRawTransaction from Core without txindex: %v", err)
			}
			fb.rawTxs[utxo.TxID] = prev
			fb.utxoFor = map[string]api.UTXO{addr: utxo}

			dest, _, _ := m.GetAddressByIndex(0, 7)
			res, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 30_000}}, 2, nil)
			if err != nil {
				t.Fatal(err)
			}
			unsigned, _ := m.ExportPSBTBase64(res)
			if tt.addr != AddressTypeTaproot {
				// Core reports Taproot key-path PSBTs as "updater" even
				// for its own wallet's PSBTs.
				if next := node.analyzeNext(unsigned); next != "signer" {
					t.Fatalf("analyzepsbt next = %q, want signer", next)
				}
			}
			signed, err := h.SignPSBT(dev, unsigned)
			if err != nil {
				t.Fatalf("device signing: %v", err)
			}
			imported, err := m.ImportPSBTFromBase64(signed)
			if err != nil {
				t.Fatal(err)
			}
			txHex, err := m.FinalizePSBTTransaction(imported)
			if err != nil {
				t.Fatal(err)
			}
			var fin struct {
				Hex string `json:"hex"`
			}
			node.json(&fin, "finalizepsbt", signed)
			if fin.Hex != txHex {
				t.Fatal("hax and Core finalized the signed PSBT differently")
			}
			node.accept(txHex)
		})
	}
}

// TestEmulatorMultisig: 2-of-3 with the device, a local seed and an
// unused seed, for each multisig script type. The local seed signs first,
// then the device.
func TestEmulatorMultisig(t *testing.T) {
	h, dev := emulatorDevice(t)
	node := startRegtest(t)
	others := []string{
		"zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
		"all all all all all all all all all all all all",
	}
	for _, st := range []MultisigScriptType{MultisigLegacy, MultisigNestedSegwit, MultisigNativeSegwit} {
		t.Run(fmt.Sprintf("script-type-%d", st), func(t *testing.T) {
			xpub, err := h.GetXpub(dev, fmt.Sprintf("m/48h/1h/0h/%dh", st))
			if err != nil {
				t.Fatal(err)
			}
			cos := []MultisigCosignerConfig{{
				Name: "device", Fingerprint: signerTestFingerprint, Xpub: xpub,
				KeyOrigin:  fmt.Sprintf("%s/48h/1h/0h/%dh", signerTestFingerprint, st),
				DeviceType: string(dev.Type),
			}}
			for i, mn := range others {
				seed, _ := MnemonicToSeed(mn, "")
				w, _ := NewHDWallet(seed, true)
				x, err := w.GetExtendedPublicKeyAtPath(fmt.Sprintf("m/48'/1'/0'/%d'", st))
				if err != nil {
					t.Fatal(err)
				}
				fp, _ := w.GetMasterFingerprintHex()
				c := MultisigCosignerConfig{Name: fmt.Sprintf("seed%d", i), Fingerprint: fp, Xpub: x,
					KeyOrigin: fmt.Sprintf("%s/48h/1h/0h/%dh", fp, st)}
				if i == 0 {
					c.IsMine, c.Mnemonic, c.DeviceType = true, mn, "local"
				}
				cos = append(cos, c)
			}
			fb := newFundedBackend(&TestNet4Params)
			m, err := NewWalletManager(t.TempDir(), fb)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.CreateMultisigWalletOnNetwork("ms", "emulator-pw", 2, cos, st, NetworkTestnet); err != nil {
				t.Fatal(err)
			}
			if err := m.UnlockWallet("ms", "emulator-pw"); err != nil {
				t.Fatal(err)
			}
			m.StopBackgroundSync()
			t.Cleanup(m.LockWallet)

			addr, err := m.GetNewAddress(0)
			if err != nil {
				t.Fatal(err)
			}
			utxo := node.fundAddress(addr)
			prev, err := node.rpcBackend().GetRawTransaction(utxo.TxID, utxo.Status.BlockHeight)
			if err != nil {
				t.Fatal(err)
			}
			fb.rawTxs[utxo.TxID] = prev
			fb.utxoFor = map[string]api.UTXO{addr: utxo}

			dest, _, _ := m.GetMultisigAddress(false, 7)
			res, err := m.CreatePSBTBatchTransaction(0, []TxRecipient{{Address: dest, Amount: 30_000}}, 2, nil)
			if err != nil {
				t.Fatal(err)
			}
			if n, err := m.SignMultisigPSBTTransaction(res); err != nil || n != 1 {
				t.Fatalf("local signature: added %d, %v", n, err)
			}
			partial, _ := m.ExportPSBTBase64(res)
			signed, err := h.SignPSBT(dev, partial)
			if err != nil {
				t.Fatalf("device signing: %v", err)
			}
			imported, err := m.ImportPSBTFromBase64(signed)
			if err != nil {
				t.Fatal(err)
			}
			txHex, err := m.FinalizePSBTTransaction(imported)
			if err != nil {
				t.Fatal(err)
			}
			node.accept(txHex)
		})
	}
}

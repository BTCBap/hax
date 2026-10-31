package models

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui"
	"github.com/BTCBap/hax/internal/wallet"
)

func freezeTestUTXOs() []api.UTXO {
	return []api.UTXO{
		{TxID: strings.Repeat("aa", 32), Vout: 0, Value: 90_000, Status: api.Status{Confirmed: true}},
		{TxID: strings.Repeat("bb", 32), Vout: 1, Value: 10_000, Status: api.Status{Confirmed: true}},
	}
}

func TestUTXOs_FreezeToggle(t *testing.T) {
	mock := NewMockWalletManager()
	m := NewUTXOsModel(mock, nil)
	m.width, m.height = 80, 24
	m.utxos = freezeTestUTXOs()
	m.sortAndUpdateTable()

	key := keyRune('f')
	model, _ := m.Update(key)
	m = model.(UTXOsModel)
	big := freezeTestUTXOs()[0]
	if !mock.IsUTXOFrozen(big.TxID, big.Vout) {
		t.Fatal("f did not freeze the selected coin")
	}
	view := m.View()
	for _, want := range []string{"❄", "COIN FROZEN", "freeze"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if got := strings.Count(stripANSI(view), "frozen"); got != 1 {
		t.Fatalf("frozen appears %d times, want only the summary segment:\n%s", got, stripANSI(view))
	}
	if got := strings.Count(stripANSI(view), "❄"); got != 1 {
		t.Fatalf("❄ appears %d times, want only the frozen row:\n%s", got, stripANSI(view))
	}
	if strings.Contains(view, "●") || strings.Contains(view, "○") {
		t.Fatalf("coins screen still uses ●/○ markers:\n%s", stripANSI(view))
	}
	if !strings.Contains(stripANSI(view), "›") {
		t.Fatalf("coins screen is missing the cursor marker:\n%s", stripANSI(view))
	}
	if !strings.Contains(stripANSI(view), "frozen 0.00090000 (1)") {
		t.Fatalf("summary does not report the frozen total:\n%s", stripANSI(view))
	}
	assertFits(t, view, 80)

	model, _ = m.Update(key)
	m = model.(UTXOsModel)
	view = m.View()
	if mock.IsUTXOFrozen(big.TxID, big.Vout) || strings.Contains(view, "❄") || strings.Contains(view, "COIN FROZEN") {
		t.Fatal("second f did not unfreeze")
	}
	if !strings.Contains(stripANSI(view), "frozen 0.00000000 (0)") {
		t.Fatalf("summary dropped the frozen segment after unfreezing:\n%s", stripANSI(view))
	}
}

func TestUTXOs_AddressUsesSharedFormat(t *testing.T) {
	const (
		addr = "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7"
		txid = "aa"
	)
	mock := NewMockWalletManager()
	mock.GetStateFn = func() wallet.WalletStateSnapshot {
		return wallet.WalletStateSnapshot{
			Transactions: []api.Transaction{{
				TxID: txid,
				Vout: []api.Vout{{ScriptpubkeyAddress: addr, Value: 90_000}},
			}},
			UTXOs: []api.UTXO{{
				TxID: txid, Vout: 0, Value: 90_000,
				Status: api.Status{Confirmed: true},
			}},
		}
	}

	m := NewUTXOsModel(mock, nil)
	m.width, m.height = 80, 24
	m.utxos = mock.GetState().UTXOs
	m.sortAndUpdateTable()

	cols := newUTXOColumns(m.contentWidth())
	want := ansi.Strip(ui.FormatAddress(addr, cols.addr))
	view := m.View()
	if !strings.Contains(ansi.Strip(view), want) {
		t.Fatalf("coins view does not show the formatted address %q:\n%s", want, ansi.Strip(view))
	}

	row := m.renderRow(0, m.page[0], cols, m.getDisplayUnit())
	if !strings.Contains(ansi.Strip(row), want) {
		t.Fatalf("row does not show the formatted address %q:\n%s", want, ansi.Strip(row))
	}
	if got, wantW := ansi.StringWidth(row), cols.total(); got != wantW {
		t.Fatalf("row width = %d, want %d", got, wantW)
	}
	assertFits(t, view, 80)
}

// TestSend_SkipsFrozenCoins: send never lists, selects or counts a
// frozen coin, so MAX can't sweep it.
func TestSend_SkipsFrozenCoins(t *testing.T) {
	mock := NewMockWalletManager()
	utxos := freezeTestUTXOs()
	_ = mock.SetUTXOFrozen(utxos[0].TxID, utxos[0].Vout, true)

	m := NewSendModel(mock, 100_000, utxos)
	if len(m.utxos) != 1 || m.utxos[0].TxID != utxos[1].TxID || m.balance != 10_000 {
		t.Fatalf("cached: utxos=%v balance=%d", m.utxos, m.balance)
	}
	model, _ := m.Update(UTXOsMsg{UTXOs: utxos})
	m = model.(SendModel)
	if len(m.utxos) != 1 || m.balance != 10_000 || m.frozenCount != 1 {
		t.Fatalf("fetched: utxos=%v balance=%d frozen=%d", m.utxos, m.balance, m.frozenCount)
	}
	m.forceSelectAllUTXOs()
	if len(m.utxoSelected) != 1 {
		t.Fatalf("MAX selected %d coins, want only the unfrozen one", len(m.utxoSelected))
	}
	m.step = SendStepUTXOSelection
	if v := m.View(); !strings.Contains(v, "1 frozen coin not shown") {
		t.Fatalf("coin control view doesn't mention the frozen coin:\n%s", v)
	}
}

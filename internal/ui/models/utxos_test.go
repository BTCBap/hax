package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/api"
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

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	model, _ := m.Update(key)
	m = model.(UTXOsModel)
	big := freezeTestUTXOs()[0]
	if !mock.IsUTXOFrozen(big.TxID, big.Vout) {
		t.Fatal("f did not freeze the selected coin")
	}
	view := m.View()
	for _, want := range []string{"❄", "frozen", "COIN FROZEN", "freeze"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	assertFits(t, view, 80)

	model, _ = m.Update(key)
	m = model.(UTXOsModel)
	if mock.IsUTXOFrozen(big.TxID, big.Vout) || strings.Contains(m.View(), "frozen") {
		t.Fatal("second f did not unfreeze")
	}
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

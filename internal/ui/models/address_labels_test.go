package models

import (
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

func labelledAccounts(labels map[string]string) func() []wallet.AccountData {
	return func() []wallet.AccountData {
		return []wallet.AccountData{{
			Addresses: []wallet.AddressData{{Index: 0, Address: "tb1q_our_addr"}, {Index: 1, Address: "tb1q_other"}},
			Labels:    labels,
		}}
	}
}

func TestAddresses_EditLabel(t *testing.T) {
	labels := map[string]string{}
	mock := NewMockWalletManager()
	mock.GetAccountsFn = labelledAccounts(labels)
	var saved [2]string
	mock.SetAddressLabelFn = func(a, l string) error { saved = [2]string{a, l}; labels[a] = l; return nil }

	m := tea.Model(NewAddressesModel(mock, nil))
	m, _ = m.Update(AddressesInitMsg{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(stripANSI(m.View()), "No label · press e to add one") {
		t.Fatal("empty label hint missing")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for _, r := range "rent" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// While editing, letters go to the input, not to shortcuts.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if saved != [2]string{"tb1q_our_addr", "rentq"} {
		t.Fatalf("saved %v", saved)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Label: rentq") {
		t.Fatalf("label not shown:\n%s", view)
	}
	assertFits(t, m.View(), 80)
}

func TestAddresses_LabelColumnWhenWide(t *testing.T) {
	mock := NewMockWalletManager()
	mock.GetAccountsFn = labelledAccounts(map[string]string{"tb1q_our_addr": "cold\x1b[31m storage"})
	m := tea.Model(NewAddressesModel(mock, nil))
	m, _ = m.Update(AddressesInitMsg{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := m.View()
	if !strings.Contains(view, "Label") || !strings.Contains(stripANSI(view), "cold[31m storage") {
		t.Fatalf("label column missing or unsanitized:\n%s", stripANSI(view))
	}
	if strings.Contains(view, "\x1b[31m storage") {
		t.Fatal("escape sequence from a label reached the terminal")
	}
	assertFits(t, view, 120)
}

func TestHistory_SearchMatchesAddressAndAddressLabel(t *testing.T) {
	m, mock := newTestHistory(t)
	mock.GetAccountsFn = func() []wallet.AccountData {
		return []wallet.AccountData{{
			Addresses: []wallet.AddressData{{Address: "tb1q_our_addr"}, {Address: "tb1q_our_change"}},
			Labels:    map[string]string{"tb1q_our_change": "Savings"},
		}}
	}
	m.transactions = sampleTransactions()
	m.loading = false

	for q, want := range map[string]int{
		"savings":       1, // label of our change address (second tx)
		"external_dest": 1, // counterparty address
		"tb1q_our_addr": 2, // both txs touch it
		"nomatch":       0,
	} {
		m.searchQuery = q
		m.updateTableData()
		if got := len(m.filteredTransactions); got != want {
			t.Errorf("search %q: %d results, want %d", q, got, want)
		}
	}
}

func TestTxTouchesAddressMatch_NilPrevout(t *testing.T) {
	tx := api.Transaction{Vin: []api.Vin{{Prevout: nil}}}
	if txTouchesAddressMatch(tx, "x", nil) {
		t.Fatal("matched nothing")
	}
}

package models

import (
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

const uriAddr = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"

func sendWithBalance(t *testing.T) (SendModel, *MockWalletManager) {
	t.Helper()
	mock := NewMockWalletManager()
	mock.GetNetworkFn = func() string { return wallet.NetworkTestnet }
	m := NewSendModel(mock, 1_000_000, []api.UTXO{{TxID: "u", Value: 1_000_000}})
	m.loading = false
	return m, mock
}

func typeAndEnter(t *testing.T, m SendModel, s string) SendModel {
	t.Helper()
	m.addressInput.SetValue(s)
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return out.(SendModel)
}

func TestSend_BIP21FillsRecipientAndPrefillsAmount(t *testing.T) {
	m, _ := sendWithBalance(t)
	m = typeAndEnter(t, m, "bitcoin:"+uriAddr+"?amount=0.0005&label=Coffee&message=Thanks")
	if m.err != nil {
		t.Fatal(m.err)
	}
	if m.step != SendStepAmount || m.recipientAddress != uriAddr {
		t.Fatalf("step %v recipient %q", m.step, m.recipientAddress)
	}
	if got := m.amountInput.Value(); got != "0.00050000" {
		t.Fatalf("amount prefill %q", got)
	}
	view := m.View()
	for _, want := range []string{"Amount from payment request", "Label: Coffee", "Message: Thanks"} {
		if !strings.Contains(stripANSI(view), want) {
			t.Fatalf("amount step missing %q", want)
		}
	}
	// The amount still needs the user's enter.
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = out.(SendModel)
	if len(m.recipients) != 1 || m.recipients[0].Amount != 50_000 || m.recipients[0].Label != "Coffee" {
		t.Fatalf("recipients %+v", m.recipients)
	}
	if m.pendingRequest != nil {
		t.Fatal("request should be consumed once the recipient is added")
	}
}

func TestSend_BIP21SatsMode(t *testing.T) {
	m, _ := sendWithBalance(t)
	m.usesBTC = false
	m = typeAndEnter(t, m, "bitcoin:"+uriAddr+"?amount=0.0005")
	if got := m.amountInput.Value(); got != "50000" {
		t.Fatalf("sats prefill %q", got)
	}
}

func TestSend_BIP21WrongNetworkRejected(t *testing.T) {
	m, _ := sendWithBalance(t)
	m = typeAndEnter(t, m, "bitcoin:bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4?amount=1")
	if m.err == nil || m.step != SendStepAddress || m.recipientAddress != "" {
		t.Fatalf("mainnet URI accepted on testnet4: err=%v step=%v", m.err, m.step)
	}
}

func TestSend_IOnlyImportsWhenAddressEmpty(t *testing.T) {
	m, _ := sendWithBalance(t)
	m.addressInput.SetValue("b")
	m.addressInput.CursorEnd()
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = out.(SendModel)
	if m.showFileBrowser || m.addressInput.Value() != "bi" {
		t.Fatalf("typing 'i' opened import (browser=%v value=%q)", m.showFileBrowser, m.addressInput.Value())
	}
	m.addressInput.SetValue("")
	out, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !out.(SendModel).showFileBrowser {
		t.Fatal("'i' on an empty field should open PSBT import")
	}
}

func TestSend_RequestLabelAppliedAfterBroadcast(t *testing.T) {
	m, mock := sendWithBalance(t)
	var gotTx, gotLabel string
	mock.SetTransactionLabelFn = func(tx, l string) error { gotTx, gotLabel = tx, l; return nil }
	m.recipients = []Recipient{{Address: uriAddr, Amount: 1000, Label: "Coffee"}, {Address: uriAddr, Amount: 1000, Label: "Coffee"}}
	out, _ := m.Update(BroadcastMsg{TxID: "abc"})
	if out.(SendModel).step != SendStepComplete || gotTx != "abc" || gotLabel != "Coffee" {
		t.Fatalf("label %q on %q", gotLabel, gotTx)
	}

	// An existing label is never overwritten.
	gotTx = ""
	mock.GetTransactionLabelFn = func(string) string { return "mine" }
	_, _ = m.Update(BroadcastMsg{TxID: "abc"})
	if gotTx != "" {
		t.Fatal("overwrote an existing label")
	}
}

func TestFeeChoiceSummary(t *testing.T) {
	m := SendModel{}
	for level, want := range map[FeeLevel]string{
		FeeLevelHigh:   "High priority, next block",
		FeeLevelMedium: "Medium priority, ~30 min",
		FeeLevelLow:    "Low priority, ~1 hour",
		FeeLevelCustom: "Medium priority, ~30 min", // custom without a rate falls back to medium
	} {
		m.feeLevel = level
		if got := m.feeChoiceSummary(); got != want {
			t.Errorf("level %v: %q", level, got)
		}
	}
	m.feeLevel, m.customFeeRate = FeeLevelCustom, 3
	if m.feeChoiceSummary() != "Custom rate" {
		t.Error("custom rate not named")
	}
}

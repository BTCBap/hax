package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/wallet"
)

func netKey(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestChooseNetwork_MainnetNeedsConfirm(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step = StepChooseNetwork
	out, _ := m.handleChooseNetwork(netKey("enter")) // cursor 0 = mainnet
	m = out.(WalletSetupModel)
	if !m.confirmMainnet || m.step != StepChooseNetwork {
		t.Fatal("mainnet must prompt for confirmation")
	}
	v := m.viewChooseNetwork()
	if strings.Contains(strings.ToLower(v), "real bitcoin") {
		t.Fatal("warning copy must not say 'real bitcoin'")
	}
	out, _ = m.handleChooseNetwork(netKey("n"))
	m = out.(WalletSetupModel)
	if m.confirmMainnet || m.network != wallet.NetworkTestnet {
		t.Fatal("decline should keep testnet")
	}
	out, _ = m.handleChooseNetwork(netKey("enter"))
	out, _ = out.(WalletSetupModel).handleChooseNetwork(netKey("y"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkMainnet || m.step != StepChooseWordCount {
		t.Fatalf("confirm: network=%s step=%v", m.network, m.step)
	}
}

func TestChooseNetwork_TestnetNoConfirm_Restore(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step, m.isRestore = StepChooseNetwork, true
	out, _ := m.handleChooseNetwork(netKey("down"))
	out, _ = out.(WalletSetupModel).handleChooseNetwork(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkTestnet || m.step != StepRestoreEnterPassword {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
}

func TestNetworkBadge(t *testing.T) {
	if renderNetworkBadge(wallet.NetworkMainnet) != "" {
		t.Fatal("mainnet must be unlabeled")
	}
	if !strings.Contains(renderNetworkBadge(wallet.NetworkTestnet), "TESTNET4") {
		t.Fatal("testnet4 tag missing")
	}
}

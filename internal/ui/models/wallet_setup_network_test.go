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

func TestChooseNetwork_HardwareFlow(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step = StepChooseKeySource
	m.cursor = 1
	out, _ := m.handleChooseKeySource(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepChooseNetwork || m.networkFor != networkForHardware {
		t.Fatalf("step=%v flow=%v", m.step, m.networkFor)
	}
	if m.getPreviousStep() != StepChooseKeySource {
		t.Fatal("back from network should return to key source")
	}
	out, _ = m.handleChooseNetwork(netKey("enter"))
	out, _ = out.(WalletSetupModel).handleChooseNetwork(netKey("y"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkMainnet || m.step != StepHardwareWalletConnect {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if m.coinType() != 0 || m.isTestnet() {
		t.Fatal("mainnet hardware wallet must use coin type 0")
	}
	if m.getPreviousStep() != StepChooseNetwork {
		t.Fatal("back from device scan should return to network choice")
	}
}

func TestChooseNetwork_MultisigFlow(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step, m.networkFor = StepChooseNetwork, networkForMultisig
	if m.getPreviousStep() != StepConfirmPassword {
		t.Fatal("back from multisig network should return to password confirm")
	}
	out, _ := m.handleChooseNetwork(netKey("down"))
	out, _ = out.(WalletSetupModel).handleChooseNetwork(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkTestnet || m.step != StepMultisigChooseType {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if got := wallet.DefaultMultisigAccountPath(wallet.MultisigNativeSegwit, m.isTestnet()); got != "m/48'/1'/0'/2'" {
		t.Fatalf("testnet multisig path %s", got)
	}
}

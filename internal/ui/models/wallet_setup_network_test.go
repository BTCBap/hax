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
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestWalletSetup_FirstScreenIsNetworkWithMainnetSelected(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	if m.step != StepChooseNetwork {
		t.Fatalf("first step = %v, want StepChooseNetwork", m.step)
	}
	if m.cursor != 0 || m.network != wallet.NetworkMainnet {
		t.Fatalf("cursor=%d network=%s, want Mainnet preselected", m.cursor, m.network)
	}
	view := m.viewChooseNetwork()
	if !strings.Contains(view, "Mainnet") || !strings.Contains(view, "Testnet4") {
		t.Fatal("network picker must list Mainnet and Testnet4")
	}

	out, _ := m.handleChooseNetwork(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkMainnet || m.step != StepChooseAction {
		t.Fatalf("network=%s step=%v, want mainnet then create/restore menu", m.network, m.step)
	}
}

func TestChooseNetwork_Testnet4SetsNetwork(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	out, _ := m.handleChooseNetwork(netKey("down"))
	out, _ = out.(WalletSetupModel).handleChooseNetwork(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.network != wallet.NetworkTestnet || m.step != StepChooseAction {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
}

func TestChooseNetwork_EscQuits(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	_, cmd := m.Update(netKey("esc"))
	if cmd == nil {
		t.Fatal("esc on the first network screen should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("esc on the first network screen should quit")
	}
}

func TestChooseAction_EscReturnsToNetwork(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step = StepChooseAction
	out, _ := m.Update(netKey("esc"))
	m = out.(WalletSetupModel)
	if m.step != StepChooseNetwork {
		t.Fatalf("step=%v, want network picker", m.step)
	}
}

func TestChooseKeySource_SoftwareSkipsNetwork(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.network = wallet.NetworkMainnet
	m.step = StepChooseKeySource
	m.cursor = 0

	out, _ := m.handleChooseKeySource(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepEnterPassphrase || m.network != wallet.NetworkMainnet {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if m.getPreviousStep() != StepChooseKeySource {
		t.Fatal("back from passphrase should return to key source")
	}
}

func TestChooseKeySource_HardwareSkipsNetwork(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.network = wallet.NetworkMainnet
	m.step = StepChooseKeySource
	m.cursor = 1

	out, _ := m.handleChooseKeySource(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepHardwareWalletConnect || m.network != wallet.NetworkMainnet {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if m.coinType() != 0 || m.isTestnet() {
		t.Fatal("mainnet hardware wallet must use coin type 0")
	}
	if m.getPreviousStep() != StepChooseKeySource {
		t.Fatal("back from device scan should return to key source")
	}
}

func TestMultisig_DoesNotAskNetworkAgain(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.network = wallet.NetworkTestnet
	m.isMultisig = true
	m.password = "longenough"
	m.step = StepConfirmPassword
	m.confirmPasswordInput.SetValue("longenough")

	out, _ := m.Update(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepMultisigChooseType || m.network != wallet.NetworkTestnet {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if m.getPreviousStep() != StepConfirmPassword {
		t.Fatal("back from multisig type should return to password confirm")
	}
}

func TestRestore_DoesNotAskNetworkAgain(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.isRestore = true
	m.network = wallet.NetworkTestnet
	m.step = StepRestoreChooseScriptType
	m.cursor = 2

	out, _ := m.handleChooseScriptType(netKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepRestoreEnterPassword || m.network != wallet.NetworkTestnet {
		t.Fatalf("network=%s step=%v", m.network, m.step)
	}
	if m.getPreviousStep() != StepRestoreChooseScriptType {
		t.Fatal("back from restore password should return to script type")
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

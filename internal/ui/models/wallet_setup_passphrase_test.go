package models

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BTCBap/hax/internal/wallet"
)

func ppKey(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeIntoFocused(t *testing.T, m WalletSetupModel, text string) WalletSetupModel {
	t.Helper()
	for _, r := range text {
		out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = out.(WalletSetupModel)
	}
	return m
}

func TestPassphrase_EmptyAllowed_Create(t *testing.T) {
	mock := &MockWalletManager{}
	m := NewWalletSetupModel(mock, nil)
	m.walletName = "w"
	m.password = "longenough"
	m.network = wallet.NetworkTestnet
	m.networkFor = networkForSoftware
	m.step = StepEnterPassphrase
	m.passphraseInput.Focus()

	out, _ := m.Update(ppKey("enter")) // empty
	m = out.(WalletSetupModel)
	if m.step != StepConfirmPassphrase {
		t.Fatalf("step=%v", m.step)
	}
	out, _ = m.Update(ppKey("enter")) // confirm empty
	m = out.(WalletSetupModel)
	if m.step != StepChooseWordCount {
		t.Fatalf("empty passphrase should advance to word count, step=%v err=%v", m.step, m.err)
	}
	if m.passphrase != "" {
		t.Fatal("expected empty passphrase")
	}

	m.cursor = 0
	out, _ = m.handleChooseWordCount(ppKey("enter"))
	m = out.(WalletSetupModel)
	if mock.CreatedPassphrase != "" {
		t.Fatalf("create should get empty passphrase, got %q", mock.CreatedPassphrase)
	}
	if m.passphrase != "" || m.passphraseInput.Value() != "" {
		t.Fatal("passphrase should be wiped after create")
	}
}

func TestPassphrase_Match_Create(t *testing.T) {
	mock := &MockWalletManager{}
	m := NewWalletSetupModel(mock, nil)
	m.walletName = "w"
	m.password = "longenough"
	m.network = wallet.NetworkTestnet
	m.networkFor = networkForSoftware
	m.step = StepEnterPassphrase
	m.passphraseInput.Focus()

	m = typeIntoFocused(t, m, "TREZOR")
	out, _ := m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	if m.passphrase != "TREZOR" || m.step != StepConfirmPassphrase {
		t.Fatalf("passphrase=%q step=%v", m.passphrase, m.step)
	}
	m.confirmPassphraseInput.Focus()
	m = typeIntoFocused(t, m, "TREZOR")
	out, _ = m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepChooseWordCount || m.err != nil {
		t.Fatalf("step=%v err=%v", m.step, m.err)
	}

	out, _ = m.handleChooseWordCount(ppKey("enter"))
	m = out.(WalletSetupModel)
	if mock.CreatedPassphrase != "TREZOR" {
		t.Fatalf("create passphrase=%q", mock.CreatedPassphrase)
	}
	if m.passphrase != "" {
		t.Fatal("passphrase not wiped after create")
	}
}

func TestPassphrase_Mismatch_Stays(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.step = StepEnterPassphrase
	m.passphraseInput.Focus()
	m = typeIntoFocused(t, m, "alpha")
	out, _ := m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	m.confirmPassphraseInput.Focus()
	m = typeIntoFocused(t, m, "beta")
	out, _ = m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepConfirmPassphrase {
		t.Fatalf("mismatch must stay on confirm, step=%v", m.step)
	}
	if m.err == nil || !strings.Contains(m.err.Error(), "passphrases do not match") {
		t.Fatalf("expected mismatch error, got %v", m.err)
	}
	if m.confirmPassphraseInput.Value() != "" {
		t.Fatal("confirm input should clear on mismatch")
	}
}

func TestPassphrase_Restore_PassedToBackend(t *testing.T) {
	mock := &MockWalletManager{
		UnlockWalletFn:  func(string, string) error { return nil },
		CreateAccountFn: func(string, wallet.AddressType, string) (uint32, error) { return 0, nil },
	}
	m := NewWalletSetupModel(mock, nil)
	m.isRestore = true
	m.walletName = "restored"
	m.password = "longenough"
	m.restoreMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m.network = wallet.NetworkTestnet
	m.selectedScriptType = wallet.AddressTypeSegWitNative
	m.passphrase = "TREZOR"
	m.step = StepRestoreConfirmPassword
	m.confirmPasswordInput.SetValue("longenough")
	m.confirmPasswordInput.Focus()

	out, _ := m.Update(ppKey("enter"))
	_ = out.(WalletSetupModel)
	if mock.CreatedPassphrase != "TREZOR" {
		t.Fatalf("restore passphrase=%q", mock.CreatedPassphrase)
	}
}

func TestPassphrase_ViewCopy(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.width = 80
	m.step = StepEnterPassphrase
	v := m.viewEnterPassphrase(false)
	for _, needle := range []string{"Optional", "Leave empty", "different wallet", "encrypted wallet file"} {
		if !strings.Contains(v, needle) {
			t.Fatalf("view missing %q", needle)
		}
	}
}

func TestPassphrase_WipeOnEsc(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.networkFor = networkForSoftware
	m.step = StepEnterPassphrase
	m.passphrase = "secret"
	m.passphraseInput.SetValue("secret")
	out, _ := m.Update(ppKey("esc"))
	m = out.(WalletSetupModel)
	if m.step != StepChooseNetwork {
		t.Fatalf("step=%v", m.step)
	}
	if m.passphrase != "" || m.passphraseInput.Value() != "" {
		t.Fatal("passphrase should wipe on back from enter step")
	}
}

func TestPassphrase_RestoreMismatch(t *testing.T) {
	m := NewWalletSetupModel(&MockWalletManager{}, nil)
	m.isRestore = true
	m.step = StepRestoreEnterPassphrase
	m.passphraseInput.Focus()
	m = typeIntoFocused(t, m, "one")
	out, _ := m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	m.confirmPassphraseInput.Focus()
	m = typeIntoFocused(t, m, "two")
	out, _ = m.Update(ppKey("enter"))
	m = out.(WalletSetupModel)
	if m.step != StepRestoreConfirmPassphrase {
		t.Fatalf("step=%v", m.step)
	}
	if m.err == nil || !strings.Contains(m.err.Error(), "passphrases do not match") {
		t.Fatalf("err=%v", m.err)
	}
}

package models

import (
	"strings"
	"testing"

	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

// typeRunes feeds one KeyMsg per rune so shortcuts cannot swallow the text.
func typeRunes(t *testing.T, m SendModel, s string) SendModel {
	t.Helper()
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = asSendModel(t, updated)
	}
	return m
}

// mockForPaste returns a wallet mock that decodes pasted base64 through the
// real wallet PSBT parser.
func mockForPaste(t *testing.T) *MockWalletManager {
	t.Helper()
	return &MockWalletManager{
		ImportPSBTFromBase64Fn: func(s string) (*wallet.PSBTResult, error) {
			packet, err := wallet.ImportPSBT(s)
			if err != nil {
				return nil, err
			}
			return &wallet.PSBTResult{Packet: packet}, nil
		},
		IsMultisigFn: func() bool { return false },
	}
}

// runCmd executes cmd and, for a batch, returns the first PSBTFileMsg found.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if found := runCmd(sub); found != nil {
			if _, isFile := found.(PSBTFileMsg); isFile {
				return found
			}
		}
	}
	return msg
}

func requirePSBTFileMsg(t *testing.T, msg tea.Msg) PSBTFileMsg {
	t.Helper()
	if msg == nil {
		t.Fatalf("expected a PSBTFileMsg, got nil message")
	}
	fileMsg, ok := msg.(PSBTFileMsg)
	if !ok {
		t.Fatalf("expected a PSBTFileMsg, got %T", msg)
	}
	return fileMsg
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// ─── Tests ─────────────────────────────────────────────────────────────────

// TestSend_ManualPathFromAddressStep covers entering a manual PSBT path from
// the address step and esc returning to the file browser.
func TestSend_ManualPathFromAddressStep(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)

	updated, _ := m.Update(keyRunes("i"))
	m = asSendModel(t, updated)
	if !m.showFileBrowser {
		t.Fatalf("pressing i at the address step did not open the file browser")
	}

	updated, _ = m.Update(keyRunes("p"))
	m = asSendModel(t, updated)
	if !m.showFileInput {
		t.Fatalf("pressing p in the file browser did not open the manual path input")
	}
	if m.showFileBrowser {
		t.Fatalf("file browser should be hidden after pressing p")
	}

	m = typeRunes(t, m, "~/Desktop/pbqv.psbt")

	if got := m.psbtFileInput.Value(); got != "~/Desktop/pbqv.psbt" {
		t.Fatalf("psbtFileInput.Value() = %q, want %q", got, "~/Desktop/pbqv.psbt")
	}
	if got := m.addressInput.Value(); got != "" {
		t.Fatalf("addressInput.Value() = %q, want empty", got)
	}
	if m.step != SendStepAddress {
		t.Fatalf("step = %v, want SendStepAddress", m.step)
	}
	if m.showFileBrowser {
		t.Fatalf("file browser should stay hidden while typing the path")
	}
	if view := m.View(); !strings.Contains(view, "Enter path to the signed PSBT file") {
		t.Fatalf("view does not show the PSBT path prompt (view length %d)", len(view))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = asSendModel(t, updated)
	if m.showFileInput {
		t.Fatalf("esc should close the manual path input")
	}
	if !m.showFileBrowser {
		t.Fatalf("esc should return to the file browser")
	}
}

// TestSend_ManualPathEnterLoads covers submitting a nonexistent path: the
// command must attempt an import and report the failure.
func TestSend_ManualPathEnterLoads(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)

	updated, _ := m.Update(keyRunes("i"))
	m = asSendModel(t, updated)
	updated, _ = m.Update(keyRunes("p"))
	m = asSendModel(t, updated)
	if !m.showFileInput {
		t.Fatalf("pressing p in the file browser did not open the manual path input")
	}

	m = typeRunes(t, m, "/nonexistent/x.psbt")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = asSendModel(t, updated)
	if cmd == nil {
		t.Fatalf("pressing enter on a manual path returned no command")
	}
	fileMsg := requirePSBTFileMsg(t, runCmd(cmd))
	if fileMsg.Operation != "import" {
		t.Fatalf("PSBTFileMsg.Operation = %q, want %q", fileMsg.Operation, "import")
	}
	if fileMsg.Err == nil {
		t.Fatalf("PSBTFileMsg.Err = nil, want an error for a nonexistent path")
	}
}

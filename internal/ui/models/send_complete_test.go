package models

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/BTCBap/hax/internal/wallet"
)

const sentTestTxID = "b1769e4f6a2c8d0e3b5f7a9c1d2e4f6081a3b5c7d9e0f2a4b6c8d0e2f4a67685a2"

func newSentTestModel(t *testing.T, network string, width int) SendModel {
	t.Helper()
	mock := NewMockWalletManager()
	mock.GetNetworkFn = func() string { return network }
	m := NewSendModel(mock, 1_500_000, nil)
	m.width = width
	m.height = 24
	m.step = SendStepComplete
	m.txID = sentTestTxID
	m.recipients = []Recipient{{Address: "tb1q9vza2e8x573nczrlzms0wvx3gsqjx7vavgkx0l", Amount: 100_000}}
	m.estimatedFee = 141
	return m
}

func TestTransactionSentScreenLinksFullURL(t *testing.T) {
	for _, network := range []string{wallet.NetworkTestnet, wallet.NetworkMainnet} {
		url := wallet.ExplorerTxURL(network, sentTestTxID)
		for _, width := range []int{80, 120, 200} {
			m := newSentTestModel(t, network, width)
			out := m.View()
			if !strings.Contains(out, "\x1b]8;") {
				t.Fatalf("%s width %d: view has no OSC 8 hyperlink", network, width)
			}
			if !strings.Contains(out, url) {
				t.Errorf("%s width %d: hyperlink target is not the full URL", network, width)
			}

			plain := ansi.Strip(out)
			for i, line := range strings.Split(plain, "\n") {
				if cells := ansi.StringWidth(line); cells > width {
					t.Errorf("%s width %d: line %d is %d cells, want at most %d", network, width, i, cells, width)
				}
			}

			urlLine := ""
			for _, line := range strings.Split(plain, "\n") {
				if strings.Contains(line, "https://") {
					urlLine = line
					break
				}
			}
			if urlLine == "" {
				t.Fatalf("%s width %d: no visible URL line", network, width)
			}
			if width <= 80 {
				if !strings.Contains(urlLine, "…") {
					t.Errorf("%s width %d: visible URL is not ellipsized: %q", network, width, urlLine)
				}
				if strings.Contains(urlLine, url) {
					t.Errorf("%s width %d: visible URL should be shortened: %q", network, width, urlLine)
				}
			}
		}
	}
}

func TestTransactionSentScreenHelpFits(t *testing.T) {
	m := newSentTestModel(t, wallet.NetworkTestnet, 80)
	plain := ansi.Strip(m.View())
	if !strings.Contains(plain, "u copy link") {
		t.Fatalf("help does not mention u copy link:\n%s", plain)
	}
	if !strings.Contains(plain, "q/esc dashboard") {
		t.Fatalf("help does not use the short dashboard label:\n%s", plain)
	}
	if strings.Contains(plain, "back to dashboard") {
		t.Fatalf("help still uses the long dashboard label:\n%s", plain)
	}
}

func TestTransactionSentCopyLink(t *testing.T) {
	fc, cleanup := withFakeClipboard(t)
	defer cleanup()
	// Keep the auto-clear goroutine asleep so it cannot outlive the test's
	// restored clipboard hooks.
	clipboardClearTimeout = time.Hour

	origSet := clipboardSet
	var oscPayload string
	clipboardSet = func(text string) tea.Cmd {
		oscPayload = text
		return func() tea.Msg { return nil }
	}
	defer func() { clipboardSet = origSet }()

	m := newSentTestModel(t, wallet.NetworkTestnet, 80)
	updated, cmd := m.Update(keyRune('u'))
	ptr, ok := updated.(*SendModel)
	if !ok {
		t.Fatalf("update returned %T, want *SendModel", updated)
	}
	sent := *ptr
	url := wallet.ExplorerTxURL(wallet.NetworkTestnet, sentTestTxID)
	if got := fc.get(); got != url {
		t.Errorf("clipboard = %q, want the full URL %q", got, url)
	}
	if oscPayload != url {
		t.Errorf("OSC 52 payload = %q, want the full URL %q", oscPayload, url)
	}
	if cmd == nil {
		t.Fatal("u returned no command")
	}

	if !sent.showCopied || sent.lastAction != "link" {
		t.Errorf("copy state = shown %v action %q, want a link copy confirmation", sent.showCopied, sent.lastAction)
	}
	if view := ansi.Strip(sent.View()); !strings.Contains(view, "link copied") {
		t.Errorf("helper text does not show the copy confirmation:\n%s", view)
	}
	if time.Since(sent.copiedTime) > 2*time.Second {
		t.Error("copy confirmation timestamp is stale")
	}
}

package models

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/BTCBap/hax/internal/backend"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// trustPresentedKey is swapped out in tests.
var trustPresentedKey = backend.TrustPresentedKey

// pinMismatch extracts a changed-server-key error from the sync state.
func pinMismatch(state wallet.WalletStateSnapshot) *backend.PinMismatchError {
	var pe *backend.PinMismatchError
	if state.Status == wallet.SyncStatusError && errors.As(state.LastError, &pe) {
		return pe
	}
	return nil
}

func shortFingerprint(fp string) string {
	if len(fp) <= 16 {
		return fp
	}
	return fp[:8] + "…" + fp[len(fp)-8:]
}

// handleRetrustKey handles the confirmation prompt. Accepting requires an
// explicit "y"; any other key cancels so a stray keypress can't trust a key.
func (m MainDashboardModel) handleRetrustKey(msg tea.KeyPressMsg) (screenModel, tea.Cmd) {
	pe := m.retrustPrompt
	if msg.String() != "y" {
		m.retrustPrompt = nil
		return m, nil
	}
	if err := trustPresentedKey(pe); err != nil {
		m.retrustErr = err
		m.retrustPrompt = nil
		return m, nil
	}
	m.retrustPrompt = nil
	m.retrustErr = nil
	m.loading = true
	return m, tea.Batch(m.spinner.Tick, m.refreshCacheCmd())
}

func (m MainDashboardModel) renderRetrustPanel(state wallet.WalletStateSnapshot) string {
	width := m.width - 2
	if width > 122 {
		width = 122
	}
	if width < 30 {
		width = 30
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ErrorText.GetForeground()).Padding(0, 1).Width(width)

	if pe := m.retrustPrompt; pe != nil {
		body := styles.ErrorText.Render("Trust the new key for "+pe.Server+"?") + "\n\n" +
			styles.DimText.Render("Previously trusted: ") + shortFingerprint(pe.Pinned) + "\n" +
			styles.DimText.Render("Now presented:      ") + shortFingerprint(pe.Presented) + "\n\n" +
			styles.NormalText.Render("Only accept if the server operator announced a key change or you\n"+
				"checked the new fingerprint another way. An unexpected change can\n"+
				"mean someone is intercepting your connection.") + "\n\n" +
			styles.RenderHelp(styles.HK("y", "trust new key"), styles.HK("any other key", "cancel"))
		return box.Render(body)
	}

	pe := pinMismatch(state)
	if pe == nil {
		if m.retrustErr != nil {
			return box.Render(styles.ErrorText.Render(fmt.Sprintf("Could not update trusted key: %v", m.retrustErr)))
		}
		return ""
	}
	body := styles.ErrorText.Render("Server key changed: "+pe.Server) + "\n" +
		styles.NormalText.Render("hax stopped connecting because the server's TLS key no longer matches\n"+
			"the one trusted earlier.") + "\n\n" +
		styles.RenderHelp(styles.HK("T", "review and re-trust"), styles.HK("t", "settings to switch server"))
	if m.retrustErr != nil {
		body += "\n" + styles.ErrorText.Render(m.retrustErr.Error())
	}
	return box.Render(body)
}

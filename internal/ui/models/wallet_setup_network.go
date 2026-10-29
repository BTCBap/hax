package models

import (
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

func (m WalletSetupModel) isTestnet() bool { return m.network != wallet.NetworkMainnet }

func (m WalletSetupModel) coinType() uint32 { return wallet.CoinTypeForNetwork(m.network) }

// handleChooseNetwork picks the wallet's network. It is the first step of
// onboarding, so the choice continues to the create/restore menu.
func (m WalletSetupModel) handleChooseNetwork(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "enter":
		if m.cursor == 0 {
			m.network = wallet.NetworkMainnet
		} else {
			m.network = wallet.NetworkTestnet
		}
		m.cursor = 0
		m.step = StepChooseAction
	}
	return m, nil
}

func (m WalletSetupModel) viewChooseNetwork() string {
	info := styles.DimText.Render("Which Bitcoin network is this wallet for?")
	options := []struct{ name, desc string }{
		{"Mainnet", "Bitcoin network."},
		{"Testnet4", "Test network; coins have no value."},
	}
	menu := ""
	for i, opt := range options {
		if m.cursor == i {
			menu += styles.SelectedItemStyle.Render("▶ "+opt.name) + "\n"
			menu += styles.DimText.Render("   "+opt.desc) + "\n"
		} else {
			menu += styles.UnselectedItemStyle.Render("  "+opt.name) + "\n"
		}
	}
	help := styles.RenderHelp(
		styles.HK("↑/↓", "navigate"),
		styles.HK("enter", "select"),
		styles.HK("esc", "quit"),
	)
	return styles.CenterHorizontal(info+"\n\n"+menu+help, m.width)
}

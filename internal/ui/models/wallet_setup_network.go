package models

import (
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

// networkFlow records which setup flow the network step belongs to, so the
// step can be shared by software, hardware and multisig wallets.
type networkFlow int

const (
	networkForSoftware networkFlow = iota
	networkForHardware
	networkForMultisig
)

func (m WalletSetupModel) isTestnet() bool { return m.network != wallet.NetworkMainnet }

func (m WalletSetupModel) coinType() uint32 { return wallet.CoinTypeForNetwork(m.network) }

func (m WalletSetupModel) handleChooseNetwork(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMainnet {
		switch msg.String() {
		case "y", "Y":
			m.confirmMainnet = false
			m.network = wallet.NetworkMainnet
			return m.advanceFromNetwork()
		case "n", "N":
			m.confirmMainnet = false
		}
		return m, nil
	}
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
			// Mainnet requires an explicit second confirmation.
			m.confirmMainnet = true
			return m, nil
		}
		m.network = wallet.NetworkTestnet
		return m.advanceFromNetwork()
	}
	return m, nil
}

func (m WalletSetupModel) advanceFromNetwork() (tea.Model, tea.Cmd) {
	m.cursor = 0
	switch m.networkFor {
	case networkForHardware:
		m.step = StepHardwareWalletConnect
		m.hwiLoading = true
		m.hwiError = nil
		m.hwiDevices = nil
		return m, tea.Batch(m.spinner.Tick, m.enumerateHWIDevicesForSingleSig())
	case networkForMultisig:
		m.step = StepMultisigChooseType
		return m, nil
	}
	if m.isRestore {
		m.step = StepRestoreEnterPassword
		return m, m.passwordInput.Focus()
	}
	m.wipePassphrase()
	m.step = StepEnterPassphrase
	return m, m.passphraseInput.Focus()
}

func (m WalletSetupModel) viewChooseNetwork() string {
	if m.confirmMainnet {
		warning := styles.RenderErrorBox(
			"Mainnet wallet\n\n"+
				"hax is beta software and has not been independently audited.\n"+
				"The network cannot be changed after creation.", m.width)
		help := styles.RenderHelp(
			styles.HK("y", "continue on mainnet"),
			styles.HK("n/esc", "go back"),
		)
		return styles.CenterHorizontal(warning+"\n\n"+help, m.width)
	}

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
		styles.HK("esc", "back"),
	)
	return styles.CenterHorizontal(info+"\n\n"+menu+help, m.width)
}

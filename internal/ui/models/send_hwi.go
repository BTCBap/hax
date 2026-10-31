package models

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
)

// enumerateHWIDevices lists connected hardware wallets via HWI
func (m SendModel) enumerateHWIDevices() tea.Cmd {
	walletMgr := m.walletManager

	return func() tea.Msg {
		devices, err := walletMgr.ListHardwareWallets()
		return HWIEnumerateMsg{Devices: devices, Err: err}
	}
}

// signWithHWI signs the PSBT using the selected hardware wallet
func (m SendModel) signWithHWI(device wallet.HardwareDevice) tea.Cmd {
	walletMgr := m.walletManager
	psbtBase64 := m.psbtBase64

	return func() tea.Msg {
		signedPSBT, err := walletMgr.SignPSBTWithHardwareWallet(psbtBase64, device)
		if err != nil {
			return HWISignMsg{Err: err}
		}

		// Parse the signed PSBT to count signatures. It must describe the
		// same unsigned transaction the user reviewed: a compromised HWI
		// or device could otherwise return signatures over other outputs.
		original, err := walletMgr.ImportPSBTFromBase64(psbtBase64)
		if err != nil {
			return HWISignMsg{Err: fmt.Errorf("failed to read the PSBT that was sent: %w", err)}
		}
		result, err := walletMgr.ImportPSBTFromBase64(signedPSBT)
		if err != nil {
			return HWISignMsg{Err: fmt.Errorf("failed to parse signed PSBT: %w", err)}
		}
		if !wallet.SameUnsignedTransaction(original.Packet, result.Packet) {
			return HWISignMsg{Err: fmt.Errorf("hardware wallet returned a different transaction; refusing it")}
		}

		sigsAdded := wallet.CountPSBTSigners(result.Packet)

		return HWISignMsg{
			SignedPSBT: signedPSBT,
			SigsAdded:  sigsAdded,
			Err:        nil,
		}
	}
}

// findMatchingDevice returns the index of the first enumerated device whose
// fingerprint matches the wallet's stored hardware config (single-sig) or any
// hardware cosigner (multisig). Returns 0 if no match is found.
func (m SendModel) findMatchingDevice(devices []wallet.HardwareDevice) int {
	if hwConfig := m.walletManager.GetHardwareWalletConfig(); hwConfig != nil && hwConfig.Fingerprint != "" {
		for i, dev := range devices {
			if dev.Fingerprint == hwConfig.Fingerprint {
				return i
			}
		}
	}
	if msConfig := m.walletManager.GetMultisigConfig(); msConfig != nil {
		for i, dev := range devices {
			for _, cosigner := range msConfig.Cosigners {
				if cosigner.DeviceType != "" && cosigner.DeviceType != "local" && dev.Fingerprint == cosigner.Fingerprint {
					return i
				}
			}
		}
	}
	return 0
}

func (m SendModel) viewHWIDeviceSelection() string {
	header := styles.TitleStyle.Render("🔌 Hardware Wallet Signing")
	subtitle := styles.SubtitleStyle.Render("Select device to sign with (via HWI)")

	var content strings.Builder

	// Show loading indicator
	if m.loading {
		content.WriteString(m.spinner.View() + " Detecting hardware wallets...\n")
	} else if m.hwiError != nil {
		content.WriteString(styles.RenderErrorBox("⚠ "+m.hwiError.Error(), panelContentWidth(m.width)) + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(hwiErrorHints(m.hwiError)))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else if len(m.hwiDevices) == 0 {
		content.WriteString(styles.DimText.Render("No hardware wallets detected.") + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(wallet.DeviceConnectionHints()))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else {
		content.WriteString(styles.NormalText.Render("Connected devices:") + "\n\n")
		content.WriteString(renderDeviceMenuFit(hardwareDeviceChoices(m.hwiDevices),
			m.selectedHWDevice, panelContentWidth(m.width)))
	}

	// Instructions
	instructions := styles.DimText.Render(
		"\nThe device will prompt you to confirm the transaction.\n" +
			"Review all details on your hardware wallet screen.")

	help := styles.RenderHelp(
		styles.HK("↑/↓", "select"),
		styles.HK("enter", "sign"),
		styles.HK("r", "retry"),
		styles.HK("esc", "back"),
	)

	return centeredPanel(header+"\n"+subtitle+"\n\n"+
		content.String()+
		instructions+"\n\n"+
		help, m.width, m.height)
}

// renderHWITroubleshootingHints formats troubleshooting hints as a bulleted list
func renderHWITroubleshootingHints(hints []string) string {
	if len(hints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.WarningText.Render("Troubleshooting:") + "\n")
	for _, hint := range hints {
		b.WriteString(styles.DimText.Render("  • "+hint) + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// hwiErrorHints extracts troubleshooting hints from an error, returning general
// device connection hints if the error is not an *wallet.HWIError
func hwiErrorHints(err error) []string {
	if hwiErr, ok := err.(*wallet.HWIError); ok {
		return hwiErr.TroubleshootingHints()
	}
	return wallet.DeviceConnectionHints()
}

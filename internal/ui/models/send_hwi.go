package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
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

		// Parse the signed PSBT to count signatures
		result, err := walletMgr.ImportPSBTFromBase64(signedPSBT)
		if err != nil {
			return HWISignMsg{Err: fmt.Errorf("failed to parse signed PSBT: %w", err)}
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
		content.WriteString(styles.RenderErrorBox("⚠ "+m.hwiError.Error(), m.width) + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(hwiErrorHints(m.hwiError)))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else if len(m.hwiDevices) == 0 {
		content.WriteString(styles.DimText.Render("No hardware wallets detected.") + "\n\n")
		content.WriteString(renderHWITroubleshootingHints(wallet.DeviceConnectionHints()))
		content.WriteString(styles.DimText.Render("Press 'r' to retry or 'esc' to go back.") + "\n")
	} else {
		content.WriteString(styles.NormalText.Render("Connected devices:") + "\n\n")

		for i, device := range m.hwiDevices {
			// Format device info with model and type
			deviceName := string(device.Type)
			if device.Model != "" {
				deviceName = device.Model
			}

			// Show fingerprint if available
			fp := device.Fingerprint
			if fp == "" || fp == "00000000" {
				fp = "unknown"
			} else if len(fp) > 8 {
				fp = fp[:8]
			}

			line := fmt.Sprintf("  %s [%s]", deviceName, fp)

			// Highlight selected device
			if i == m.selectedHWDevice {
				line = styles.HighlightText.Render("► " + line)
			} else {
				line = styles.DimText.Render("  " + line)
			}

			// Show status flags with warnings
			if device.NeedsPin {
				line += " " + styles.WarningText.Render("⚠ needs PIN")
			}
			if device.NeedsPass {
				line += " " + styles.DimText.Render("(needs passphrase)")
			}
			if device.Error != "" {
				line += "\n    " + styles.ErrorText.Render(device.Error)
			}

			content.WriteString(line + "\n")
		}

		// Show PIN warning for selected device
		if m.selectedHWDevice >= 0 && m.selectedHWDevice < len(m.hwiDevices) {
			selected := m.hwiDevices[m.selectedHWDevice]
			if selected.NeedsPin {
				content.WriteString("\n" + styles.WarningText.Render(
					"⚠ Enter PIN on your device first, then press Enter to sign.") + "\n")
			}
		}
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

	return header + "\n" + subtitle + "\n\n" +
		content.String() +
		instructions + "\n\n" +
		help
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

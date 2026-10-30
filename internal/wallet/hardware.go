package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// HardwareWalletType represents supported hardware wallet types
type HardwareWalletType string

const (
	HWTypeTrezor   HardwareWalletType = "trezor"
	HWTypeLedger   HardwareWalletType = "ledger"
	HWTypeColdcard HardwareWalletType = "coldcard"
	HWTypeKeepKey  HardwareWalletType = "keepkey"
	HWTypeBitBox   HardwareWalletType = "bitbox02"
	HWTypeJade     HardwareWalletType = "jade"
)

// HardwareDevice represents a connected hardware wallet
type HardwareDevice struct {
	Type        HardwareWalletType `json:"type"`
	Model       string             `json:"model"`
	Path        string             `json:"path"`
	Fingerprint string             `json:"fingerprint"`
	NeedsPin    bool               `json:"needs_pin_sent"`
	NeedsPass   bool               `json:"needs_passphrase_sent"`
	Error       string             `json:"error,omitempty"`
}

// HWIErrorCode classifies HWI errors for actionable user messages
type HWIErrorCode int

const (
	HWIErrNotInstalled HWIErrorCode = iota
	HWIErrNoDevice
	HWIErrDeviceLocked
	HWIErrUserCancelled
	HWIErrUnknownMultisigWallet
	HWIErrSigningFailed
	HWIErrDeviceError
)

// ErrHWINotInstalled is a reusable sentinel for the common "not installed" error
var ErrHWINotInstalled = &HWIError{Code: HWIErrNotInstalled, RawMsg: "hwi not installed"}

// HWIError wraps HWI errors with a classified code and actionable message
type HWIError struct {
	Code   HWIErrorCode
	RawMsg string // Original error string
}

func (e *HWIError) Error() string {
	return e.UserMessage()
}

// UserMessage returns an actionable message for the user
func (e *HWIError) UserMessage() string {
	switch e.Code {
	case HWIErrNotInstalled:
		return "HWI not installed. Install with: brew install pipx && pipx install hwi"
	case HWIErrNoDevice:
		return "No device connected. Plug in your hardware wallet and try again."
	case HWIErrDeviceLocked:
		return "Device is locked. Enter your PIN on the device first, then retry."
	case HWIErrUserCancelled:
		return "Action cancelled on device."
	case HWIErrUnknownMultisigWallet:
		return "Device rejected PSBT as unknown multisig wallet. On Coldcard: import wallet config first, or enable Settings > Multisig Wallets > Allow from PSBT."
	case HWIErrSigningFailed:
		return "Signing failed: " + e.RawMsg
	case HWIErrDeviceError:
		return "Device error: " + e.RawMsg
	default:
		return "HWI error: " + e.RawMsg
	}
}

// classifyHWIError parses an HWI error string and returns a classified error
func classifyHWIError(rawErr string) *HWIError {
	lower := strings.ToLower(rawErr)

	switch {
	case strings.Contains(lower, "no device"):
		return &HWIError{Code: HWIErrNoDevice, RawMsg: rawErr}
	case strings.Contains(lower, "not found"):
		return &HWIError{Code: HWIErrNoDevice, RawMsg: rawErr}
	case strings.Contains(lower, "needs pin"),
		strings.Contains(lower, "pin required"),
		strings.Contains(lower, "device locked"),
		strings.Contains(lower, "pin needed"):
		return &HWIError{Code: HWIErrDeviceLocked, RawMsg: rawErr}
	case strings.Contains(lower, "cancelled"),
		strings.Contains(lower, "canceled"),
		strings.Contains(lower, "rejected"),
		strings.Contains(lower, "user refused"),
		strings.Contains(lower, "action cancelled"),
		strings.Contains(lower, "conditions not satisfied"):
		return &HWIError{Code: HWIErrUserCancelled, RawMsg: rawErr}
	case strings.Contains(lower, "unknown multisig wallet"),
		strings.Contains(lower, "unknown wallet"),
		strings.Contains(lower, "wallet policy not registered"),
		strings.Contains(lower, "wallet not registered"),
		strings.Contains(lower, "policy is not known"),
		strings.Contains(lower, "policy not known"),
		strings.Contains(lower, "not in multisig wallet"),
		strings.Contains(lower, "unknown policy"):
		return &HWIError{Code: HWIErrUnknownMultisigWallet, RawMsg: rawErr}
	case strings.Contains(lower, "sign") && (strings.Contains(lower, "fail") || strings.Contains(lower, "error")):
		return &HWIError{Code: HWIErrSigningFailed, RawMsg: rawErr}
	default:
		return &HWIError{Code: HWIErrDeviceError, RawMsg: rawErr}
	}
}

// HWIClient provides interface to Bitcoin Core's Hardware Wallet Interface (HWI)
// HWI is a Python tool that provides a common interface to hardware wallets.
// See: https://github.com/bitcoin-core/HWI
type HWIClient struct {
	hwiPath   string // Path to hwi executable
	testnet   bool   // Whether to use testnet
	available bool   // Whether hwi is available
}

// NewHWIClient creates a new HWI client.
// Returns nil if HWI is not installed.
func NewHWIClient(testnet bool) *HWIClient {
	path := resolveHWIPath()
	if path == "" {
		return &HWIClient{available: false}
	}

	return &HWIClient{
		hwiPath:   path,
		testnet:   testnet,
		available: true,
	}
}

// resolveHWIPath finds an executable HWI that the current user can trust.
// HAX_HWI_PATH takes priority; otherwise PATH and common install locations
// are tried. Candidates that are not regular files or are group/world
// writable are skipped so a planted binary cannot run as the wallet (H3-30).
func resolveHWIPath() string {
	var candidates []string
	if configured := os.Getenv("HAX_HWI_PATH"); configured != "" {
		candidates = append(candidates, configured)
	} else {
		if path, err := exec.LookPath("hwi"); err == nil {
			candidates = append(candidates, path)
		}
		homeDir, _ := os.UserHomeDir()
		candidates = append(candidates, "/usr/local/bin/hwi", "/opt/homebrew/bin/hwi")
		if homeDir != "" {
			candidates = append(candidates, filepath.Join(homeDir, ".local", "bin", "hwi"))
		}
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs) //nolint:gosec // HAX_HWI_PATH and install locations are user-chosen
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0o022 != 0 {
			continue
		}
		return abs
	}
	return ""
}

// IsAvailable returns true if HWI is installed and accessible
func (h *HWIClient) IsAvailable() bool {
	return h.available
}

// GetVersion returns the HWI version string
func (h *HWIClient) GetVersion() (string, error) {
	if !h.available {
		return "", ErrHWINotInstalled
	}

	output, err := h.runCommand("--version")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// EnumerateDevices lists all connected hardware wallets
func (h *HWIClient) EnumerateDevices() ([]HardwareDevice, error) {
	if !h.available {
		return nil, ErrHWINotInstalled
	}

	output, err := h.runCommand("enumerate")
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate devices: %w", err)
	}

	var devices []HardwareDevice
	if err := json.Unmarshal([]byte(output), &devices); err != nil {
		return nil, fmt.Errorf("failed to parse device list: %w", err)
	}
	for i := range devices {
		devices[i].Model = sanitizeHWIText(devices[i].Model)
		devices[i].Error = sanitizeHWIText(devices[i].Error)
	}

	return devices, nil
}

// sanitizeHWIText strips control and format characters from text that may
// come from a device or a compromised HWI binary before the UI renders it.
func sanitizeHWIText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

// GetXpub retrieves the extended public key at the given derivation path
func (h *HWIClient) GetXpub(device HardwareDevice, path string) (string, error) {
	if !h.available {
		return "", ErrHWINotInstalled
	}

	args := []string{"-d", device.Path, "-t", string(device.Type), "getxpub", path}
	output, err := h.runCommand(args...)
	if err != nil {
		return "", fmt.Errorf("failed to get xpub: %w", err)
	}

	// Parse JSON response
	var result struct {
		Xpub string `json:"xpub"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return "", fmt.Errorf("failed to parse xpub response: %w", err)
	}

	return result.Xpub, nil
}

// SignPSBT signs a PSBT using the hardware wallet
// Returns the signed PSBT in base64 format
func (h *HWIClient) SignPSBT(device HardwareDevice, psbtBase64 string) (string, error) {
	if !h.available {
		return "", ErrHWINotInstalled
	}

	args := []string{"-d", device.Path, "-t", string(device.Type), "signtx", psbtBase64}
	output, err := h.runCommand(args...)
	if err != nil {
		return "", fmt.Errorf("failed to sign PSBT: %w", err)
	}

	// Parse JSON response
	var result struct {
		PSBT string `json:"psbt"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return "", fmt.Errorf("failed to parse sign response: %w", err)
	}

	if result.PSBT == "" {
		return "", fmt.Errorf("device returned empty PSBT — signing may have been cancelled")
	}

	return result.PSBT, nil
}

// DisplayAddress shows an address on the hardware wallet for verification
func (h *HWIClient) DisplayAddress(device HardwareDevice, path string, addrType string) (string, error) {
	if !h.available {
		return "", ErrHWINotInstalled
	}

	args := []string{"-d", device.Path, "-t", string(device.Type), "displayaddress", "--path", path}
	if addrType != "" {
		args = append(args, "--addr-type", addrType)
	}

	output, err := h.runCommand(args...)
	if err != nil {
		return "", fmt.Errorf("failed to display address: %w", err)
	}

	// Parse JSON response
	var result struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return "", fmt.Errorf("failed to parse address response: %w", err)
	}

	return result.Address, nil
}

// PromptPin prompts the user to enter PIN on the device
func (h *HWIClient) PromptPin(device HardwareDevice) error {
	if !h.available {
		return ErrHWINotInstalled
	}

	args := []string{"-d", device.Path, "-t", string(device.Type), "promptpin"}
	_, err := h.runCommand(args...)
	if err != nil {
		return fmt.Errorf("failed to prompt for PIN: %w", err)
	}

	return nil
}

// SendPin sends the PIN to the device (for Trezor-style PIN entry)
func (h *HWIClient) SendPin(device HardwareDevice, pin string) error {
	if !h.available {
		return ErrHWINotInstalled
	}

	// Validate PIN format (should be numeric)
	for _, c := range pin {
		if c < '0' || c > '9' {
			return fmt.Errorf("invalid PIN format")
		}
	}

	args := []string{"-d", device.Path, "-t", string(device.Type), "sendpin"}
	_, err := h.runCommandInput(pin+"\n", args...)
	if err != nil {
		return fmt.Errorf("failed to send PIN: %w", err)
	}

	return nil
}

// runCommand executes an HWI command and returns the output
func (h *HWIClient) runCommand(args ...string) (string, error) {
	return h.runCommandInput("", args...)
}

// runCommandInput executes HWI with stdin (used for PIN entry, which must
// never appear in the process list).
func (h *HWIClient) runCommandInput(stdin string, args ...string) (string, error) {
	cmdArgs := make([]string, 0, len(args)+2)

	// Only pass --chain for device-specific commands (HWI 2.x).
	// enumerate and --version don't accept it.
	if h.testnet && len(args) > 0 && args[0] != "enumerate" && args[0] != "--version" {
		cmdArgs = append(cmdArgs, "--chain", "test")
	}
	cmdArgs = append(cmdArgs, args...)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// #nosec G204 -- hwiPath is the resolved HWI binary; args are passed as argv, not via a shell
	cmd := exec.CommandContext(ctx, h.hwiPath, cmdArgs...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("hwi timed out")
		}
		// Try to extract error from stderr or stdout
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", classifyHWIError(errMsg)
	}

	output := stdout.String()

	// Check if HWI returned a JSON error response (exit code 0 but device rejected)
	var hwiResp struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if json.Unmarshal([]byte(output), &hwiResp) == nil && hwiResp.Error != "" {
		return "", classifyHWIError(hwiResp.Error)
	}

	return output, nil
}

// TroubleshootingHints returns context-specific troubleshooting tips for the error
func (e *HWIError) TroubleshootingHints() []string {
	switch e.Code {
	case HWIErrNotInstalled:
		return []string{
			"macOS: brew install libusb pipx && pipx install hwi",
			"Linux: sudo apt install libusb-1.0-0-dev python3-dev pipx && pipx install hwi",
			"Restart your terminal after installing",
		}
	case HWIErrNoDevice:
		return DeviceConnectionHints()
	case HWIErrDeviceLocked:
		return []string{
			"Enter your PIN directly on the device",
			"If the device shows no prompt, unplug and reconnect it",
		}
	case HWIErrUserCancelled:
		return nil
	case HWIErrUnknownMultisigWallet:
		return []string{
			"Import the multisig wallet/config on the device first",
			"Coldcard: Settings > Multisig Wallets > Allow from PSBT",
			"Coldcard PSBT import uses a generic wallet name (for example, PSBT-2-of-3); rename it on device if needed",
			"Make sure PSBT includes all global xpubs and key origins",
			"Re-export PSBT and retry signing",
		}
	case HWIErrSigningFailed:
		return []string{
			"Ensure the device is unlocked and on the correct app",
			"For Ledger: open the Bitcoin app before signing",
			"Try unplugging and reconnecting the device",
		}
	case HWIErrDeviceError:
		return []string{
			"Try unplugging and reconnecting the device",
			"Restart the device and try again",
			"Close other wallet software (Sparrow, Ledger Live, etc.)",
		}
	default:
		return DeviceConnectionHints()
	}
}

// DeviceConnectionHints returns general troubleshooting tips for device connection issues
func DeviceConnectionHints() []string {
	return []string{
		"Check that the USB cable is securely connected",
		"Unlock the device and enter your PIN",
		"For Ledger: open the Bitcoin app on the device",
		"Close other wallet software (Sparrow, Ledger Live, etc.)",
		"Linux: you may need to configure udev rules for your device",
	}
}

// HWIInstallStep represents a single step in the HWI installation guide
type HWIInstallStep struct {
	Command     string // Terminal command to run
	Description string // What this step does
}

// HWIPlatformSteps returns platform-specific installation steps for the current OS
func HWIPlatformSteps() (platform string, steps []HWIInstallStep, note string) {
	return hwiPlatformStepsForOS(runtime.GOOS)
}

// hwiPlatformStepsForOS returns installation steps for the given OS (testable)
func hwiPlatformStepsForOS(goos string) (platform string, steps []HWIInstallStep, note string) {
	switch goos {
	case "darwin":
		return "macOS", []HWIInstallStep{
			{"brew install libusb", "Install USB device library"},
			{"brew install pipx", "Install Python package manager"},
			{"pipx ensurepath", "Add pipx to your PATH"},
			{"pipx install hwi", "Install Hardware Wallet Interface"},
		}, "Restart your terminal after step 3, then run step 4."
	case "linux":
		return "Linux", []HWIInstallStep{
			{"sudo apt install libusb-1.0-0-dev libudev-dev python3-dev pipx", "Install dependencies"},
			{"pipx ensurepath", "Add pipx to your PATH"},
			{"pipx install hwi", "Install Hardware Wallet Interface"},
			{"sudo hwi installudevrules", "Set up USB device permissions"},
		}, "On Fedora/Arch, replace apt with your package manager.\nRestart your terminal after step 2. Reboot after step 4."
	default:
		return goos, []HWIInstallStep{
			{"pip install hwi", "Install HWI via pip"},
		}, "See https://github.com/bitcoin-core/HWI for detailed instructions."
	}
}

// HWIInstallInstructions returns platform-specific instructions for installing HWI
func HWIInstallInstructions() string {
	platform, steps, note := HWIPlatformSteps()
	var b strings.Builder
	b.WriteString("Hardware Wallet Interface (HWI) is not installed.\n\n")
	b.WriteString(platform + ":\n")
	for i, step := range steps {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, step.Command)
	}
	if note != "" {
		b.WriteString("\n" + note + "\n")
	}
	b.WriteString("\nFor more info: https://github.com/bitcoin-core/HWI")
	return b.String()
}

// VerifyHWIInstallation checks if HWI is properly installed and returns its version
func VerifyHWIInstallation() (string, error) {
	client := NewHWIClient(false)
	if !client.IsAvailable() {
		return "", fmt.Errorf("hwi not found in PATH")
	}
	return client.GetVersion()
}

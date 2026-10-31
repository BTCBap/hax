package models

import (
	"fmt"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/BTCBap/hax/internal/wallet"
)

func setupKey(key string) tea.KeyPressMsg {
	switch key {
	case "enter":
		return keyCode(tea.KeyEnter)
	case "esc":
		return keyCode(tea.KeyEsc)
	case "tab":
		return keyCode(tea.KeyTab)
	case "up":
		return keyCode(tea.KeyUp)
	case "down":
		return keyCode(tea.KeyDown)
	case "right":
		return keyCode(tea.KeyRight)
	case "left":
		return keyCode(tea.KeyLeft)
	default:
		return keyRunes(key)
	}
}

func pressSetup(m WalletSetupModel, key string) WalletSetupModel {
	out, _ := m.Update(setupKey(key))
	return out.(WalletSetupModel)
}

func newProgressSetup(t *testing.T) WalletSetupModel {
	t.Helper()
	mock := NewMockWalletManager()
	mock.CreateWalletFn = func(string, string, int) (string, error) { return testMnemonic, nil }
	m := NewWalletSetupModel(mock, nil)
	m.width, m.height = 100, 40
	return m
}

// requireProgressStage checks the exported progress state and the exact text
// the progress box renders for the current step.
func requireProgressStage(t *testing.T, m WalletSetupModel, step, percent string, want float64) {
	t.Helper()
	if got := m.getStepProgress(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("progress = %v, want %v (step %v)", got, want, m.step)
	}
	view := stripANSI(m.renderProgressBar())
	if !strings.Contains(view, "Step "+step) {
		t.Fatalf("progress box missing %q:\n%s", "Step "+step, view)
	}
	if !strings.Contains(view, percent) {
		t.Fatalf("progress box missing percent field %q:\n%s", percent, view)
	}
}

func requireComplete(t *testing.T, m WalletSetupModel, step string) {
	t.Helper()
	if !m.flowComplete {
		t.Fatalf("flowComplete = false after final step %v", m.step)
	}
	if got := m.getStepProgress(); got != 1 {
		t.Fatalf("completed progress = %v, want 1", got)
	}
	view := stripANSI(m.renderProgressBar())
	if !strings.Contains(view, percentText(100)) {
		t.Fatalf("completed view missing 100%%:\n%s", view)
	}
	if !strings.Contains(view, "Step "+step) {
		t.Fatalf("completed view missing step %q:\n%s", step, view)
	}
}

func percentText(value int) string {
	return fmt.Sprintf("%3d%%", value)
}

func driveToKeySource(t *testing.T) WalletSetupModel {
	t.Helper()
	m := newProgressSetup(t)
	m = pressSetup(m, "enter") // network -> create/restore
	m = pressSetup(m, "enter") // create -> wallet type
	m = pressSetup(m, "enter") // single-sig -> name
	m = typeKeys(m, "wallet1")
	m = pressSetup(m, "enter") // name -> password
	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter") // password -> confirm
	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter") // confirm -> script type
	m = pressSetup(m, "enter") // native segwit -> key source
	if m.step != StepChooseKeySource {
		t.Fatalf("step = %v, want key source", m.step)
	}
	return m
}

func TestSoftwareWalletProgressFollowsSteps(t *testing.T) {
	m := newProgressSetup(t)
	if got := m.getStepProgress(); got != 0 {
		t.Fatalf("menu progress = %v, want 0", got)
	}

	m = pressSetup(m, "enter")
	m = pressSetup(m, "enter")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "1/10", percentText(0), 0)

	m = typeKeys(m, "wallet1")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "2/10", percentText(10), 0.1)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "3/10", percentText(20), 0.2)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "4/10", percentText(30), 0.3)

	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "5/10", percentText(40), 0.4)

	m = pressSetup(m, "enter") // generate seed -> passphrase
	requireProgressStage(t, m, "6/10", percentText(50), 0.5)

	m = pressSetup(m, "enter") // empty passphrase
	requireProgressStage(t, m, "7/10", percentText(60), 0.6)

	m = pressSetup(m, "enter") // confirm empty passphrase
	requireProgressStage(t, m, "8/10", percentText(70), 0.7)

	m = pressSetup(m, "enter") // 12 words -> show mnemonic
	requireProgressStage(t, m, "9/10", percentText(80), 0.8)

	m = pressSetup(m, "enter") // show -> confirm mnemonic
	requireProgressStage(t, m, "10/10", percentText(90), 0.9)
	if m.getStepProgress() >= 1 {
		t.Fatal("final step must stay below 100% before the wallet is saved")
	}

	words := strings.Fields(testMnemonic)
	for i, pos := range m.backupWords {
		m = typeKeys(m, words[pos])
		m = pressSetup(m, "enter")
		if m.err != nil {
			t.Fatalf("word %d rejected: %v", i, m.err)
		}
		if i < len(m.backupWords)-1 {
			want := (9 + float64(i+1)/float64(len(m.backupWords))) / 10
			requireProgressStage(t, m, "10/10", percentText(int(want*100+0.5)), want)
		}
	}
	requireComplete(t, m, "10/10")
}

func TestHardwareWalletProgressFollowsSteps(t *testing.T) {
	m := driveToKeySource(t)
	if m.step != StepChooseKeySource {
		t.Fatalf("step = %v, want key source", m.step)
	}
	// The shared prologue belongs to the create flow until the user picks a
	// hardware key source at this step.
	requireProgressStage(t, m, "5/10", percentText(40), 0.4)

	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // hardware wallet -> connect
	if m.step != StepHardwareWalletConnect {
		t.Fatalf("step = %v, want hardware connect", m.step)
	}
	requireProgressStage(t, m, "6/7", percentText(71), 5.0/7.0)

	out, _ := m.Update(HWIXpubMsg{Xpub: "tpub-mock", Fingerprint: "abcd1234"})
	m = out.(WalletSetupModel)
	if m.step != StepHardwareWalletReview {
		t.Fatalf("step = %v, want hardware review", m.step)
	}
	requireProgressStage(t, m, "7/7", percentText(86), 6.0/7.0)
	if m.getStepProgress() >= 1 {
		t.Fatal("review must be below 100% before the wallet is created")
	}

	m = pressSetup(m, "enter")
	requireComplete(t, m, "7/7")
}

func TestRestoreFromSeedProgressFollowsSteps(t *testing.T) {
	m := newProgressSetup(t)
	m = pressSetup(m, "enter") // network -> create/restore
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // restore -> method
	m = pressSetup(m, "enter") // from seed -> name
	requireProgressStage(t, m, "1/7", percentText(0), 0)

	m = typeKeys(m, "restored")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "2/7", percentText(14), 1.0/7.0)

	restoreMnemonic, err := wallet.GenerateMnemonic(wallet.Mnemonic12Words)
	if err != nil {
		t.Fatalf("generate mnemonic: %v", err)
	}
	m = typeKeys(m, restoreMnemonic)
	m = pressSetup(m, "enter")
	if m.err != nil {
		t.Fatalf("mnemonic rejected: %v", m.err)
	}
	requireProgressStage(t, m, "3/7", percentText(29), 2.0/7.0)

	m = pressSetup(m, "enter") // empty passphrase
	requireProgressStage(t, m, "4/7", percentText(43), 3.0/7.0)

	m = pressSetup(m, "enter") // confirm empty passphrase
	requireProgressStage(t, m, "5/7", percentText(57), 4.0/7.0)

	m = pressSetup(m, "enter") // native segwit -> password
	requireProgressStage(t, m, "6/7", percentText(71), 5.0/7.0)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "7/7", percentText(86), 6.0/7.0)
	if m.getStepProgress() >= 1 {
		t.Fatal("confirm password must be below 100% before restore finishes")
	}

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireComplete(t, m, "7/7")
}

func TestRestoreFromFileProgressFollowsSteps(t *testing.T) {
	m := newProgressSetup(t)
	m = pressSetup(m, "enter")
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter")
	m = pressSetup(m, "down")
	m = pressSetup(m, "down")
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // wallet file -> file list
	if m.step != StepRestoreChooseFile {
		t.Fatalf("step = %v, want restore choose file", m.step)
	}
	requireProgressStage(t, m, "1/2", percentText(0), 0)

	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "2/2", percentText(50), 0.5)
	if m.getStepProgress() >= 1 {
		t.Fatal("password step must be below 100% before unlock succeeds")
	}

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireComplete(t, m, "2/2")
}

func TestAutoLoginProgressIsOneStep(t *testing.T) {
	m := newProgressSetup(t)
	m.hasLastWallet = true
	m.selectedWallet = "test-wallet"
	m.step = StepRestoreFilePassword
	requireProgressStage(t, m, "1/1", percentText(0), 0)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireComplete(t, m, "1/1")
}

func createMultisigToCosigners(t *testing.T) WalletSetupModel {
	t.Helper()
	m := newProgressSetup(t)
	m = pressSetup(m, "enter") // network -> create/restore
	m = pressSetup(m, "enter") // create -> wallet type
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // multisig -> name
	requireProgressStage(t, m, "1/7", percentText(0), 0)
	m = typeKeys(m, "multisig-one")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "2/7", percentText(14), 1.0/7.0)
	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "3/7", percentText(29), 2.0/7.0)
	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter") // -> multisig config
	requireProgressStage(t, m, "4/7", percentText(43), 3.0/7.0)
	return m
}

func addCosignerFromSeed(t *testing.T, m WalletSetupModel, mnemonic string) WalletSetupModel {
	t.Helper()
	before := len(m.multisigCosigners)
	if m.multisigInputMode == 0 {
		m = pressSetup(m, "tab") // xpub -> seed mode
	}
	m = typeKeys(m, mnemonic)
	m = pressSetup(m, "enter")
	if m.err != nil {
		t.Fatalf("cosigner rejected: %v", m.err)
	}
	if len(m.multisigCosigners) != before+1 {
		t.Fatalf("cosigner not added: mode=%d step=%v", m.multisigInputMode, m.step)
	}
	return m
}

// generateCosignerMnemonic returns a valid mnemonic with no 'h', because the
// cosigner screen treats a lone "h" key as the hardware-wallet shortcut.
func generateCosignerMnemonic(t *testing.T) string {
	t.Helper()
	for {
		mnemonic, err := wallet.GenerateMnemonic(wallet.Mnemonic12Words)
		if err != nil {
			t.Fatalf("generate mnemonic: %v", err)
		}
		if !strings.Contains(mnemonic, "h") {
			return mnemonic
		}
	}
}

func TestMultisigCreateProgressFollowsSteps(t *testing.T) {
	m := createMultisigToCosigners(t)
	requireProgressStage(t, m, "4/7", percentText(43), 3.0/7.0)

	m = pressSetup(m, "enter") // config -> confirm
	requireProgressStage(t, m, "5/7", percentText(57), 4.0/7.0)

	m = pressSetup(m, "enter") // confirm -> cosigners
	requireProgressStage(t, m, "6/7", percentText(71), 5.0/7.0)

	mnemonics := make([]string, m.multisigN)
	for i := range mnemonics {
		mnemonics[i] = generateCosignerMnemonic(t)
	}

	m = addCosignerFromSeed(t, m, mnemonics[0])
	requireProgressStage(t, m, "6/7", percentText(76), (5+1.0/3.0)/7)

	m = addCosignerFromSeed(t, m, mnemonics[1])
	requireProgressStage(t, m, "6/7", percentText(81), (5+2.0/3.0)/7)

	m = addCosignerFromSeed(t, m, mnemonics[2])
	if m.step != StepMultisigReview {
		t.Fatalf("step = %v, want multisig review", m.step)
	}
	requireProgressStage(t, m, "7/7", percentText(86), 6.0/7.0)
	if m.getStepProgress() >= 1 {
		t.Fatal("review must be below 100% before the wallet is created")
	}

	m = pressSetup(m, "enter")
	requireComplete(t, m, "7/7")
}

func TestMultisigImportProgressFollowsSteps(t *testing.T) {
	m := newProgressSetup(t)
	m = pressSetup(m, "enter")
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // restore -> method
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // manual multisig -> name
	requireProgressStage(t, m, "1/7", percentText(0), 0)

	m = typeKeys(m, "multisig-two")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "2/7", percentText(14), 1.0/7.0)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "3/7", percentText(29), 2.0/7.0)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "4/7", percentText(43), 3.0/7.0)

	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "5/7", percentText(57), 4.0/7.0)

	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "6/7", percentText(71), 5.0/7.0)

	mnemonics := make([]string, m.multisigN)
	for i := range mnemonics {
		mnemonics[i] = generateCosignerMnemonic(t)
	}
	for i, mnemonic := range mnemonics {
		m = addCosignerFromSeed(t, m, mnemonic)
		if i < len(mnemonics)-1 {
			want := (5 + float64(i+1)/float64(m.multisigN)) / 7
			requireProgressStage(t, m, "6/7", percentText(int(want*100+0.5)), want)
		}
	}
	if m.step != StepMultisigReview {
		t.Fatalf("step = %v, want multisig review", m.step)
	}
	requireProgressStage(t, m, "7/7", percentText(86), 6.0/7.0)
	if m.getStepProgress() >= 1 {
		t.Fatal("review must be below 100% before the imported wallet is created")
	}

	m = pressSetup(m, "enter")
	requireComplete(t, m, "7/7")
}

func testDescriptor(t *testing.T) string {
	t.Helper()
	seed, err := wallet.MnemonicToSeed(
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "")
	if err != nil {
		t.Fatalf("mnemonic to seed: %v", err)
	}
	hd, err := wallet.NewHDWallet(seed, true)
	if err != nil {
		t.Fatalf("new hd wallet: %v", err)
	}
	xpub, err := hd.GetMultisigAccountXpub(wallet.BIP48Path{
		CoinType:   1,
		Account:    0,
		ScriptType: wallet.MultisigNativeSegwit,
	})
	if err != nil {
		t.Fatalf("derive xpub: %v", err)
	}
	return fmt.Sprintf("wsh(sortedmulti(2,[73c5da0a/48h/1h/0h/2h]%s,[a1b2c3d4/48h/1h/0h/2h]%s))", xpub, xpub)
}

func TestDescriptorImportProgressFollowsSteps(t *testing.T) {
	m := newProgressSetup(t)
	m = pressSetup(m, "enter")
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // restore -> method
	m = pressSetup(m, "down")
	m = pressSetup(m, "down")
	m = pressSetup(m, "enter") // descriptor import
	if m.step != StepMultisigImportDescriptor {
		t.Fatalf("step = %v, want descriptor input", m.step)
	}
	requireProgressStage(t, m, "1/4", percentText(0), 0)

	m.descriptorInput.SetValue(testDescriptor(t))
	m = pressSetup(m, "enter")
	if m.step != StepMultisigImportReview {
		t.Fatalf("step = %v, want descriptor review (err=%v)", m.step, m.err)
	}
	requireProgressStage(t, m, "2/4", percentText(25), 0.25)

	m = typeKeys(m, "descriptor-wallet")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "3/4", percentText(50), 0.5)

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireProgressStage(t, m, "4/4", percentText(75), 0.75)
	if m.getStepProgress() >= 1 {
		t.Fatal("confirm password must be below 100% before creation finishes")
	}

	m = typeKeys(m, "longenough")
	m = pressSetup(m, "enter")
	requireComplete(t, m, "4/4")
}

func TestProgressNeverGoesBackwardOnForwardKeys(t *testing.T) {
	m := newProgressSetup(t)
	last := 0.0
	forward := []string{"enter", "enter", "enter"}
	for _, key := range forward {
		m = pressSetup(m, key)
		got := m.getStepProgress()
		if got < last {
			t.Fatalf("progress went backward on %q: %v -> %v", key, last, got)
		}
		last = got
	}
	last = m.getStepProgress()
	for _, input := range []string{"wallet1", "longenough", "longenough"} {
		m = typeKeys(m, input)
		m = pressSetup(m, "enter")
		got := m.getStepProgress()
		if got < last {
			t.Fatalf("progress went backward after %q: %v -> %v", input, last, got)
		}
		last = got
	}
	m = pressSetup(m, "enter") // script type
	if got := m.getStepProgress(); got < last {
		t.Fatalf("progress went backward at script type: %v -> %v", last, got)
	}
}

// TestProgressBoxPositionsStayFixed walks every screen that shows a progress
// box and checks that the bar and the fixed-width percent field keep the same
// columns and that the box keeps the same line count. The hardware flow shares
// its prologue with the create flow until the user picks a hardware key source,
// so those shared steps are covered by "create software" and the hardware case
// covers the steps unique to the hardware list.
func TestProgressBoxPositionsStayFixed(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*WalletSetupModel)
		steps []WalletSetupStep
	}{
		{
			name:  "create software",
			setup: func(*WalletSetupModel) {},
			steps: []WalletSetupStep{
				StepEnterName, StepEnterPassword, StepConfirmPassword,
				StepChooseScriptType, StepChooseKeySource, StepEnterPassphrase,
				StepConfirmPassphrase, StepChooseWordCount, StepShowMnemonic,
				StepConfirmMnemonic,
			},
		},
		{
			name:  "create hardware",
			setup: func(*WalletSetupModel) {},
			steps: []WalletSetupStep{
				StepHWISetup, StepHardwareWalletConnect, StepHardwareWalletReview,
			},
		},
		{
			name:  "restore seed",
			setup: func(m *WalletSetupModel) { m.isRestore = true },
			steps: []WalletSetupStep{
				StepRestoreEnterName, StepRestoreEnterMnemonic,
				StepRestoreEnterPassphrase, StepRestoreConfirmPassphrase,
				StepRestoreChooseScriptType, StepRestoreEnterPassword,
				StepRestoreConfirmPassword,
			},
		},
		{
			name:  "restore file",
			setup: func(*WalletSetupModel) {},
			steps: []WalletSetupStep{StepRestoreChooseFile, StepRestoreFilePassword},
		},
		{
			name:  "auto login",
			setup: func(m *WalletSetupModel) { m.hasLastWallet = true },
			steps: []WalletSetupStep{StepRestoreFilePassword},
		},
		{
			name:  "multisig create",
			setup: func(m *WalletSetupModel) { m.isMultisig = true },
			steps: []WalletSetupStep{
				StepEnterName, StepEnterPassword, StepConfirmPassword,
				StepMultisigChooseType, StepMultisigConfirm,
				StepMultisigEnterCosigners, StepMultisigReview,
			},
		},
		{
			name: "multisig import",
			setup: func(m *WalletSetupModel) {
				m.isRestore = true
				m.isMultisig = true
			},
			steps: []WalletSetupStep{
				StepEnterName, StepEnterPassword, StepConfirmPassword,
				StepMultisigChooseType, StepMultisigConfirm,
				StepMultisigEnterCosigners, StepMultisigReview,
			},
		},
		{
			name: "descriptor import",
			setup: func(m *WalletSetupModel) {
				m.parsedDescriptor = &wallet.MultisigWalletConfig{}
			},
			steps: []WalletSetupStep{
				StepMultisigImportDescriptor, StepMultisigImportReview,
				StepEnterPassword, StepConfirmPassword,
			},
		},
		{
			name:  "completed",
			setup: func(m *WalletSetupModel) { m.flowComplete = true },
			steps: []WalletSetupStep{StepConfirmMnemonic},
		},
	}

	var wantBar, wantPercent viewPosition
	var wantLines int
	seen := false
	for _, tc := range cases {
		for _, step := range tc.steps {
			m := newProgressSetup(t)
			tc.setup(&m)
			m.step = step

			view := stripANSI(m.renderProgressBar())
			if view == "" {
				t.Fatalf("%s step %v has no progress box", tc.name, step)
			}
			progress := m.getStepProgress()
			glyph := "░"
			if progress > 0 {
				glyph = "█"
			}
			bar := requireTextPosition(t, view, glyph)
			percent := requireTextPosition(t, view, formatProgressPercent(progress))
			lines := requireLineCount(t, view)

			if !seen {
				wantBar, wantPercent, wantLines = bar, percent, lines
				seen = true
				continue
			}
			if bar != wantBar {
				t.Fatalf("%s step %v moved bar from %+v to %+v", tc.name, step, wantBar, bar)
			}
			if percent != wantPercent {
				t.Fatalf("%s step %v moved percent field from %+v to %+v", tc.name, step, wantPercent, percent)
			}
			if lines != wantLines {
				t.Fatalf("%s step %v has %d progress-box lines, want %d", tc.name, step, lines, wantLines)
			}
		}
	}
}

package models

import "fmt"

// walletSetupFlow identifies the ordered step list a setup screen belongs to.
// Each flow is a single list so the "Step N/M" indicator, the title counter,
// and the progress bar all read from the same source.
type walletSetupFlow int

const (
	flowCreateSoftware walletSetupFlow = iota
	flowCreateHardware
	flowRestoreSeed
	flowRestoreFile
	flowMultisigCreate
	flowMultisigImport
	flowDescriptorImport
)

// maxIncompleteProgress is the highest value shown while a flow is still
// running. 100% is reserved for the message that confirms the wallet is saved.
const maxIncompleteProgress = 0.99

// walletSetupProgressBarWidth is fixed so the bar and percent field keep their
// position as the percentage digits change.
const walletSetupProgressBarWidth = 50

// currentFlow picks the ordered step list for the screen that is showing.
// Menu screens that choose between flows are not part of a flow list.
func (m WalletSetupModel) currentFlow() walletSetupFlow {
	switch m.step {
	case StepMultisigImportDescriptor, StepMultisigImportReview:
		return flowDescriptorImport
	}
	if m.parsedDescriptor != nil || m.parsedSingleSig != nil {
		return flowDescriptorImport
	}
	// Auto-login starts directly on the file password step without setting
	// isRestore, so the step itself selects the restore-from-file flow.
	switch m.step {
	case StepRestoreChooseFile, StepRestoreFilePassword:
		return flowRestoreFile
	}
	if m.isRestore {
		if m.isMultisig {
			return flowMultisigImport
		}
		return flowRestoreSeed
	}
	if m.isMultisig {
		return flowMultisigCreate
	}
	switch m.step {
	case StepHWISetup, StepHardwareWalletConnect, StepHardwareWalletReview:
		return flowCreateHardware
	}
	return flowCreateSoftware
}

// flowSteps returns the ordered steps that make up the current flow. Only
// screens the user must pass through are listed; optional guide screens map
// onto the step they help complete.
func (m WalletSetupModel) flowSteps() []WalletSetupStep {
	switch m.currentFlow() {
	case flowCreateHardware:
		return []WalletSetupStep{
			StepEnterName,
			StepEnterPassword,
			StepConfirmPassword,
			StepChooseScriptType,
			StepChooseKeySource,
			StepHardwareWalletConnect,
			StepHardwareWalletReview,
		}
	case flowRestoreSeed:
		return []WalletSetupStep{
			StepRestoreEnterName,
			StepRestoreEnterMnemonic,
			StepRestoreEnterPassphrase,
			StepRestoreConfirmPassphrase,
			StepRestoreChooseScriptType,
			StepRestoreEnterPassword,
			StepRestoreConfirmPassword,
		}
	case flowRestoreFile:
		if m.hasLastWallet {
			return []WalletSetupStep{StepRestoreFilePassword}
		}
		return []WalletSetupStep{StepRestoreChooseFile, StepRestoreFilePassword}
	case flowMultisigCreate, flowMultisigImport:
		return []WalletSetupStep{
			StepEnterName,
			StepEnterPassword,
			StepConfirmPassword,
			StepMultisigChooseType,
			StepMultisigConfirm,
			StepMultisigEnterCosigners,
			StepMultisigReview,
		}
	case flowDescriptorImport:
		return []WalletSetupStep{
			StepMultisigImportDescriptor,
			StepMultisigImportReview,
			StepEnterPassword,
			StepConfirmPassword,
		}
	default:
		return []WalletSetupStep{
			StepEnterName,
			StepEnterPassword,
			StepConfirmPassword,
			StepChooseScriptType,
			StepChooseKeySource,
			StepEnterPassphrase,
			StepConfirmPassphrase,
			StepChooseWordCount,
			StepShowMnemonic,
			StepConfirmMnemonic,
		}
	}
}

// flowStepNumber returns the 1-based position of the current screen and the
// flow's total step count. The HWI guide screen is part of the connect step.
func (m WalletSetupModel) flowStepNumber() (step, total int, ok bool) {
	steps := m.flowSteps()
	for i, candidate := range steps {
		if m.step == candidate ||
			(candidate == StepHardwareWalletConnect && m.step == StepHWISetup) {
			return i + 1, len(steps), true
		}
	}
	return 0, 0, false
}

// stepCounterSuffix renders the same counter the progress box uses, for
// screen titles that include "[N/M]".
func (m WalletSetupModel) stepCounterSuffix() string {
	step, total, ok := m.flowStepNumber()
	if !ok {
		return ""
	}
	return fmt.Sprintf(" [%d/%d]", step, total)
}

// currentStepSubProgress reports completion of work inside the current step
// when the model knows the total. It does not count the step itself.
func (m WalletSetupModel) currentStepSubProgress() float64 {
	switch m.step {
	case StepConfirmMnemonic:
		if m.backupSkipConfirm || len(m.backupWords) == 0 {
			return 0
		}
		return clampFraction(float64(m.backupPos) / float64(len(m.backupWords)))
	case StepMultisigEnterCosigners:
		if m.multisigN <= 0 {
			return 0
		}
		return clampFraction(float64(len(m.multisigCosigners)) / float64(m.multisigN))
	}
	return 0
}

func clampFraction(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// rawFlowProgress is completed steps plus the known fraction of the current
// step, divided by the flow's total. It ignores the high-water mark.
func (m WalletSetupModel) rawFlowProgress() float64 {
	step, total, ok := m.flowStepNumber()
	if !ok || total == 0 {
		return 0
	}
	completed := float64(step - 1)
	return clampFraction((completed + m.currentStepSubProgress()) / float64(total))
}

// getStepProgress returns the bar percentage for the current screen. It stays
// below 100% while the final step is in progress. A high-water mark keeps
// forward navigation from moving backwards; going back with esc lowers it.
func (m WalletSetupModel) getStepProgress() float64 {
	if m.flowComplete {
		return 1
	}
	if _, _, ok := m.flowStepNumber(); !ok {
		return 0
	}
	progress := m.rawFlowProgress()
	if m.progressFlowKnown && m.progressFlow == m.currentFlow() && m.progressHigh > progress {
		progress = m.progressHigh
	}
	if progress > maxIncompleteProgress {
		return maxIncompleteProgress
	}
	return progress
}

// trackProgress records the high-water mark after an update. A smaller value
// is accepted when the user moved back a step or removed sub-progress.
func (m *WalletSetupModel) trackProgress(previousFlow walletSetupFlow, previousStep int) {
	if m.flowComplete {
		m.progressFlow = m.currentFlow()
		m.progressFlowKnown = true
		m.progressHigh = 1
		return
	}
	step, _, ok := m.flowStepNumber()
	if !ok {
		m.progressFlow = m.currentFlow()
		m.progressFlowKnown = false
		m.progressHigh = 0
		return
	}
	progress := m.rawFlowProgress()
	if !m.progressFlowKnown || m.progressFlow != previousFlow {
		m.progressFlow = m.currentFlow()
		m.progressFlowKnown = true
		m.progressHigh = progress
		return
	}
	if step <= previousStep && progress < m.progressHigh {
		m.progressHigh = progress
		return
	}
	if progress > m.progressHigh {
		m.progressHigh = progress
	}
}

// showProgressBar reports whether the current screen belongs to an ordered
// flow. Flow-choice menus do not show progress for a flow not chosen yet.
func (m WalletSetupModel) showProgressBar() bool {
	switch m.step {
	case StepChooseNetwork, StepChooseAction, StepChooseWalletType, StepRestoreChooseMethod:
		return false
	}
	_, _, ok := m.flowStepNumber()
	return ok
}

// formatProgressPercent renders a right-aligned four-column field:
// "  0%", " 42%", "100%".
func formatProgressPercent(progress float64) string {
	percent := int(clampFraction(progress)*100 + 0.5)
	return fmt.Sprintf("%3d%%", percent)
}

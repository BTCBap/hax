package models

import (
	"fmt"
	"strings"
	"testing"
)

type viewPosition struct {
	line int
	col  int
}

func requireTextPosition(t *testing.T, view, text string) viewPosition {
	t.Helper()
	lines := strings.Split(stripANSI(view), "\n")
	line, col, ok := findChoiceLine(lines, text)
	if !ok {
		t.Fatalf("%q not found in view:\n%s", text, stripANSI(view))
	}
	return viewPosition{line: line, col: col}
}

func requireLineCount(t *testing.T, view string) int {
	t.Helper()
	return len(strings.Split(stripANSI(view), "\n"))
}

// The backup word check asks for words with single- and double-digit numbers.
// Its title, progress, prompt, input and help must stay on the same rows and
// columns no matter which word is asked, whether an answer was typed, or
// whether the wrong-word error is shown.
func TestSeedQuizLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		pos   int
		input string
		err   error
	}{
		{name: "word 3 empty", pos: 0},
		{name: "word 3 typed", pos: 0, input: "zoo"},
		{name: "word 3 error", pos: 0, err: fmt.Errorf("that is not word #3; check your backup or press esc to see the seed again")},
		{name: "word 12 empty", pos: 1},
		{name: "word 12 typed", pos: 1, input: "zoo"},
		{name: "word 12 error", pos: 1, err: fmt.Errorf("that is not word #12; check your backup or press esc to see the seed again")},
	}

	var wantTitle, wantPrompt, wantProgress, wantInput viewPosition
	var wantLines int
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newBackupModel(t)
			m.backupWords = []int{2, 11} // word #3 and word #12
			m.backupPos = tc.pos
			m.backupInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			title := requireTextPosition(t, view, "Check your backup")
			progress := requireTextPosition(t, view, " of 2")
			prompt := requireTextPosition(t, view, "Enter word #")
			input := requireTextPosition(t, view, "> ")
			lines := requireLineCount(t, view)

			if progress.line != title.line {
				t.Fatalf("progress at %+v is not on the title line %+v", progress, title)
			}

			if i == 0 {
				wantTitle, wantProgress = title, progress
				wantPrompt, wantInput, wantLines = prompt, input, lines
				return
			}
			if title != wantTitle {
				t.Fatalf("title moved from %+v to %+v", wantTitle, title)
			}
			if progress != wantProgress {
				t.Fatalf("progress moved from %+v to %+v", wantProgress, progress)
			}
			if prompt != wantPrompt {
				t.Fatalf("prompt moved from %+v to %+v", wantPrompt, prompt)
			}
			if input != wantInput {
				t.Fatalf("input moved from %+v to %+v", wantInput, input)
			}
			if lines != wantLines {
				t.Fatalf("view has %d lines, want %d", lines, wantLines)
			}
		})
	}
}

func TestUnlockPasswordLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		input string
		err   error
	}{
		{name: "empty", input: ""},
		{name: "typed", input: "secret"},
		{name: "error", input: "", err: fmt.Errorf("incorrect password")},
		{name: "typed with error", input: "secret", err: fmt.Errorf("incorrect password")},
	}

	var wantTitle, wantPrompt, wantInput viewPosition
	var wantLines int
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewUnlockModel(NewMockWalletManager())
			m.width = 80
			m.height = 24
			m.walletList = []string{"my-wallet"}
			m.walletName = "my-wallet"
			m.passwordInput.Focus()
			m.passwordInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			title := requireTextPosition(t, view, "Unlock Wallet")
			prompt := requireTextPosition(t, view, "Password:")
			input := requireTextPosition(t, view, "> ")
			lines := requireLineCount(t, view)

			if i == 0 {
				wantTitle, wantPrompt = title, prompt
				wantInput, wantLines = input, lines
				return
			}
			if title != wantTitle {
				t.Fatalf("title moved from %+v to %+v", wantTitle, title)
			}
			if prompt != wantPrompt {
				t.Fatalf("password prompt moved from %+v to %+v", wantPrompt, prompt)
			}
			if input != wantInput {
				t.Fatalf("password input moved from %+v to %+v", wantInput, input)
			}
			if lines != wantLines {
				t.Fatalf("view has %d lines, want %d", lines, wantLines)
			}
		})
	}
}

// Moving through every input and output of the visualizer must not change
// the line count or move either header.
func TestTxFlowLayoutStable(t *testing.T) {
	v := TxFlowView{Flow: txFlowFixture(9, 4), Width: 80, Height: 24}
	base := v.View()
	wantLines := requireLineCount(t, base)
	wantInputs := requireTextPosition(t, base, "Inputs")
	wantOutputs := requireTextPosition(t, base, "Outputs")

	for _, side := range []TxFlowSide{FlowInput, FlowOutput} {
		v.SwitchSide(side)
		for i := 0; i < len(v.itemsFor(side)); i++ {
			view := v.View()
			if got := requireLineCount(t, view); got != wantLines {
				t.Fatalf("%s cursor %d: %d lines, want %d", sideName(side), i, got, wantLines)
			}
			if got := requireTextPosition(t, view, "Inputs"); got != wantInputs {
				t.Fatalf("%s cursor %d: Inputs header moved from %+v to %+v",
					sideName(side), i, wantInputs, got)
			}
			if got := requireTextPosition(t, view, "Outputs"); got != wantOutputs {
				t.Fatalf("%s cursor %d: Outputs header moved from %+v to %+v",
					sideName(side), i, wantOutputs, got)
			}

			v.MoveDown()
		}
	}
}

func TestSendAmountLayoutStable(t *testing.T) {
	cases := []struct {
		name  string
		input string
		err   error
	}{
		{name: "empty", input: ""},
		{name: "typed", input: "0.001"},
		{name: "error", input: "", err: fmt.Errorf("amount is too low")},
		{name: "typed with error", input: "0.001", err: fmt.Errorf("amount is too low")},
	}

	var wantHeader, wantPrompt viewPosition
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestSend(t)
			advanceToAmount(&m)
			m.width = 100
			m.height = 40
			m.amountInput.SetValue(tc.input)
			m.err = tc.err

			view := m.View()
			header := requireTextPosition(t, view, "Send Bitcoin")
			prompt := requireTextPosition(t, view, "Enter amount in BTC:")

			if i == 0 {
				wantHeader, wantPrompt = header, prompt
				return
			}
			if header != wantHeader {
				t.Fatalf("header moved from %+v to %+v", wantHeader, header)
			}
			if prompt != wantPrompt {
				t.Fatalf("amount prompt moved from %+v to %+v", wantPrompt, prompt)
			}
		})
	}
}

func TestConnectionRPCFormLayoutStable(t *testing.T) {
	cases := []string{"", "node.example.com", "a-very-long-host-name.example.onion"}
	var wantHost, wantUser viewPosition
	var wantLines int
	for i, host := range cases {
		m := NewConnectionSetupModel(nil, false)
		m.width = 80
		m.height = 40
		m.step = StepConfigureRPC
		m.hostInput.SetValue(host)

		view := m.View()
		hostLabel := requireTextPosition(t, view, "Host:")
		userLabel := requireTextPosition(t, view, "Username:")
		lines := requireLineCount(t, view)

		if i == 0 {
			wantHost, wantUser, wantLines = hostLabel, userLabel, lines
			continue
		}
		if hostLabel != wantHost {
			t.Fatalf("host field moved from %+v to %+v", wantHost, hostLabel)
		}
		if userLabel != wantUser {
			t.Fatalf("username field moved from %+v to %+v", wantUser, userLabel)
		}
		if lines != wantLines {
			t.Fatalf("view has %d lines, want %d", lines, wantLines)
		}
	}
}

// Every step the create flow can show must have a step number; a missing case
// renders "Step 0/0" and makes the progress bar jump backwards.
func TestCreateFlowProgressHasNoZeroStep(t *testing.T) {
	steps := []WalletSetupStep{
		StepChooseNetwork, StepChooseAction, StepChooseWalletType,
		StepEnterName, StepEnterPassword, StepConfirmPassword,
		StepChooseScriptType, StepChooseKeySource,
		StepEnterPassphrase, StepConfirmPassphrase, StepChooseWordCount,
		StepShowMnemonic, StepConfirmMnemonic,
		StepHWISetup, StepHardwareWalletConnect, StepHardwareWalletReview,
	}
	m := NewWalletSetupModel(NewMockWalletManager(), nil)
	m.width, m.height = 100, 40
	for _, step := range steps {
		m.step = step
		view := stripANSI(m.View())
		if strings.Contains(view, "Step 0/0") {
			t.Fatalf("step %d renders Step 0/0:\n%s", step, view)
		}
	}
}

package models

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeInto(m *SendModel, s string) {
	for _, r := range s {
		m.handleFeeSelection(keyRune(r))
	}
}

func customFeeModel(rate string) *SendModel {
	m := NewSendModel(NewMockWalletManager(), 100_000, nil)
	m.step = SendStepFee
	m.editingCustom = true
	m.customFeeInput.Focus()
	m.customFeeInput.SetValue(rate)
	return &m
}

func TestCustomFee_SoftCapNeedsTypedConfirmation(t *testing.T) {
	m := customFeeModel("750")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	if m.highFeePending != 750 || m.customFeeRate != 0 {
		t.Fatalf("pending=%v rate=%v", m.highFeePending, m.customFeeRate)
	}
	if v := m.View(); !strings.Contains(v, "unusually high") {
		t.Fatalf("confirmation prompt missing:\n%s", v)
	}
	// "y" alone is not enough.
	typeInto(m, "y")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	if m.customFeeRate != 0 || m.err == nil {
		t.Fatal("wrong confirmation accepted")
	}
	m.highFeeInput.SetValue("")
	typeInto(m, "High Fee")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	if m.customFeeRate != 750 || m.highFeePending != 0 || m.editingCustom {
		t.Fatalf("rate=%v pending=%v editing=%v", m.customFeeRate, m.highFeePending, m.editingCustom)
	}
}

func TestCustomFee_SoftCapEscReturnsToEditing(t *testing.T) {
	m := customFeeModel("900")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	m.handleFeeSelection(keyCode(tea.KeyEsc))
	if m.highFeePending != 0 || !m.editingCustom || m.customFeeRate != 0 {
		t.Fatal("esc should return to rate entry without applying")
	}
}

func TestCustomFee_BelowSoftCapAndAboveHardCap(t *testing.T) {
	m := customFeeModel("500")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	if m.customFeeRate != 500 || m.highFeePending != 0 {
		t.Fatal("500 sat/vB should apply without confirmation")
	}
	m = customFeeModel("10001")
	m.handleFeeSelection(keyCode(tea.KeyEnter))
	if m.customFeeRate != 0 || m.highFeePending != 0 || m.err == nil {
		t.Fatal("hard cap not enforced")
	}
}

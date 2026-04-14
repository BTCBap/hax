package wallet

import (
	"errors"
	"testing"
)

func TestSigningErrorHints(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		inputIdx int
		details  string
		wantHint string
	}{
		{
			name:     "ErrKeyMismatch",
			err:      ErrKeyMismatch,
			inputIdx: 0,
			details:  "pubkey mismatch",
			wantHint: "Key mismatch: Your wallet key doesn't match this PSBT. This transaction may be for a different wallet.",
		},
		{
			name:     "ErrNoLocalKey",
			err:      ErrNoLocalKey,
			inputIdx: -1,
			details:  "no mnemonic",
			wantHint: "No local signing key available. Import the cosigner seed phrase or use a hardware wallet to sign.",
		},
		{
			name:     "ErrAlreadySigned",
			err:      ErrAlreadySigned,
			inputIdx: 0,
			details:  "duplicate sig",
			wantHint: "Already signed: This input is already signed by your key. No additional signature needed.",
		},
		{
			name:     "ErrInvalidDerivationPath",
			err:      ErrInvalidDerivationPath,
			inputIdx: 1,
			details:  "path too short",
			wantHint: "Invalid derivation path in PSBT. The transaction may have been created with incompatible software.",
		},
		{
			name:     "ErrMissingWitnessUtxo",
			err:      ErrMissingWitnessUtxo,
			inputIdx: 2,
			details:  "nil UTXO",
			wantHint: "Missing UTXO data in PSBT. The PSBT may be incomplete or corrupted.",
		},
		{
			name:     "ErrEmptyPSBT",
			err:      ErrEmptyPSBT,
			inputIdx: -1,
			details:  "no inputs",
			wantHint: "Empty PSBT: No inputs to sign.",
		},
		{
			name:     "ErrWalletMismatch",
			err:      ErrWalletMismatch,
			inputIdx: -1,
			details:  "fingerprint mismatch",
			wantHint: "Wrong wallet: This PSBT was created for a different wallet. The master fingerprint doesn't match.",
		},
		{
			name:     "ErrScriptTypeMismatch",
			err:      ErrScriptTypeMismatch,
			inputIdx: -1,
			details:  "taproot vs segwit",
			wantHint: "Script type mismatch: This PSBT uses a different address type than your wallet (e.g., Taproot vs Native SegWit).",
		},
		{
			name:     "ErrWalletLocked",
			err:      ErrWalletLocked,
			inputIdx: -1,
			details:  "needs password",
			wantHint: "Wallet is locked. Enter your password to unlock before signing.",
		},
		{
			name:     "ErrMissingWitnessScript",
			err:      ErrMissingWitnessScript,
			inputIdx: 0,
			details:  "empty witness script",
			wantHint: "Missing witness script for multisig input. The PSBT may be incomplete — re-export from the coordinator.",
		},
		{
			name:     "ErrInsufficientSigs",
			err:      ErrInsufficientSigs,
			inputIdx: -1,
			details:  "have 1 need 2",
			wantHint: "Not enough signatures to broadcast. Collect more cosigner signatures before finalizing.",
		},
		{
			name:     "ErrSigningFailed",
			err:      ErrSigningFailed,
			inputIdx: 0,
			details:  "crypto error",
			wantHint: "Signing failed due to a cryptographic error. The PSBT data may be corrupted.",
		},
		{
			name:     "ErrDerivationFailed",
			err:      ErrDerivationFailed,
			inputIdx: 0,
			details:  "bad seed",
			wantHint: "Failed to derive signing key. The wallet seed may not match this PSBT.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := newSigningError(tt.err, tt.inputIdx, tt.details)

			// Error() should return the hint
			if se.Error() != tt.wantHint {
				t.Errorf("Error() = %q, want %q", se.Error(), tt.wantHint)
			}

			// Details should be preserved
			if se.Details != tt.details {
				t.Errorf("Details = %q, want %q", se.Details, tt.details)
			}

			// InputIdx should be preserved
			if se.InputIdx != tt.inputIdx {
				t.Errorf("InputIdx = %d, want %d", se.InputIdx, tt.inputIdx)
			}
		})
	}
}

func TestSigningErrorUnwrap(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantIs  error
		wantNot error
	}{
		{"KeyMismatch unwraps", ErrKeyMismatch, ErrKeyMismatch, ErrWalletMismatch},
		{"WalletLocked unwraps", ErrWalletLocked, ErrWalletLocked, ErrKeyMismatch},
		{"InsufficientSigs unwraps", ErrInsufficientSigs, ErrInsufficientSigs, ErrEmptyPSBT},
		{"SigningFailed unwraps", ErrSigningFailed, ErrSigningFailed, ErrAlreadySigned},
		{"DerivationFailed unwraps", ErrDerivationFailed, ErrDerivationFailed, ErrNoLocalKey},
		{"MissingWitnessScript unwraps", ErrMissingWitnessScript, ErrMissingWitnessScript, ErrMissingWitnessUtxo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := newSigningError(tt.err, 0, "test")

			if !errors.Is(se, tt.wantIs) {
				t.Errorf("errors.Is(se, %v) = false, want true", tt.wantIs)
			}
			if errors.Is(se, tt.wantNot) {
				t.Errorf("errors.Is(se, %v) = true, want false", tt.wantNot)
			}
		})
	}
}

func TestSigningErrorFallbackDisplay(t *testing.T) {
	// When no hint is set (unknown error type), Error() should fall back to
	// showing input index + error message
	se := &SigningError{
		Err:      errors.New("some unknown error"),
		InputIdx: 3,
		Details:  "details",
	}
	want := "input 3: some unknown error"
	if se.Error() != want {
		t.Errorf("Error() = %q, want %q", se.Error(), want)
	}

	// With InputIdx -1, should just show the error
	se2 := &SigningError{
		Err:      errors.New("unknown"),
		InputIdx: -1,
		Details:  "details",
	}
	if se2.Error() != "unknown" {
		t.Errorf("Error() = %q, want %q", se2.Error(), "unknown")
	}
}

package wallet

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// MinPasswordLength is the minimum length for a wallet encryption password.
const MinPasswordLength = 8

// MaxWalletNameLength bounds wallet names (they become file names).
const MaxWalletNameLength = 64

var walletNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]*$`)

// ValidateWalletName ensures a wallet name is safe to use as a file name inside
// the wallet directory: no path separators, no "..", no leading dot/space.
func ValidateWalletName(name string) error {
	if name == "" {
		return fmt.Errorf("wallet name cannot be empty")
	}
	if utf8.RuneCountInString(name) > MaxWalletNameLength {
		return fmt.Errorf("wallet name must be at most %d characters", MaxWalletNameLength)
	}
	if !walletNameRe.MatchString(name) {
		return fmt.Errorf("wallet name may only contain letters, numbers, spaces, '.', '_' and '-', and must start with a letter or number")
	}
	for i := 0; i+1 < len(name); i++ {
		if name[i] == '.' && name[i+1] == '.' {
			return fmt.Errorf("wallet name cannot contain '..'")
		}
	}
	return nil
}

// ValidatePassword enforces the minimum wallet password policy.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	return nil
}

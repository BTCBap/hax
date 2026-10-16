package wallet

// Wipe overwrites b with zeros. Use it for key material held in []byte.
//
// Go strings are immutable and may be copied by the runtime, so secrets that
// pass through a string (e.g. a mnemonic decoded from JSON) can't be reliably
// erased. Wipe is best-effort hygiene for the byte slices we own.
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

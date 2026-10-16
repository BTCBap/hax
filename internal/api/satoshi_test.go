package api

import "testing"

func TestParseBTC(t *testing.T) {
	ok := map[string]Satoshi{
		"0.29": 28_999_999 + 1, "0.00001": 1000, "1": 100_000_000, " 0.5 ": 50_000_000,
		".1": 10_000_000, "1.": 100_000_000, "0.00000001": 1, "21000000": MaxMoney,
	}
	for in, want := range ok {
		got, err := ParseBTC(in)
		if err != nil || got != want {
			t.Errorf("ParseBTC(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", ".", "0", "0.0", "-1", "1e-3", "NaN", "Inf", "0.000000001", "21000000.00000001", "1.2.3", "99999999999"} {
		if _, err := ParseBTC(in); err == nil {
			t.Errorf("ParseBTC(%q) succeeded, want error", in)
		}
	}
}

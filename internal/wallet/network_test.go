package wallet

import "testing"

func TestValidateAddressForNetwork_Strict(t *testing.T) {
	cases := []struct {
		addr, network string
		ok            bool
	}{
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", NetworkMainnet, true},
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", NetworkTestnet, false},
		{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", NetworkTestnet, true},
		{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", NetworkMainnet, false},
		{"1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", NetworkMainnet, true},
		{"1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", NetworkTestnet, false},
		{"mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn", NetworkTestnet, true},
		{"mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn", NetworkMainnet, false},
		{"bcrt1qw508d6qejxtdg4y5r3zarvary0c5xw7kygt080", NetworkTestnet, false},
		{"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "", false},
	}
	for _, c := range cases {
		if err := ValidateAddressForNetwork(c.addr, c.network); (err == nil) != c.ok {
			t.Errorf("%s on %q: err=%v want ok=%v", c.addr, c.network, err, c.ok)
		}
	}
}

func TestExplorerTxURL(t *testing.T) {
	if ExplorerTxURL(NetworkMainnet, "ab") != "https://mempool.space/tx/ab" ||
		ExplorerTxURL(NetworkTestnet, "ab") != "https://mempool.space/testnet4/tx/ab" {
		t.Fatal("bad explorer url")
	}
}

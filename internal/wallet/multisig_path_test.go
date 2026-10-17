package wallet

import "testing"

func TestMultisigAddressPathCoinType(t *testing.T) {
	for network, coin := range map[string]string{NetworkTestnet: "1'", NetworkMainnet: "0'"} {
		got := multisigAddressPath(network, MultisigNativeSegwit, 0, 5)
		if want := "m/48'/" + coin + "/0'/2'/0/5"; got != want {
			t.Errorf("%s: path %s, want %s", network, got, want)
		}
	}
}

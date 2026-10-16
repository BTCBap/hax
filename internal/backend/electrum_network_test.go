package backend

import "testing"

func TestAddressToScriptHash_RejectsForeignNetwork(t *testing.T) {
	b, err := NewElectrumBackend(DefaultElectrumConfig("testnet"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.addressToScriptHash("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"); err == nil {
		t.Error("mainnet address accepted by testnet backend")
	}
	if _, err := b.addressToScriptHash("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"); err != nil {
		t.Errorf("testnet address rejected: %v", err)
	}

	mb, _ := NewElectrumBackend(DefaultElectrumConfig("mainnet"))
	if _, err := mb.addressToScriptHash("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"); err == nil {
		t.Error("testnet address accepted by mainnet backend")
	}
}

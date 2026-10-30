package wallet

import "testing"

// TestTestNet4ParamsCanonical pins the values to Bitcoin Core's
// chainparams.cpp testnet4 definition (H3-47).
func TestTestNet4ParamsCanonical(t *testing.T) {
	p := &TestNet4Params
	if p.Name != "testnet4" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Net != 0x1C163F28 {
		t.Errorf("magic = %#x, want 0x1c163f28", p.Net)
	}
	if p.GenesisHash == nil ||
		p.GenesisHash.String() != "00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043" {
		t.Errorf("genesis = %v", p.GenesisHash)
	}
	if p.DefaultPort != "48333" {
		t.Errorf("DefaultPort = %q, want 48333", p.DefaultPort)
	}
	if p.Bech32HRPSegwit != "tb" || p.PubKeyHashAddrID != 0x6f || p.ScriptHashAddrID != 0xc4 {
		t.Errorf("address encoding = %q/%#x/%#x", p.Bech32HRPSegwit,
			p.PubKeyHashAddrID, p.ScriptHashAddrID)
	}
	if p.HDCoinType != 1 {
		t.Errorf("HDCoinType = %d, want 1", p.HDCoinType)
	}
	if p.PowLimitBits != 0x1d00ffff {
		t.Errorf("PowLimitBits = %#x", p.PowLimitBits)
	}
	if p.SubsidyReductionInterval != 210000 {
		t.Errorf("SubsidyReductionInterval = %d", p.SubsidyReductionInterval)
	}
	if p.TargetTimePerBlock != 600 || p.TargetTimespan != 1209600 {
		t.Errorf("timing = %d/%d", p.TargetTimePerBlock, p.TargetTimespan)
	}
	if len(p.Checkpoints) != 0 {
		t.Error("testnet4 has no checkpoint under this definition")
	}
}

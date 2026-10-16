package wallet

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestNet4Params defines the network parameters for Bitcoin Testnet4
var TestNet4Params = chaincfg.Params{
	Name: "testnet4",
	Net:  0x1C163F28, // Testnet4 magic bytes

	// Address encoding
	PubKeyHashAddrID: 0x6f, // Same as testnet3: starts with m or n
	ScriptHashAddrID: 0xc4, // Same as testnet3: starts with 2
	Bech32HRPSegwit:  "tb", // Same as testnet3: starts with tb1
	PrivateKeyID:     0xef, // Same as testnet3

	// BIP32 hierarchical deterministic extended keys
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // Same as testnet3: tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // Same as testnet3: tpub

	// BIP44 coin type
	HDCoinType: 1, // Testnet coin type

	// Genesis block
	GenesisHash: mustParseHash("00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043"),

	// Network configuration
	DefaultPort: "48333", // Testnet4 default port
	DNSSeeds: []chaincfg.DNSSeed{
		{Host: "seed.testnet4.bitcoin.sprovoost.nl", HasFiltering: true},
		{Host: "seed.testnet4.wiz.biz", HasFiltering: true},
	},

	// Checkpoints (to be filled as network matures)
	Checkpoints: []chaincfg.Checkpoint{},

	// Block target timespan and interval
	TargetTimespan:           14 * 24 * 60 * 60, // 2 weeks
	TargetTimePerBlock:       10 * 60,           // 10 minutes
	RetargetAdjustmentFactor: 4,                 // 25% less, 400% more

	// Difficulty
	ReduceMinDifficulty:  true,
	MinDiffReductionTime: 20 * 60, // 20 minutes
	GenerateSupported:    false,
	PowLimitBits:         0x1d00ffff,

	// Rule change deployment thresholds
	RuleChangeActivationThreshold: 1512, // 75% of MinerConfirmationWindow
	MinerConfirmationWindow:       2016,

	// Subsidy halving
	SubsidyReductionInterval: 210000,

	// Deployments - all features active on Testnet4
	Deployments: [chaincfg.DefinedDeployments]chaincfg.ConsensusDeployment{
		chaincfg.DeploymentTestDummy: {
			BitNumber: 28,
		},
		chaincfg.DeploymentCSV: {
			BitNumber: 0,
		},
		chaincfg.DeploymentSegwit: {
			BitNumber: 1,
		},
		chaincfg.DeploymentTaproot: {
			BitNumber: 2,
		},
	},
}

// mustParseHash is a helper to parse hex strings into chainhash.Hash
func mustParseHash(hashStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		panic(err)
	}
	return hash
}

func init() {
	// Register testnet4 params
	if err := chaincfg.Register(&TestNet4Params); err != nil {
		panic("failed to register testnet4 params: " + err.Error())
	}
}

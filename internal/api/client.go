package api

// Data types for Bitcoin blockchain data
// These types are shared between backends (Electrum, RPC) and UI layers

// AddressInfo contains information about a Bitcoin address
type AddressInfo struct {
	Address      string       `json:"address"`
	ChainStats   ChainStats   `json:"chain_stats"`
	MempoolStats MempoolStats `json:"mempool_stats"`
}

// ChainStats contains confirmed blockchain statistics
type ChainStats struct {
	FundedTxoCount int64   `json:"funded_txo_count"`
	FundedTxoSum   Satoshi `json:"funded_txo_sum"`
	SpentTxoCount  int64   `json:"spent_txo_count"`
	SpentTxoSum    Satoshi `json:"spent_txo_sum"`
	TxCount        int64   `json:"tx_count"`
}

// MempoolStats contains unconfirmed mempool statistics
type MempoolStats struct {
	FundedTxoCount int64   `json:"funded_txo_count"`
	FundedTxoSum   Satoshi `json:"funded_txo_sum"`
	SpentTxoCount  int64   `json:"spent_txo_count"`
	SpentTxoSum    Satoshi `json:"spent_txo_sum"`
	TxCount        int64   `json:"tx_count"`
}

// UTXO represents an unspent transaction output
type UTXO struct {
	TxID   string `json:"txid"`
	Vout   uint32 `json:"vout"`
	Value  Satoshi `json:"value"`
	Status Status  `json:"status"`
}

// Status represents the confirmation status of a transaction
type Status struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int64  `json:"block_height,omitempty"`
	BlockHash   string `json:"block_hash,omitempty"`
	BlockTime   int64  `json:"block_time,omitempty"`
}

// Transaction represents a Bitcoin transaction
type Transaction struct {
	TxID     string `json:"txid"`
	Version  int    `json:"version"`
	Locktime int64  `json:"locktime"`
	Size     int    `json:"size"`
	Weight   int    `json:"weight"`
	Fee      Satoshi `json:"fee"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
	Status   Status `json:"status"`
}

// Vin represents a transaction input
type Vin struct {
	TxID      string   `json:"txid"`
	Vout      uint32   `json:"vout"`
	Prevout   *Vout    `json:"prevout,omitempty"`
	Scriptsig string   `json:"scriptsig"`
	Witness   []string `json:"witness,omitempty"`
	Sequence  uint32   `json:"sequence"`
}

// Vout represents a transaction output
type Vout struct {
	ScriptpubkeyAsm     string `json:"scriptpubkey_asm"`
	ScriptpubkeyType    string `json:"scriptpubkey_type"`
	ScriptpubkeyAddress string `json:"scriptpubkey_address"`
	Value               Satoshi `json:"value"`
}

// FeeEstimates contains fee rate estimates in sat/vB for different confirmation targets.
// Values are float64 to support sub-1 sat/vB precision (e.g. 0.5 sat/vB on testnet4).
type FeeEstimates struct {
	FastestFee  float64 `json:"fastestFee"`  // Next block
	HalfHourFee float64 `json:"halfHourFee"` // ~30 minutes
	HourFee     float64 `json:"hourFee"`     // ~1 hour
	EconomyFee  float64 `json:"economyFee"`  // ~1 day
	MinimumFee  float64 `json:"minimumFee"`  // ~1 week
}

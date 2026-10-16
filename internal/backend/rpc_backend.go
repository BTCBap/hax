package backend

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BTCBap/hax/internal/api"

	"github.com/btcsuite/btcd/wire"
)

// RPCBackend implements BitcoinBackend using Bitcoin Core's JSON-RPC interface.
//
// Required Bitcoin Core configuration:
//   - -server=1 (enables RPC)
//   - -txindex=1 (enables getrawtransaction for any txid)
//
// Optional (recommended):
//   - -blockfilterindex=1 (enables scanblocks for address history detection)
//
// Without blockfilterindex, addresses that were used then fully spent
// may not be detected during gap limit scanning.
type RPCBackend struct {
	config    *BackendConfig
	client    *http.Client
	baseURL   string
	connected bool
	mu        sync.RWMutex
	requestID atomic.Uint64

	// Server info captured during connection
	version string // Bitcoin Core version string
	chain   string // Chain name (e.g., "testnet4", "main")
}

// rpcRequest is a JSON-RPC 2.0 request
type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// rpcResponse is a JSON-RPC 2.0 response
type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	ID     uint64          `json:"id"`
}

// rpcError represents a JSON-RPC error
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewRPCBackend creates a new Bitcoin Core RPC backend
func NewRPCBackend(config *BackendConfig) (*RPCBackend, error) {
	if config.Type != BackendTypeRPC {
		return nil, fmt.Errorf("invalid backend type for RPCBackend: %s", config.Type)
	}
	if config.RPC == nil {
		return nil, fmt.Errorf("RPC configuration is required")
	}

	return &RPCBackend{
		config: config,
	}, nil
}

// callRPC sends a JSON-RPC request to the Bitcoin Core node and returns the result.
func (b *RPCBackend) callRPC(method string, params interface{}) (json.RawMessage, error) {
	b.mu.RLock()
	client := b.client
	baseURL := b.baseURL
	b.mu.RUnlock()

	if client == nil || baseURL == "" {
		return nil, fmt.Errorf("not connected to Bitcoin Core")
	}

	if params == nil {
		params = []interface{}{}
	}

	id := b.requestID.Add(1)
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(b.config.RPC.Username, b.config.RPC.Password)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("RPC request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: check RPC username and password")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("access forbidden: check rpcallowip in bitcoin.conf")
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

// Connect establishes connection to the Bitcoin Core RPC server
func (b *RPCBackend) Connect() error {
	b.mu.Lock()
	if b.connected {
		b.mu.Unlock()
		return nil
	}

	scheme := "http"
	if b.config.RPC.UseSSL {
		scheme = "https"
	}
	b.baseURL = fmt.Sprintf("%s://%s:%d", scheme, b.config.RPC.Host, b.config.RPC.Port)

	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 10,
	}
	if b.config.RPC.UseSSL {
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	b.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	b.connected = true
	b.mu.Unlock()

	// Verify connectivity and capture server info
	result, err := b.callRPC("getblockchaininfo", nil)
	if err != nil {
		b.mu.Lock()
		b.connected = false
		b.client = nil
		b.baseURL = ""
		b.mu.Unlock()
		return fmt.Errorf("failed to connect to Bitcoin Core: %w", err)
	}

	var info struct {
		Chain                string `json:"chain"`
		InitialBlockDownload bool   `json:"initialblockdownload"`
	}
	if err := json.Unmarshal(result, &info); err != nil {
		b.mu.Lock()
		b.connected = false
		b.client = nil
		b.baseURL = ""
		b.mu.Unlock()
		return fmt.Errorf("failed to parse blockchain info: %w", err)
	}

	// Get version info
	verResult, err := b.callRPC("getnetworkinfo", nil)
	var version string
	if err == nil {
		var netInfo struct {
			SubVersion string `json:"subversion"`
		}
		if json.Unmarshal(verResult, &netInfo) == nil {
			version = netInfo.SubVersion
		}
	}

	b.mu.Lock()
	b.chain = info.Chain
	b.version = version
	b.mu.Unlock()

	if info.InitialBlockDownload {
		debugLog("[RPC] Warning: node is still syncing (IBD)")
	}

	return nil
}

// Disconnect closes the connection to the Bitcoin Core RPC server
func (b *RPCBackend) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil {
		b.client.CloseIdleConnections()
	}
	b.connected = false
	b.client = nil
	b.baseURL = ""
	return nil
}

// IsConnected returns the connection status
func (b *RPCBackend) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// TestConnection verifies the Bitcoin Core RPC server is responsive
func (b *RPCBackend) TestConnection() error {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return err
		}
	}

	_, err := b.callRPC("getblockchaininfo", nil)
	return err
}

// Type returns the backend type
func (b *RPCBackend) Type() BackendType {
	return BackendTypeRPC
}

// ServerVersion returns the Bitcoin Core version string (e.g., "/Satoshi:27.0.0/")
func (b *RPCBackend) ServerVersion() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.version
}

// Chain returns the chain name (e.g., "testnet4", "main")
func (b *RPCBackend) Chain() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.chain
}

// ScripthashNotifications returns a nil channel; RPC does not support Electrum subscriptions.
func (b *RPCBackend) ScripthashNotifications() <-chan ScripthashNotification {
	return nil
}

// HeaderNotifications returns a nil channel; RPC does not support Electrum subscriptions.
func (b *RPCBackend) HeaderNotifications() <-chan HeaderNotification {
	return nil
}

// SubscribeHeaders is a no-op for RPC backend.
func (b *RPCBackend) SubscribeHeaders() (int64, error) {
	return 0, nil
}

// SubscribeAddress is a no-op for RPC backend.
func (b *RPCBackend) SubscribeAddress(_ string) (string, error) {
	return "", nil
}

// GetAddressHistory checks whether an address has any transaction history.
func (b *RPCBackend) GetAddressHistory(address string) (bool, error) {
	info, err := b.GetAddressInfo(address)
	if err != nil {
		return false, err
	}
	return info.ChainStats.TxCount > 0 || info.MempoolStats.TxCount > 0, nil
}

// AddressToScripthash returns empty string; RPC does not use Electrum scripthashes.
func (b *RPCBackend) AddressToScripthash(_ string) (string, error) {
	return "", nil
}

// ServerInfo returns version and chain name for display purposes.
func (b *RPCBackend) ServerInfo() (software, protocol string) {
	return b.ServerVersion(), b.Chain()
}

// CertFingerprint returns empty string; RPC does not use TLS certificate pinning.
func (b *RPCBackend) CertFingerprint() string {
	return ""
}

// GetBlockHeight retrieves the current block height
func (b *RPCBackend) GetBlockHeight() (int64, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return 0, err
		}
	}

	result, err := b.callRPC("getblockcount", nil)
	if err != nil {
		return 0, err
	}

	var height int64
	if err := json.Unmarshal(result, &height); err != nil {
		return 0, fmt.Errorf("failed to parse block height: %w", err)
	}

	return height, nil
}

// GetFeeEstimates retrieves fee rate estimates via Bitcoin Core estimatesmartfee.
// Failed targets are left unavailable; if every target fails an error is returned
// instead of silently inventing 1 sat/vB.
func (b *RPCBackend) GetFeeEstimates() (*api.FeeEstimates, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	// Target blocks: 1 (fastest), 3 (half hour), 6 (hour), 144 (economy), 504 (minimum)
	targets := []int{1, 3, 6, 144, 504}
	rates := make([]float64, len(targets))
	ok := make([]bool, len(targets))

	for i, target := range targets {
		result, err := b.callRPC("estimatesmartfee", []interface{}{target})
		if err != nil {
			continue
		}

		var estimate struct {
			FeeRate float64  `json:"feerate"` // BTC/kvB
			Errors  []string `json:"errors"`
		}
		if err := json.Unmarshal(result, &estimate); err != nil {
			continue
		}

		if len(estimate.Errors) > 0 || estimate.FeeRate <= 0 {
			continue
		}

		// Convert BTC/kvB to sat/vB: BTC/kvB * 1e8 / 1000 = sat/vB
		satPerVB := estimate.FeeRate * 1e8 / 1000
		if satPerVB <= 0 {
			continue
		}
		if satPerVB < 0.1 {
			satPerVB = 0.1
		}
		rates[i] = satPerVB
		ok[i] = true
	}

	return assembleFeeEstimates(rates, ok)
}

// BroadcastTransaction broadcasts a raw transaction to the network
func (b *RPCBackend) BroadcastTransaction(rawTxHex string) (string, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return "", err
		}
	}

	result, err := b.callRPC("sendrawtransaction", []interface{}{rawTxHex})
	if err != nil {
		return "", fmt.Errorf("broadcast failed: %w", err)
	}

	var txid string
	if err := json.Unmarshal(result, &txid); err != nil {
		return "", fmt.Errorf("failed to parse txid: %w", err)
	}

	return txid, nil
}

// GetTransaction retrieves detailed transaction information.
// Requires -txindex=1 on the Bitcoin Core node for non-wallet transactions.
func (b *RPCBackend) GetTransaction(txid string) (*api.Transaction, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	// getrawtransaction txid verbose=true
	result, err := b.callRPC("getrawtransaction", []interface{}{txid, true})
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %s: %w", txid, err)
	}

	return b.parseVerboseTransaction(result)
}

// parseVerboseTransaction maps Bitcoin Core's verbose transaction JSON to api.Transaction
func (b *RPCBackend) parseVerboseTransaction(data json.RawMessage) (*api.Transaction, error) {
	var raw struct {
		TxID     string   `json:"txid"`
		Version  int      `json:"version"`
		Size     int      `json:"size"`
		VSize    int      `json:"vsize"`
		Weight   int      `json:"weight"`
		Locktime int64    `json:"locktime"`
		Hex      string   `json:"hex"`
		Fee      *float64 `json:"fee,omitempty"` // Available with txindex in Core 25.0+
		Vin      []struct {
			TxID      string `json:"txid"`
			Vout      uint32 `json:"vout"`
			ScriptSig struct {
				Hex string `json:"hex"`
			} `json:"scriptSig"`
			TxInWitness []string `json:"txinwitness"`
			Sequence    uint32   `json:"sequence"`
			Coinbase    string   `json:"coinbase,omitempty"`
			Prevout     *struct {
				Value        float64 `json:"value"`
				ScriptPubKey struct {
					Address string `json:"address"`
					Type    string `json:"type"`
				} `json:"scriptPubKey"`
			} `json:"prevout,omitempty"` // Available in Core 25.0+
		} `json:"vin"`
		Vout []struct {
			Value        float64 `json:"value"`
			N            uint32  `json:"n"`
			ScriptPubKey struct {
				Hex     string `json:"hex"`
				Address string `json:"address"`
				Type    string `json:"type"`
			} `json:"scriptPubKey"`
		} `json:"vout"`
		BlockHash     string `json:"blockhash,omitempty"`
		Confirmations int64  `json:"confirmations,omitempty"`
		Time          int64  `json:"time,omitempty"`
		BlockTime     int64  `json:"blocktime,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse transaction: %w", err)
	}

	tx := &api.Transaction{
		TxID:     raw.TxID,
		Version:  raw.Version,
		Locktime: raw.Locktime,
		Size:     raw.Size,
		Weight:   raw.Weight,
		Status: api.Status{
			Confirmed: raw.Confirmations > 0,
			BlockHash: raw.BlockHash,
			BlockTime: raw.BlockTime,
		},
	}

	if raw.Fee != nil {
		tx.Fee = btcToSatoshi(*raw.Fee)
	}

	// Get block height from blockhash if confirmed
	if raw.BlockHash != "" && raw.Confirmations > 0 {
		if height, err := b.getBlockHeight(raw.BlockHash); err == nil {
			tx.Status.BlockHeight = height
		}
	}

	// If we have the raw hex but no size/weight, compute from hex
	if tx.Size == 0 && raw.Hex != "" {
		rawBytes, err := hex.DecodeString(raw.Hex)
		if err == nil {
			tx.Size = len(rawBytes)
			var msgTx wire.MsgTx
			if msgTx.Deserialize(bytes.NewReader(rawBytes)) == nil {
				tx.Weight = msgTx.SerializeSizeStripped()*3 + len(rawBytes)
			}
		}
	}

	// Convert inputs
	tx.Vin = make([]api.Vin, len(raw.Vin))
	for i, vin := range raw.Vin {
		tx.Vin[i] = api.Vin{
			TxID:      vin.TxID,
			Vout:      vin.Vout,
			Scriptsig: vin.ScriptSig.Hex,
			Witness:   vin.TxInWitness,
			Sequence:  vin.Sequence,
		}

		// Map prevout if available (Core 25.0+)
		if vin.Prevout != nil {
			tx.Vin[i].Prevout = &api.Vout{
				ScriptpubkeyType:    vin.Prevout.ScriptPubKey.Type,
				ScriptpubkeyAddress: vin.Prevout.ScriptPubKey.Address,
				Value:               btcToSatoshi(vin.Prevout.Value),
			}
		}
	}

	// Convert outputs
	tx.Vout = make([]api.Vout, len(raw.Vout))
	for i, vout := range raw.Vout {
		tx.Vout[i] = api.Vout{
			ScriptpubkeyType:    vout.ScriptPubKey.Type,
			ScriptpubkeyAddress: vout.ScriptPubKey.Address,
			Value:               btcToSatoshi(vout.Value),
		}
	}

	// If fee not provided by Core, calculate from inputs-outputs
	if tx.Fee == 0 && len(tx.Vin) > 0 && tx.Vin[0].TxID != "" {
		b.populateInputValuesAndFee(tx)
	}

	return tx, nil
}

// populateInputValuesAndFee fetches previous transactions to populate input values and calculate fee.
// Only needed when Core doesn't provide fee/prevout (versions before 25.0).
func (b *RPCBackend) populateInputValuesAndFee(tx *api.Transaction) {
	if len(tx.Vin) == 0 {
		return
	}
	// Skip coinbase
	if tx.Vin[0].TxID == "0000000000000000000000000000000000000000000000000000000000000000" {
		return
	}

	var totalInput api.Satoshi
	var totalOutput api.Satoshi
	txCache := make(map[string]*api.Transaction)

	for i := range tx.Vin {
		vin := &tx.Vin[i]
		if vin.Prevout != nil {
			totalInput += vin.Prevout.Value
			continue
		}

		prevTx, ok := txCache[vin.TxID]
		if !ok {
			var err error
			prevTx, err = b.GetTransaction(vin.TxID)
			if err != nil {
				continue
			}
			txCache[vin.TxID] = prevTx
		}

		if int(vin.Vout) < len(prevTx.Vout) {
			prevOut := &prevTx.Vout[vin.Vout]
			vin.Prevout = &api.Vout{
				ScriptpubkeyType:    prevOut.ScriptpubkeyType,
				ScriptpubkeyAddress: prevOut.ScriptpubkeyAddress,
				Value:               prevOut.Value,
			}
			totalInput += prevOut.Value
		}
	}

	for _, vout := range tx.Vout {
		totalOutput += vout.Value
	}

	if totalInput > 0 && tx.Fee == 0 {
		tx.Fee = totalInput - totalOutput
	}
}

// getBlockHeight returns the height for a given block hash
func (b *RPCBackend) getBlockHeight(blockHash string) (int64, error) {
	result, err := b.callRPC("getblockheader", []interface{}{blockHash})
	if err != nil {
		return 0, err
	}

	var header struct {
		Height int64 `json:"height"`
	}
	if err := json.Unmarshal(result, &header); err != nil {
		return 0, err
	}
	return header.Height, nil
}

// GetAddressUTXOs retrieves unspent transaction outputs for an address using scantxoutset
func (b *RPCBackend) GetAddressUTXOs(address string) ([]api.UTXO, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	descriptor := fmt.Sprintf("addr(%s)", address)
	result, err := b.callRPC("scantxoutset", []interface{}{"start", []interface{}{descriptor}})
	if err != nil {
		return nil, fmt.Errorf("scantxoutset failed for %s: %w", address, err)
	}

	var scan struct {
		Success  bool `json:"success"`
		Unspents []struct {
			TxID     string  `json:"txid"`
			Vout     uint32  `json:"vout"`
			Amount   float64 `json:"amount"`
			Coinbase bool    `json:"coinbase"`
			Height   int64   `json:"height"`
		} `json:"unspents"`
		TotalAmount float64 `json:"total_amount"`
	}
	if err := json.Unmarshal(result, &scan); err != nil {
		return nil, fmt.Errorf("failed to parse scantxoutset result: %w", err)
	}

	utxos := make([]api.UTXO, len(scan.Unspents))
	for i, u := range scan.Unspents {
		utxos[i] = api.UTXO{
			TxID:  u.TxID,
			Vout:  u.Vout,
			Value: btcToSatoshi(u.Amount),
			Status: api.Status{
				Confirmed:   u.Height > 0,
				BlockHeight: u.Height,
			},
		}
	}

	return utxos, nil
}

// GetAddressBalance returns the balance for an address in satoshis
func (b *RPCBackend) GetAddressBalance(address string) (api.Satoshi, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return 0, err
		}
	}

	descriptor := fmt.Sprintf("addr(%s)", address)
	result, err := b.callRPC("scantxoutset", []interface{}{"start", []interface{}{descriptor}})
	if err != nil {
		return 0, fmt.Errorf("scantxoutset failed: %w", err)
	}

	var scan struct {
		TotalAmount float64 `json:"total_amount"`
	}
	if err := json.Unmarshal(result, &scan); err != nil {
		return 0, fmt.Errorf("failed to parse balance: %w", err)
	}

	return btcToSatoshi(scan.TotalAmount), nil
}

// GetAddressInfo returns information about an address including whether it has been used.
// Uses scantxoutset to check for current UTXOs, then falls back to scanblocks
// for historical usage detection (requires -blockfilterindex=1).
func (b *RPCBackend) GetAddressInfo(address string) (*api.AddressInfo, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	info := &api.AddressInfo{
		Address: address,
	}

	// Check for current UTXOs via scantxoutset
	descriptor := fmt.Sprintf("addr(%s)", address)
	result, err := b.callRPC("scantxoutset", []interface{}{"start", []interface{}{descriptor}})
	if err != nil {
		return nil, fmt.Errorf("scantxoutset failed: %w", err)
	}

	var scan struct {
		Success     bool    `json:"success"`
		TotalAmount float64 `json:"total_amount"`
		Unspents    []struct {
			TxID   string  `json:"txid"`
			Amount float64 `json:"amount"`
		} `json:"unspents"`
	}
	if err := json.Unmarshal(result, &scan); err != nil {
		return nil, fmt.Errorf("failed to parse scantxoutset result: %w", err)
	}

	if len(scan.Unspents) > 0 {
		// Address has current UTXOs — it's been used
		info.ChainStats.FundedTxoCount = int64(len(scan.Unspents))
		info.ChainStats.FundedTxoSum = btcToSatoshi(scan.TotalAmount)
		info.ChainStats.TxCount = int64(len(scan.Unspents))
		return info, nil
	}

	// No current UTXOs — check historical usage via scanblocks (if available)
	if used := b.checkAddressUsedViaScanblocks(address); used {
		info.ChainStats.TxCount = 1
		info.ChainStats.FundedTxoCount = 1
	}

	return info, nil
}

// checkAddressUsedViaScanblocks checks if an address has ever appeared in a block.
// Returns false if scanblocks is unavailable (no blockfilterindex).
func (b *RPCBackend) checkAddressUsedViaScanblocks(address string) bool {
	descriptor := fmt.Sprintf("addr(%s)", address)
	result, err := b.callRPC("scanblocks", []interface{}{"start", []interface{}{descriptor}})
	if err != nil {
		// scanblocks not available — can't determine historical usage
		return false
	}

	var scan struct {
		RelevantBlocks []string `json:"relevant_blocks"`
	}
	if err := json.Unmarshal(result, &scan); err != nil {
		return false
	}

	return len(scan.RelevantBlocks) > 0
}

// GetAddressTransactions retrieves transaction history for an address.
// Uses scanblocks to find relevant blocks, then extracts transactions.
// Falls back to UTXO-only transactions if blockfilterindex is not available.
func (b *RPCBackend) GetAddressTransactions(address string) ([]api.Transaction, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	// Try scanblocks to find all blocks containing this address
	descriptor := fmt.Sprintf("addr(%s)", address)
	result, err := b.callRPC("scanblocks", []interface{}{"start", []interface{}{descriptor}})
	if err == nil {
		return b.getTransactionsFromBlocks(result, address)
	}

	// Fallback: return transactions from current UTXOs only
	return b.getTransactionsFromUTXOs(address)
}

// getTransactionsFromBlocks extracts transactions involving the address from scanblocks results
func (b *RPCBackend) getTransactionsFromBlocks(scanResult json.RawMessage, address string) ([]api.Transaction, error) {
	var scan struct {
		RelevantBlocks []string `json:"relevant_blocks"`
	}
	if err := json.Unmarshal(scanResult, &scan); err != nil {
		return nil, fmt.Errorf("failed to parse scanblocks result: %w", err)
	}

	var transactions []api.Transaction
	seen := make(map[string]bool)

	for _, blockHash := range scan.RelevantBlocks {
		// getblock with verbosity=2 (decoded transactions)
		blockResult, err := b.callRPC("getblock", []interface{}{blockHash, 2})
		if err != nil {
			debugLog("[RPC] Failed to get block %s: %v", blockHash, err)
			continue
		}

		var block struct {
			Height int64             `json:"height"`
			Time   int64             `json:"time"`
			Hash   string            `json:"hash"`
			Tx     []json.RawMessage `json:"tx"`
		}
		if err := json.Unmarshal(blockResult, &block); err != nil {
			continue
		}

		for _, txData := range block.Tx {
			tx, err := b.parseVerboseTransaction(txData)
			if err != nil {
				continue
			}

			if seen[tx.TxID] {
				continue
			}

			// Check if transaction involves this address
			if txInvolvesAddress(tx, address) {
				tx.Status.Confirmed = true
				tx.Status.BlockHeight = block.Height
				tx.Status.BlockHash = block.Hash
				tx.Status.BlockTime = block.Time
				transactions = append(transactions, *tx)
				seen[tx.TxID] = true
			}
		}
	}

	return transactions, nil
}

// getTransactionsFromUTXOs returns transactions associated with current UTXOs.
// This is a fallback when scanblocks is not available.
func (b *RPCBackend) getTransactionsFromUTXOs(address string) ([]api.Transaction, error) {
	utxos, err := b.GetAddressUTXOs(address)
	if err != nil {
		return nil, err
	}

	var transactions []api.Transaction
	seen := make(map[string]bool)

	for _, utxo := range utxos {
		if seen[utxo.TxID] {
			continue
		}

		tx, err := b.GetTransaction(utxo.TxID)
		if err != nil {
			continue
		}

		transactions = append(transactions, *tx)
		seen[utxo.TxID] = true
	}

	return transactions, nil
}

// txInvolvesAddress checks if a transaction has any inputs or outputs involving the address
func txInvolvesAddress(tx *api.Transaction, address string) bool {
	for _, vout := range tx.Vout {
		if vout.ScriptpubkeyAddress == address {
			return true
		}
	}
	for _, vin := range tx.Vin {
		if vin.Prevout != nil && vin.Prevout.ScriptpubkeyAddress == address {
			return true
		}
	}

	return false
}

// btcToSatoshi converts a BTC float64 amount to Satoshi with proper rounding
func btcToSatoshi(btc float64) api.Satoshi {
	return api.Satoshi(math.Round(btc * 1e8))
}

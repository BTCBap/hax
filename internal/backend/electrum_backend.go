package backend

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BTCBap/hax/internal/api"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"golang.org/x/net/proxy"
)

// debugEnabled is set once at startup based on DEBUG env var
var debugEnabled = len(os.Getenv("DEBUG")) > 0

// debugLog logs only when DEBUG env var is set
func debugLog(format string, args ...interface{}) {
	if debugEnabled {
		log.Printf(format, args...)
	}
}

// ScripthashNotification is sent when a subscribed scripthash's history changes
type ScripthashNotification struct {
	Scripthash string
	Status     string // Status hash of the address history
}

// HeaderNotification is sent when a new block is found
type HeaderNotification struct {
	Height int64
	Hex    string
}

// electrumResponse is used to pass both result and error from readLoop to request
type electrumResponse struct {
	Result json.RawMessage
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
}

// ElectrumBackend implements BitcoinBackend using the Electrum protocol
type ElectrumBackend struct {
	config    *BackendConfig
	conn      net.Conn
	reader    *bufio.Reader
	connected bool
	mu        sync.RWMutex
	requestID uint64

	// Server info captured during handshake
	serverSoftware  string
	protocolVersion string
	certFingerprint string

	// Subscription support
	scripthashChan   chan ScripthashNotification
	headerChan       chan HeaderNotification
	pendingResponses map[uint64]chan electrumResponse
	pendingMu        sync.Mutex
	readerDone       chan struct{}
	subscriptions    map[string]bool // scripthash -> subscribed
	subMu            sync.RWMutex
}

// NewElectrumBackend creates a new Electrum backend
func NewElectrumBackend(config *BackendConfig) (*ElectrumBackend, error) {
	if config.Type != BackendTypeElectrum {
		return nil, fmt.Errorf("invalid backend type for ElectrumBackend: %s", config.Type)
	}

	if config.Electrum == nil {
		return nil, fmt.Errorf("Electrum configuration is required")
	}

	return &ElectrumBackend{
		config:           config,
		connected:        false,
		scripthashChan:   make(chan ScripthashNotification, 100),
		headerChan:       make(chan HeaderNotification, 10),
		pendingResponses: make(map[uint64]chan electrumResponse),
		subscriptions:    make(map[string]bool),
	}, nil
}

// ScripthashNotifications returns the channel for scripthash notifications
func (b *ElectrumBackend) ScripthashNotifications() <-chan ScripthashNotification {
	return b.scripthashChan
}

// HeaderNotifications returns the channel for header notifications
func (b *ElectrumBackend) HeaderNotifications() <-chan HeaderNotification {
	return b.headerChan
}

// Connect establishes connection to the Electrum server
func (b *ElectrumBackend) Connect() error {
	b.mu.Lock()

	if b.connected {
		b.mu.Unlock()
		return nil
	}

	// Bridge Tor → proxy settings (UI sets UseTor but Connect checks UseProxy)
	if b.config.Electrum.UseTor && !b.config.Electrum.UseProxy {
		b.config.Electrum.UseProxy = true
		b.config.Electrum.ProxyHost = "127.0.0.1"
		port, _ := strconv.Atoi(b.config.Electrum.TorSOCKS5Port)
		if port == 0 {
			port = 9050
		}
		b.config.Electrum.ProxyPort = port
	}

	addr := net.JoinHostPort(b.config.Electrum.Host, strconv.Itoa(b.config.Electrum.Port))

	var conn net.Conn
	var err error

	// Set connection timeout
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	// Use SOCKS5 proxy if configured (for Tor)
	if b.config.Electrum.UseProxy {
		proxyAddr := net.JoinHostPort(b.config.Electrum.ProxyHost, strconv.Itoa(b.config.Electrum.ProxyPort))
		socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, dialer)
		if err != nil {
			b.mu.Unlock()
			return fmt.Errorf("failed to create SOCKS5 proxy on %s: %w", proxyAddr, err)
		}

		conn, err = socksDialer.Dial("tcp", addr)
		if err != nil {
			b.mu.Unlock()
			return fmt.Errorf("failed to connect via proxy: %w", err)
		}

		// Wrap with TLS if using SSL
		if b.config.Electrum.UseSSL {
			tlsConn := tls.Client(conn, &tls.Config{
				ServerName:       b.config.Electrum.Host,
				VerifyConnection: verifyConnection(addr),
			})
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				b.mu.Unlock()
				return fmt.Errorf("TLS handshake failed: %w", err)
			}
			b.captureCertFingerprint(tlsConn)
			conn = tlsConn
		}
	} else if b.config.Electrum.UseSSL {
		// Direct TLS connection
		tlsConfig := &tls.Config{
			ServerName:       b.config.Electrum.Host,
			VerifyConnection: verifyConnection(addr),
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			b.mu.Unlock()
			return fmt.Errorf("failed to connect with TLS: %w", err)
		}
		if tlsConn, ok := conn.(*tls.Conn); ok {
			b.captureCertFingerprint(tlsConn)
		}
	} else {
		// Plain TCP connection
		conn, err = dialer.Dial("tcp", addr)
		if err != nil {
			b.mu.Unlock()
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Store connection and mark as connected
	b.conn = conn
	b.reader = bufio.NewReader(conn)
	b.connected = true
	b.readerDone = make(chan struct{})
	b.mu.Unlock()

	// Start background reader for subscriptions
	go b.readLoop()

	// Perform protocol handshake (outside lock - it uses its own read/write)
	if err := b.handshake(); err != nil {
		b.mu.Lock()
		if b.conn != nil {
			b.conn.Close()
		}
		b.conn = nil
		b.reader = nil
		b.connected = false
		if b.readerDone != nil {
			close(b.readerDone)
		}
		b.mu.Unlock()
		return fmt.Errorf("handshake failed: %w", err)
	}

	return nil
}

// readLoop runs in background reading all server messages
func (b *ElectrumBackend) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			debugLog("[ELECTRUM] readLoop panic recovered: %v", r)
		}
		b.mu.Lock()
		b.connected = false
		b.mu.Unlock()
	}()

	for {
		b.mu.RLock()
		if !b.connected || b.reader == nil {
			b.mu.RUnlock()
			return
		}
		reader := b.reader
		conn := b.conn
		b.mu.RUnlock()

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		line, err := reader.ReadBytes('\n')
		if err != nil {
			// Check if we're shutting down
			select {
			case <-b.readerDone:
				return
			default:
			}
			// Connection error - mark disconnected
			return
		}

		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			Result  json.RawMessage `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			ID *uint64 `json:"id"`
		}

		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.ID == nil {
			b.handleNotification(msg.Method, msg.Params)
		} else {
			b.pendingMu.Lock()
			if ch, ok := b.pendingResponses[*msg.ID]; ok {
				ch <- electrumResponse{
					Result: msg.Result,
					Error:  msg.Error,
				}
				delete(b.pendingResponses, *msg.ID)
			}
			b.pendingMu.Unlock()
		}
	}
}

// handleNotification processes incoming subscription notifications
func (b *ElectrumBackend) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "blockchain.scripthash.subscribe":
		// Params: [scripthash, status]
		var p []interface{}
		if err := json.Unmarshal(params, &p); err != nil || len(p) < 2 {
			return
		}
		scripthash, _ := p[0].(string)
		status, _ := p[1].(string)
		if len(scripthash) != 64 {
			debugLog("[ELECTRUM] Invalid scripthash notification (len=%d), skipping", len(scripthash))
			return
		}
		debugLog("[ELECTRUM] Scripthash notification: %s", scripthash[:16])
		select {
		case b.scripthashChan <- ScripthashNotification{Scripthash: scripthash, Status: status}:
			debugLog("[ELECTRUM] Sent scripthash notification to channel")
		default:
			debugLog("[ELECTRUM] Channel full, dropped scripthash notification")
		}

	case "blockchain.headers.subscribe":
		// Params: [{"height": N, "hex": "..."}]
		var p []struct {
			Height int64  `json:"height"`
			Hex    string `json:"hex"`
		}
		if err := json.Unmarshal(params, &p); err != nil || len(p) < 1 {
			return
		}
		debugLog("[ELECTRUM] Header notification: block %d", p[0].Height)
		select {
		case b.headerChan <- HeaderNotification{Height: p[0].Height, Hex: p[0].Hex}:
			debugLog("[ELECTRUM] Sent header notification to channel")
		default:
			debugLog("[ELECTRUM] Channel full, dropped header notification")
		}
	}
}

// Disconnect closes the connection to the Electrum server
func (b *ElectrumBackend) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Signal reader to stop
	if b.readerDone != nil {
		select {
		case <-b.readerDone:
			// Already closed
		default:
			close(b.readerDone)
		}
	}

	return b.disconnectLocked()
}

// disconnectLocked closes the connection when the lock is already held
func (b *ElectrumBackend) disconnectLocked() error {
	if !b.connected {
		return nil
	}

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
		b.reader = nil
	}

	b.connected = false
	return nil
}

// IsConnected returns the connection status
func (b *ElectrumBackend) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// TestConnection verifies the Electrum server is responsive
func (b *ElectrumBackend) TestConnection() error {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return err
		}
	}

	// Send server.ping request
	_, err := b.request("server.ping", []interface{}{})
	return err
}

// Type returns the backend type
func (b *ElectrumBackend) Type() BackendType {
	return BackendTypeElectrum
}

// handshake performs the Electrum protocol handshake
func (b *ElectrumBackend) handshake() error {
	// server.version "client_name" "protocol_version"
	result, err := b.request("server.version", []interface{}{"hax-wallet", "1.4"})
	if err != nil {
		return fmt.Errorf("version handshake failed: %w", err)
	}

	// Result should be ["server_software", "protocol_version"]
	var version []string
	if err := json.Unmarshal(result, &version); err != nil {
		return fmt.Errorf("failed to parse version response: %w", err)
	}

	if len(version) >= 2 {
		b.mu.Lock()
		b.serverSoftware = version[0]
		b.protocolVersion = version[1]
		b.mu.Unlock()
	}

	return nil
}

// captureCertFingerprint extracts the SHA256 fingerprint of the server's TLS certificate
func (b *ElectrumBackend) captureCertFingerprint(conn *tls.Conn) {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		hash := sha256.Sum256(state.PeerCertificates[0].Raw)
		// Format as colon-separated hex pairs
		parts := make([]string, len(hash))
		for i, v := range hash {
			parts[i] = fmt.Sprintf("%02X", v)
		}
		b.certFingerprint = strings.Join(parts, ":")
	}
}

// ServerInfo returns the server software and protocol version from the handshake
func (b *ElectrumBackend) ServerInfo() (software, protocol string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.serverSoftware, b.protocolVersion
}

// CertFingerprint returns the SHA256 fingerprint of the server's TLS certificate
func (b *ElectrumBackend) CertFingerprint() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.certFingerprint
}

// request sends a JSON-RPC request to the Electrum server and waits for response
func (b *ElectrumBackend) request(method string, params []interface{}) (json.RawMessage, error) {
	b.mu.Lock()
	if b.conn == nil || !b.connected {
		b.mu.Unlock()
		// Try to reconnect
		if err := b.Connect(); err != nil {
			return nil, fmt.Errorf("not connected and reconnect failed: %w", err)
		}
		b.mu.Lock()
	}

	id := b.requestID
	b.requestID++
	conn := b.conn
	b.mu.Unlock()

	// Create response channel
	respChan := make(chan electrumResponse, 1)

	// Register pending response
	b.pendingMu.Lock()
	b.pendingResponses[id] = respChan
	b.pendingMu.Unlock()

	// Clean up on return
	defer func() {
		b.pendingMu.Lock()
		delete(b.pendingResponses, id)
		b.pendingMu.Unlock()
	}()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      id,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Send request (newline-delimited JSON)
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		// Mark as disconnected so next request will reconnect
		b.mu.Lock()
		b.connected = false
		b.mu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("electrum error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(30 * time.Second):
		// Mark as disconnected so next request will reconnect
		b.mu.Lock()
		b.connected = false
		b.mu.Unlock()
		return nil, fmt.Errorf("request timeout - connection may be stale")
	}
}

// SubscribeHeaders subscribes to new block headers
// Returns the current tip on first call, subsequent updates come via HeaderNotifications()
func (b *ElectrumBackend) SubscribeHeaders() (int64, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return 0, err
		}
	}

	result, err := b.request("blockchain.headers.subscribe", []interface{}{})
	if err != nil {
		return 0, err
	}

	var header struct {
		Height int64  `json:"height"`
		Hex    string `json:"hex"`
	}

	if err := json.Unmarshal(result, &header); err != nil {
		return 0, fmt.Errorf("failed to parse header: %w", err)
	}

	return header.Height, nil
}

// SubscribeAddress subscribes to an address for transaction notifications
// Returns the current status hash (or null if no history)
func (b *ElectrumBackend) SubscribeAddress(address string) (string, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return "", err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return "", err
	}

	// Track subscription
	b.subMu.Lock()
	b.subscriptions[scriptHash] = true
	b.subMu.Unlock()

	result, err := b.request("blockchain.scripthash.subscribe", []interface{}{scriptHash})
	if err != nil {
		return "", err
	}

	// Result is the status hash (or null if no history)
	var status *string
	if err := json.Unmarshal(result, &status); err != nil {
		return "", fmt.Errorf("failed to parse status: %w", err)
	}

	if status == nil {
		return "", nil // No transaction history
	}
	return *status, nil
}

// GetAddressHistory returns whether an address has any transaction history
// This is used to determine if an address has been "used"
func (b *ElectrumBackend) GetAddressHistory(address string) (bool, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return false, err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return false, err
	}

	result, err := b.request("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return false, err
	}

	var history []interface{}
	if err := json.Unmarshal(result, &history); err != nil {
		return false, fmt.Errorf("failed to parse history: %w", err)
	}

	return len(history) > 0, nil
}

// ScripthashToAddress returns the address for a scripthash (for notification handling)
// Note: This requires maintaining a reverse mapping
func (b *ElectrumBackend) AddressToScripthash(address string) (string, error) {
	return b.addressToScriptHash(address)
}

// addressToScriptHash converts a Bitcoin address to Electrum script hash
// Electrum uses reversed SHA256 of the scriptPubKey
func (b *ElectrumBackend) addressToScriptHash(address string) (string, error) {
	// Determine network params
	var params *chaincfg.Params
	if b.config.Network == "testnet" {
		params = &chaincfg.TestNet3Params // btcutil uses TestNet3Params for testnet
	} else {
		params = &chaincfg.MainNetParams
	}

	// Decode the address
	addr, err := btcutil.DecodeAddress(address, params)
	if err != nil {
		return "", fmt.Errorf("failed to decode address: %w", err)
	}
	if !addr.IsForNet(params) {
		return "", fmt.Errorf("address %s is not valid for network %s", address, b.config.Network)
	}

	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return "", fmt.Errorf("failed to create script: %w", err)
	}

	hash := sha256.Sum256(script)

	// Electrum uses reversed byte order
	reversed := make([]byte, 32)
	for i := 0; i < 32; i++ {
		reversed[i] = hash[31-i]
	}

	return hex.EncodeToString(reversed), nil
}

// Blockchain data methods

// GetAddressInfo retrieves information about an address
func (b *ElectrumBackend) GetAddressInfo(address string) (*api.AddressInfo, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return nil, err
	}

	// Get balance
	balanceResult, err := b.request("blockchain.scripthash.get_balance", []interface{}{scriptHash})
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	var balance struct {
		Confirmed   int64 `json:"confirmed"`
		Unconfirmed int64 `json:"unconfirmed"`
	}
	if err := json.Unmarshal(balanceResult, &balance); err != nil {
		return nil, fmt.Errorf("failed to parse balance: %w", err)
	}

	// Get transaction history to count transactions
	historyResult, err := b.request("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	var history []struct {
		TxHash string `json:"tx_hash"`
		Height int64  `json:"height"`
	}
	if err := json.Unmarshal(historyResult, &history); err != nil {
		return nil, fmt.Errorf("failed to parse history: %w", err)
	}

	// Build AddressInfo (simplified - Electrum doesn't provide exact funded/spent counts)
	info := &api.AddressInfo{
		Address: address,
		ChainStats: api.ChainStats{
			FundedTxoSum: api.Satoshi(balance.Confirmed),
			TxCount:      int64(len(history)),
		},
		MempoolStats: api.MempoolStats{
			FundedTxoSum: api.Satoshi(balance.Unconfirmed),
		},
	}

	// If there's any history, mark as having funded txos
	if len(history) > 0 {
		info.ChainStats.FundedTxoCount = 1
	}

	return info, nil
}

// GetAddressBalance returns the balance for an address in satoshis
func (b *ElectrumBackend) GetAddressBalance(address string) (api.Satoshi, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return 0, err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return 0, err
	}

	result, err := b.request("blockchain.scripthash.get_balance", []interface{}{scriptHash})
	if err != nil {
		return 0, err
	}

	var balance struct {
		Confirmed   int64 `json:"confirmed"`
		Unconfirmed int64 `json:"unconfirmed"`
	}

	if err := json.Unmarshal(result, &balance); err != nil {
		return 0, fmt.Errorf("failed to parse balance: %w", err)
	}

	return api.Satoshi(balance.Confirmed + balance.Unconfirmed), nil
}

// GetAddressUTXOs retrieves all unspent transaction outputs for an address
func (b *ElectrumBackend) GetAddressUTXOs(address string) ([]api.UTXO, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return nil, err
	}

	result, err := b.request("blockchain.scripthash.listunspent", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	var electrumUTXOs []struct {
		TxHash string `json:"tx_hash"`
		TxPos  uint32 `json:"tx_pos"`
		Value  int64  `json:"value"`
		Height int64  `json:"height"`
	}

	if err := json.Unmarshal(result, &electrumUTXOs); err != nil {
		return nil, fmt.Errorf("failed to parse UTXOs: %w", err)
	}

	// Convert to api.UTXO format
	utxos := make([]api.UTXO, len(electrumUTXOs))
	for i, u := range electrumUTXOs {
		utxos[i] = api.UTXO{
			TxID:  u.TxHash,
			Vout:  u.TxPos,
			Value: api.Satoshi(u.Value),
			Status: api.Status{
				Confirmed:   u.Height > 0,
				BlockHeight: u.Height,
			},
		}
	}

	return utxos, nil
}

// GetAddressTransactions retrieves transaction history for an address
func (b *ElectrumBackend) GetAddressTransactions(address string) ([]api.Transaction, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	scriptHash, err := b.addressToScriptHash(address)
	if err != nil {
		return nil, err
	}

	// Get transaction history
	historyResult, err := b.request("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}

	var history []struct {
		TxHash string `json:"tx_hash"`
		Height int64  `json:"height"`
	}

	if err := json.Unmarshal(historyResult, &history); err != nil {
		return nil, fmt.Errorf("failed to parse history: %w", err)
	}

	// Fetch full transaction details for each
	transactions := make([]api.Transaction, 0, len(history))
	for _, h := range history {
		tx, err := b.GetTransaction(h.TxHash)
		if err != nil {
			// Skip transactions we can't fetch
			continue
		}

		// Set confirmation status from history response
		// Height <= 0 means unconfirmed (0 = mempool, -1 = unconfirmed with unconfirmed parents)
		tx.Status.Confirmed = h.Height > 0
		tx.Status.BlockHeight = h.Height

		// Get block timestamp if confirmed
		if h.Height > 0 {
			blockTime, err := b.getBlockTimestamp(h.Height)
			if err == nil {
				tx.Status.BlockTime = blockTime
			}
		}

		// Populate input values and calculate fee
		// This is done per-transaction to avoid slowing down initial load too much
		_ = b.populateInputValues(tx)

		transactions = append(transactions, *tx)
	}

	return transactions, nil
}

// getBlockTimestamp retrieves the timestamp for a block at given height
func (b *ElectrumBackend) getBlockTimestamp(height int64) (int64, error) {
	// Get block header at height
	result, err := b.request("blockchain.block.header", []interface{}{height})
	if err != nil {
		return 0, err
	}

	var headerHex string
	if err := json.Unmarshal(result, &headerHex); err != nil {
		return 0, err
	}

	// Decode header hex
	headerBytes, err := hex.DecodeString(headerHex)
	if err != nil {
		return 0, err
	}

	// Bitcoin block header is 80 bytes:
	// - Version: 4 bytes (little-endian)
	// - Previous block hash: 32 bytes
	// - Merkle root: 32 bytes
	// - Timestamp: 4 bytes (little-endian) at offset 68
	// - Bits: 4 bytes
	// - Nonce: 4 bytes
	if len(headerBytes) < 80 {
		return 0, fmt.Errorf("invalid header length: %d", len(headerBytes))
	}

	// Extract timestamp (bytes 68-71, little-endian uint32)
	timestamp := int64(headerBytes[68]) |
		int64(headerBytes[69])<<8 |
		int64(headerBytes[70])<<16 |
		int64(headerBytes[71])<<24

	return timestamp, nil
}

// GetTransaction retrieves details of a specific transaction
func (b *ElectrumBackend) GetTransaction(txid string) (*api.Transaction, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	// Get raw transaction hex (mempool-electrs doesn't support verbose mode)
	result, err := b.request("blockchain.transaction.get", []interface{}{txid})
	if err != nil {
		return nil, err
	}

	var rawHex string
	if err := json.Unmarshal(result, &rawHex); err != nil {
		return nil, fmt.Errorf("failed to parse transaction hex: %w", err)
	}

	rawBytes, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction hex: %w", err)
	}

	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(rawBytes)); err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}

	var params *chaincfg.Params
	if b.config.Network == "testnet" {
		params = &chaincfg.TestNet3Params
	} else {
		params = &chaincfg.MainNetParams
	}

	// Convert to api.Transaction format
	tx := &api.Transaction{
		TxID:     txid,
		Version:  int(msgTx.Version),
		Locktime: int64(msgTx.LockTime),
		Size:     len(rawBytes),
		Weight:   msgTx.SerializeSizeStripped()*3 + len(rawBytes), // Approximate weight
		Status: api.Status{
			Confirmed: false, // Default to unconfirmed - GetAddressTransactions sets this correctly
		},
	}

	// Convert inputs
	tx.Vin = make([]api.Vin, len(msgTx.TxIn))
	for i, txIn := range msgTx.TxIn {
		// Convert witness to string slice
		var witness []string
		for _, w := range txIn.Witness {
			witness = append(witness, hex.EncodeToString(w))
		}

		tx.Vin[i] = api.Vin{
			TxID:      txIn.PreviousOutPoint.Hash.String(),
			Vout:      txIn.PreviousOutPoint.Index,
			Scriptsig: hex.EncodeToString(txIn.SignatureScript),
			Witness:   witness,
			Sequence:  txIn.Sequence,
		}
	}

	// Convert outputs
	tx.Vout = make([]api.Vout, len(msgTx.TxOut))
	for i, txOut := range msgTx.TxOut {
		// Try to extract address from scriptPubKey
		_, addresses, _, _ := txscript.ExtractPkScriptAddrs(txOut.PkScript, params)
		var address string
		if len(addresses) > 0 {
			address = addresses[0].String()
		}

		// Determine script type
		scriptClass := txscript.GetScriptClass(txOut.PkScript)

		tx.Vout[i] = api.Vout{
			ScriptpubkeyType:    scriptClass.String(),
			ScriptpubkeyAddress: address,
			Value:               api.Satoshi(txOut.Value),
		}
	}

	return tx, nil
}

// GetTransactionWithFee retrieves transaction details including fee calculation
// This fetches input values from previous transactions to calculate the fee
func (b *ElectrumBackend) GetTransactionWithFee(txid string) (*api.Transaction, error) {
	tx, err := b.GetTransaction(txid)
	if err != nil {
		return nil, err
	}

	// Populate input values and calculate fee
	if err := b.populateInputValues(tx); err != nil {
		// If we can't get input values, still return the tx but without fee
		return tx, nil
	}

	return tx, nil
}

// populateInputValues fetches previous transactions to get input values and calculates fee
func (b *ElectrumBackend) populateInputValues(tx *api.Transaction) error {
	if len(tx.Vin) == 0 {
		return nil
	}

	// Skip coinbase transactions (no fee)
	if tx.Vin[0].TxID == "0000000000000000000000000000000000000000000000000000000000000000" {
		return nil
	}

	var totalInput api.Satoshi
	var totalOutput api.Satoshi

	// Cache previously fetched transactions to avoid duplicate fetches
	txCache := make(map[string]*api.Transaction)

	// Fetch input values from previous transactions
	for i := range tx.Vin {
		vin := &tx.Vin[i]

		// Check cache first
		prevTx, ok := txCache[vin.TxID]
		if !ok {
			// Fetch the previous transaction
			var err error
			prevTx, err = b.GetTransaction(vin.TxID)
			if err != nil {
				// Can't get previous tx, skip this input
				continue
			}
			txCache[vin.TxID] = prevTx
		}

		// Get the output being spent
		if int(vin.Vout) < len(prevTx.Vout) {
			prevOut := &prevTx.Vout[vin.Vout]
			// Set the Prevout on the input
			vin.Prevout = &api.Vout{
				ScriptpubkeyType:    prevOut.ScriptpubkeyType,
				ScriptpubkeyAddress: prevOut.ScriptpubkeyAddress,
				Value:               prevOut.Value,
			}
			totalInput += prevOut.Value
		}
	}

	// Calculate total outputs
	for _, vout := range tx.Vout {
		totalOutput += vout.Value
	}

	// Fee = inputs - outputs
	if totalInput > 0 {
		tx.Fee = totalInput - totalOutput
	}

	return nil
}

// BroadcastTransaction broadcasts a raw transaction to the network
func (b *ElectrumBackend) BroadcastTransaction(rawTxHex string) (string, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return "", err
		}
	}

	result, err := b.request("blockchain.transaction.broadcast", []interface{}{rawTxHex})
	if err != nil {
		return "", err
	}

	var txid string
	if err := json.Unmarshal(result, &txid); err != nil {
		return "", fmt.Errorf("failed to parse txid: %w", err)
	}

	return txid, nil
}

// GetFeeEstimates retrieves current fee rate estimates.
// Uses mempool.space REST API for precise sat/vB values, falls back to Electrum RPC.
func (b *ElectrumBackend) GetFeeEstimates() (*api.FeeEstimates, error) {
	fees, err := b.fetchMempoolPreciseFees()
	if err == nil {
		return fees, nil
	}
	debugLog("mempool.space precise fees unavailable, falling back to Electrum RPC: %v", err)

	return b.fetchElectrumFeeEstimates()
}

// fetchMempoolPreciseFees fetches fee estimates from mempool.space /api/v1/fees/precise.
// Returns precise sat/vB values including sub-1 rates.
func (b *ElectrumBackend) fetchMempoolPreciseFees() (*api.FeeEstimates, error) {
	var baseURL string
	if b.config.Network == "mainnet" {
		baseURL = "https://mempool.space/api/v1/fees/precise"
	} else {
		baseURL = "https://mempool.space/testnet4/api/v1/fees/precise"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var fees api.FeeEstimates
	if err := json.NewDecoder(resp.Body).Decode(&fees); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fees, nil
}

// fetchElectrumFeeEstimates uses Electrum RPC blockchain.estimatefee as a fallback.
func (b *ElectrumBackend) fetchElectrumFeeEstimates() (*api.FeeEstimates, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return nil, err
		}
	}

	targets := []int{1, 3, 6, 144, 504}
	fees := make([]float64, len(targets))

	for i, target := range targets {
		result, err := b.request("blockchain.estimatefee", []interface{}{target})
		if err != nil {
			fees[i] = 1
			continue
		}

		var feeRate float64
		if err := json.Unmarshal(result, &feeRate); err != nil {
			fees[i] = 1
			continue
		}

		if feeRate < 0 {
			fees[i] = 1
		} else {
			// Convert BTC/kB to sat/vB: BTC/kB * 1e8 / 1000
			fees[i] = feeRate * 100000
			if fees[i] < 0.1 {
				fees[i] = 0.1
			}
		}
	}

	return &api.FeeEstimates{
		FastestFee:  fees[0],
		HalfHourFee: fees[1],
		HourFee:     fees[2],
		EconomyFee:  fees[3],
		MinimumFee:  fees[4],
	}, nil
}

// GetBlockHeight retrieves the current block height
func (b *ElectrumBackend) GetBlockHeight() (int64, error) {
	if !b.IsConnected() {
		if err := b.Connect(); err != nil {
			return 0, err
		}
	}

	// blockchain.headers.subscribe returns the current tip
	result, err := b.request("blockchain.headers.subscribe", []interface{}{})
	if err != nil {
		return 0, err
	}

	var header struct {
		Height int64  `json:"height"`
		Hex    string `json:"hex"`
	}

	if err := json.Unmarshal(result, &header); err != nil {
		return 0, fmt.Errorf("failed to parse header: %w", err)
	}

	return header.Height, nil
}

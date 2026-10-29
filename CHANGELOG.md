# Changelog

This is a lightweight project changelog, not a strict release ledger.

## [Unreleased]

## [1.2.0] - 2026-10-28

### Changed
- Onboarding starts with the network choice (Mainnet preselected, Testnet4 optional); the wallet's network now decides the server, so a mainnet wallet never connects to a testnet server
- The first-run connection screen is gone; the server can still be changed later in Settings (servers listed are for the open wallet's network)
- Menus show the highlighted option's description on the same line, and screens reserve space for errors and hints so content no longer jumps

### Fixed
- First connection could fail with no way forward; hax now connects only to the server for the wallet's network (the one you saved, or that network's default), shows the error if it can't connect, and lets you choose another server for that network from the unlock screen (Ctrl+S on a network mismatch) or Settings
- Wallets whose saved server was for the other network now open on their own network's default server automatically
- The seed backup check no longer shifts as you type or when a wrong word is entered

## [1.1.0] - 2026-10-17

### Added
- Public repo baseline docs: README, SECURITY, CONTRIBUTING, CODE_OF_CONDUCT
- GitHub automation and templates for CI, issues, and pull requests
- Public beta checklist and compatibility tracking docs
- Taproot (P2TR) address support with BIP86 derivation
- Multisig wallet infrastructure (BIP48 derivation, P2SH/P2SH-P2WSH/P2WSH)
- Semantic versioning with build-time injection
- Bitcoin Core backend: live new-block alerts (polled)
- Send: BIP21 `bitcoin:` URIs fill address, amount and label; the review names the chosen fee priority
- Address labels (Addresses screen, BIP-329 export/import), and history search by address or address label
- Automatic reconnect with backoff and a "Reconnecting in Ns" status when the server drops; Electrum subscriptions are replayed and missed blocks or payments are reported
- Hardware wallet end-to-end check against a device emulator and a regtest node (`go test -tags emulator`)
- Speed up a pending transaction from History (`b`): replace it at a higher fee (RBF, BIP125 rules enforced) or spend our output with a child (CPFP) sized to the package feerate
- The Sign & Broadcast screen shows what the PSBT does before signing: each input and output, change proven by the wallet's own keys, fee and feerate, and warnings (inputs or outputs that aren't ours, mismatched amounts, non-default sighash, dust, high or too-low fee)
- Freeze coins from the UTXO screen (`f`): frozen coins stay in the encrypted wallet file, are never picked by coin selection, MAX or fee bumps, and travel in BIP-329 exports as `spendable: false`
- History warns when Bitcoin Core has no block filter index (`-blockfilterindex=1`) and can only show transactions behind unspent coins

### Changed
- Project naming now keeps the repo and module as `hax` while the binary remains `hax`
- Repository TODOs now distinguish public beta work from v1 production gates
- Public launch stance is source-first until release verification is in place

### Fixed
- Hardware wallet descriptor export ended at the account xpub; it now carries `/<0;1>/*`, so Core and Sparrow derive the wallet's receive and change addresses
- Bitcoin Core history missed transactions that spent a coin without paying the same address again
- On-device address verification no longer falls back to another connected device, and no longer reports a match when the device returns no address
- BIP-329 import skipped any line with a non-string value, such as Sparrow's output records
- Multisig address paths showed coin type `1'` on mainnet
- Hardware wallets can sign `hax` PSBTs: inputs now carry the previous transaction (legacy and SegWit v0), Taproot inputs and change carry the BIP371 key origin and internal key, and nested SegWit inputs and change carry their redeem script
- Signing an imported nested SegWit (P2SH-P2WPKH) PSBT produced an invalid signature
- The README release badge showed "invalid"
- A dropped connection mid-sync no longer shows a zero balance as synced; the last good data is kept and the error is shown
- Locking during the initial sync no longer hangs
- Scroll momentum carryover across screen and step transitions

## Pre-GitHub Testnet History (through 2026-01-30)

### Added
- Full HD wallet support (BIP32/39/44/49/84)
- Native SegWit (bech32) addresses by default
- Legacy (P2PKH) and Nested SegWit (P2SH-P2WPKH) support
- Electrum server connectivity with TLS and Tor support
- PSBT (BIP174) support for transaction construction
- Real-time balance and transaction updates via Electrum subscriptions
- Transaction history with sorting and search
- BIP329 label import/export
- Wallet descriptor export (Sparrow-compatible)
- Coin control (UTXO selection)
- Batch transactions (multiple recipients)
- Send All feature
- Custom fee rate support (including sub-1 sat/vB)
- Export formats: Koinly CSV, raw transaction hex
- QR code generation for receive addresses
- Mnemonic phrase backup with security confirmation
- Auto-login for most recent wallet

### Security
- AES-256-GCM encryption for wallet files
- PBKDF2-SHA256 with 100,000 iterations for key derivation
- BIP39 mnemonic phrase generation with cryptographic entropy
- Testnet only (mainnet disabled for safety)

### Technical
- Built with Bubbletea TUI framework (Elm architecture)
- Charmbracelet Lipgloss styling
- Background sync with instant cached display
- Testnet4 network support

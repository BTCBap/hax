# Changelog

This is a lightweight project changelog, not a strict release ledger.

## [Unreleased]

### Added
- Public repo baseline docs: README, SECURITY, CONTRIBUTING, CODE_OF_CONDUCT
- GitHub automation and templates for CI, issues, and pull requests
- Public beta checklist and compatibility tracking docs
- Taproot (P2TR) address support with BIP86 derivation
- Multisig wallet infrastructure (BIP48 derivation, P2SH/P2SH-P2WSH/P2WSH)
- Semantic versioning with build-time injection

### Changed
- Project naming now keeps the repo and module as `hax` while the binary remains `hax`
- Repository TODOs now distinguish public beta work from v1 production gates
- Public launch stance is source-first until release verification is in place

### Fixed
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

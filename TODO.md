# hax TODO

## Next

- [ ] Send flow: step breadcrumb bar
- [ ] Wallet setup: seed backup check (re-enter a few words) and password strength meter
- [ ] Dashboard: "last synced" age
- [ ] Layout pass at 80×24
- [ ] Split `send.go`, `wallet_setup.go`, and `settings.go` into smaller files

## Validation

- [ ] Unfunded smoke test in a clean environment: create, restore, unlock, receive, navigation, PSBT import/export
- [ ] Funded smoke test: send, sign, broadcast (testnet4 faucet or a local chain)
- [ ] Run the [recovery drill](docs/recovery-drill.md) and record the result
- [ ] Fill in the [hardware wallet and multisig matrix](docs/hardware-wallet-compatibility.md) with real-device results
- [ ] Test release binaries on macOS, Linux, and Windows

## Later

- [ ] Bitcoin Core backend: live block and transaction updates (RPC has no push notifications today)
- [ ] Broader hardware wallet and multisig interoperability
- [ ] More tests for disconnect and stale-state behavior
- [ ] Repository topics and starter issues on GitHub

## Someday

- [ ] Miniscript support, policy compiler, and Miniscript-aware PSBT analysis
- [ ] Taproot script-path multisig and MuSig2
- [ ] FROST threshold signatures
- [ ] Transaction flow visualizer
- [ ] Lightning, Ark, and other L2 integrations

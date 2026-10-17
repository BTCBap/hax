# hax TODO

## Validation

Manual checks; they need a person, real funds, or real devices.

- [ ] Unfunded smoke test in a clean environment: create, restore, unlock, receive, navigation, PSBT import/export
- [ ] Funded smoke test: send, sign, broadcast (testnet4 faucet or a local chain)
- [ ] Run the [recovery drill](docs/recovery-drill.md) and record the result
- [ ] Fill in the [hardware wallet and multisig matrix](docs/hardware-wallet-compatibility.md) with real-device results
- [ ] Test release binaries on macOS, Linux, and Windows

## Next

- [x] Connection resilience: clear status when the server drops, automatic reconnect with backoff, visible "reconnecting" state
- [x] Send: paste a `bitcoin:` (BIP21) URI to fill address, amount and label
- [x] Send: send max / sweep (already there: `m` on the amount step)
- [x] Send: clearer fee choice on review
- [x] Address and transaction labels, with history search and filter
- [x] Hardware signing: PSBTs carry previous transactions, BIP371 Taproot fields and nested SegWit redeem scripts (checked on a Trezor emulator)

## Later

- [x] Bitcoin Core backend: live block updates (polled; payments show on the next block or 30s sync)
- [ ] Broader hardware wallet and multisig interoperability (needs real devices and other wallets)
- [x] More tests for disconnect and stale-state behavior
- [ ] Repository topics and starter issues on GitHub (repo settings)

## Someday

- [ ] Miniscript support, policy compiler, and Miniscript-aware PSBT analysis
- [ ] Taproot script-path multisig and MuSig2
- [ ] FROST threshold signatures
- [ ] Transaction flow visualizer
- [ ] Lightning, Ark, and other L2 integrations

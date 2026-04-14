# hax TODO

## Ready For GitHub

- [x] Rename project, module, and binary to `hax`
- [x] Remove internal-only tracked repo files (`.agents`, `.claude`, local notes, compiled binary)
- [x] Add public repo baseline docs: README, LICENSE, SECURITY, CONTRIBUTING, CODE_OF_CONDUCT
- [x] Add GitHub baseline: CI workflow, issue templates, PR template
- [x] Add demo GIF
- [x] Add screenshot and social preview image
- [x] Verify the current publish branch builds and tests locally (`make build`, `make test`)

## Next

- [ ] Run an unfunded clean-environment smoke test for create, restore, unlock, receive, UI navigation, and PSBT import/export
- [ ] Run a funded clean-environment smoke test for send, sign, and broadcast using testnet4 faucet funds or a local chain
- [ ] Document a software-wallet recovery drill with a pass/fail checklist
- [ ] Fill in the hardware-wallet and multisig compatibility matrix with real validation results
- [ ] Add repository topics on GitHub
- [ ] Tag a few starter issues after the first push
- [ ] Turn on branch protection once the repo is live

## Later

- [ ] Dual-network support (mainnet + testnet4)
- [ ] Finish Bitcoin Core backend readiness beyond basic RPC connectivity
- [ ] Deterministic transaction safety checks before sign and broadcast
- [ ] Backup and recovery confidence for single-sig and multisig wallets
- [ ] More coverage for critical user flows and disconnect or stale-state behavior
- [ ] Broader hardware wallet and multisig interoperability validation
- [ ] Publish cross-compiled binaries for macOS, Linux, and Windows
- [ ] Publish SHA256 checksums
- [ ] Test release binaries on target platforms
- [ ] Additional mainnet safety rails before any real-money release

## Someday

- [ ] Lightning Network
- [ ] Spark L2
- [ ] Ark protocol
- [ ] Second
- [ ] Transaction flow visualizer
- [ ] Miniscript support
- [ ] Policy language compiler
- [ ] Miniscript-aware PSBT analysis
- [ ] MuSig2 aggregated signatures
- [ ] Taproot script-path multisig
- [ ] FROST threshold signatures

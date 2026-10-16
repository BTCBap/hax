# hax Threat Model

## Status

This document describes the current beta threat model for `hax`.

It is intentionally narrow and should be read together with the public-beta warning in the README.

## Scope

In scope:

- Local wallet file encryption and password-based unlock
- Seed generation, restore, and local storage flows
- Address derivation and transaction construction
- PSBT import, export, sign, finalize, and broadcast flows
- Hardware wallet interaction through HWI
- Network-specific correctness for testnet4

Out of scope for now:

- Mainnet operational safety
- Supply-chain hardening for release artifacts
- Protection against a fully compromised local machine
- Third-party firmware flaws in hardware wallets
- Strong anonymity guarantees

## Assets Worth Protecting

- Seed phrases and derived private keys
- Wallet passwords
- PSBT contents before signing
- Transaction outputs, fees, and change destinations
- Backup files and exported descriptors

## Trust Boundaries

- The local machine is only partially trusted. If the host is fully compromised, wallet safety is not guaranteed.
- External backends and APIs are not fully trusted and must not silently change wallet meaning.
- Hardware wallets are treated as separate trust domains and should confirm sensitive actions on-device when possible.
- Imported files and pasted PSBTs are untrusted input.

## Assumptions

- Users verify backups before depending on them.
- Users understand the current beta is testnet4-first.
- Hardware wallet firmware and HWI installations come from trusted sources.
- The operator is not running the app on a hostile or shared machine.

## Main Risks To Reduce Before v1

- Network mix-ups between testnet and future mainnet support
- Transaction-preview mistakes that hide abnormal fees or wrong change
- Recovery failures caused by incomplete backup or restore coverage
- Hardware-wallet interoperability gaps across device families
- Unsigned or unverifiable release artifacts

## Current Mitigations

- Explicit per-wallet network; recipient addresses strictly validated against it (no cross-network fallback)
- Active network shown in the UI, mainnet highlighted
- Encrypted wallet files at rest
- PSBT-based signing flow
- Fee estimates only from the configured Electrum or Core backend (no third-party HTTP fee APIs); failed estimates surface as unavailable rather than invented rates
- Existing automated tests across wallet, backend, and UI packages
- Public disclosure guidance in `SECURITY.md`

## Non-Goals

`hax` does not currently promise:

- Production-ready mainnet safety
- Formal verification
- Reproducible builds with signed attestations
- Resistance to malware on the local host

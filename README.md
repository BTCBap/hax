<p align="center">
  <img src="hax-logo.jpg" alt="hax" width="200">
</p>

<h1 align="center">hax</h1>

<p align="center">
  A terminal Bitcoin wallet: PSBT-first, hardware-wallet aware, and multisig capable, for mainnet and testnet4.
</p>

<p align="center">
  <a href="https://github.com/BTCBap/hax/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/BTCBap/hax/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/BTCBap/hax/releases"><img alt="Release" src="https://img.shields.io/github/v/release/BTCBap/hax"></a>
  <a href="https://pkg.go.dev/github.com/BTCBap/hax"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/BTCBap/hax.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

> [!WARNING]
> **hax has not been independently audited.** Try it on testnet4 first.

<p align="center">
  <img src="docs/images/hax-tour.gif" alt="Animated tour: splash, network pick, create or restore, seed backup and quiz, unlock, dashboard, receive, device verification, send steps and review, hardware wallet picker, transaction flow, PSBT signing, fees, history and speed-up, UTXOs, addresses, settings, server picker, multisig, label import, backup export, stealth mode and help" width="820">
</p>

| Dashboard | Receive |
|---|---|
| ![Dashboard with balance, recent activity, fees and menu](docs/images/dashboard.png) | ![Receive with address, QR code and derivation path](docs/images/receive.png) |
| **Send review** | **UTXOs** |
| ![Send review with recipient, amount, fee and change output](docs/images/send.png) | ![UTXOs with amounts, confirmations and a frozen coin](docs/images/utxos.png) |
| **History** | **Help** |
| ![History with confirmed transactions and labels](docs/images/history.png) | ![Help with keyboard shortcuts](docs/images/help.png) |
| **Transaction flow** | **Verify on device** |
| ![Transaction flow visualizer showing inputs, fee and outputs](docs/images/txflow.png) | ![Hardware wallet address verification matching the device](docs/images/verify.png) |

## Features

- **HD wallets:** BIP32/BIP39 with BIP44 legacy, BIP49 nested SegWit, BIP84 native SegWit, and BIP86 Taproot accounts, plus an optional BIP39 passphrase stored only inside the encrypted wallet file
- **PSBT (BIP174):** create, import, export, sign, finalize, and broadcast, including animated QR (UR / BBQr) for air-gapped signers
- **Hardware wallets:** Coldcard, Trezor, Ledger, BitBox02, Jade, and KeepKey through [HWI](https://github.com/bitcoin-core/HWI), with on-device address verification
- **Multisig:** BIP48 P2WSH, P2SH-P2WSH, and P2SH wallets with Sparrow-compatible descriptor import and export
- **Coin control:** Branch-and-Bound selection, frozen coins, send-all, batch payments, and dust and fee-anomaly warnings at review
- **Fee bumping:** RBF and CPFP for pending transactions, plus a PSBT inspector before every signature
- **Backends:** Electrum with TLS and optional Tor SOCKS5, or your own Bitcoin Core node over JSON-RPC (`-blockfilterindex=1` for full history)
- **Payments and labels:** BIP21 `bitcoin:` links fill the address, amount, and label; BIP-329 labels cover transactions and addresses, with searchable history and Koinly CSV export
- **Encrypted at rest:** AES-256-GCM with an Argon2id key (64 MiB, t=3, p=4); older PBKDF2 wallets migrate on unlock, and files are `0600` in a `0700` directory
- **Networks:** mainnet and testnet4, with addresses validated against the wallet's network

## Install

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/BTCBap/hax/main/install.sh | sh
```

The installer downloads the release, verifies its SHA-256 checksum, and installs `hax`. It installs to `/usr/local/bin` when available and may ask for your sudo password; pass `--no-sudo` to install to `~/.local/bin` instead.

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/BTCBap/hax/main/install.ps1 | iex
```

macOS app: download the macOS archive from [releases](https://github.com/BTCBap/hax/releases/latest), extract it, then right-click `Hax.app` and choose Open. Gatekeeper warns that the app is unsigned.

Windows archive: unzip it and double-click `hax.exe`; if SmartScreen appears, choose More info > Run anyway.

Linux packages: `sudo apt install ./hax_*_linux_amd64.deb` or `sudo dnf install ./hax_*_linux_amd64.rpm`.

Prefer to verify by hand? Download an archive from [releases](https://github.com/BTCBap/hax/releases/latest), then follow [docs/install.md](docs/install.md) for the checksum and cosign steps.

Install with Go: `go install github.com/BTCBap/hax/cmd/hax@latest`.

Run: type `hax`.

## Quick start

1. Run `hax` and pick a network and a backend: Electrum or Bitcoin Core RPC.
2. Create a wallet or restore one from a seed. Write the seed on paper and keep it offline.
3. Try testnet4 first. A faucet is at [mempool.space/testnet4/faucet](https://mempool.space/testnet4/faucet).

Usage, key bindings, and configuration are in [docs/usage.md](docs/usage.md).

## Documentation

- [Install details and release verification](docs/install.md)
- [Building from source](docs/building.md)
- [Usage, key bindings, and configuration](docs/usage.md)
- [Security model](docs/security-model.md)
- [Threat model](docs/threat-model.md)
- [Hardware wallet and multisig compatibility](docs/hardware-wallet-compatibility.md)
- [Recovery drill](docs/recovery-drill.md)
- [Release verification](docs/release-verification.md)
- [Smoke test](docs/smoke-test.md)
- [References](docs/references.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md). The audit status is in the warning above.

## License

[MIT](LICENSE)

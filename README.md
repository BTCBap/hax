<p align="center">
  <img src="hax-logo.jpg" alt="hax" width="200">
</p>

<h1 align="center">hax</h1>

<p align="center">
  A terminal Bitcoin wallet for <b>testnet4</b>: PSBT-first, hardware-wallet aware, multisig capable.
</p>

<p align="center">
  <a href="https://github.com/BTCBap/hax/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/BTCBap/hax/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/BTCBap/hax/releases"><img alt="Release" src="https://img.shields.io/github/v/release/BTCBap/hax?include_prereleases&sort=semver"></a>
  <a href="https://pkg.go.dev/github.com/BTCBap/hax"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/BTCBap/hax.svg"></a>
  <a href="https://goreportcard.com/report/github.com/BTCBap/hax"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/BTCBap/hax"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="Network: testnet4" src="https://img.shields.io/badge/network-testnet4-orange.svg">
</p>

> [!WARNING]
> **hax is beta software.** It supports mainnet and testnet4, and has not been independently audited. Every screen shows the active network; mainnet is flagged in red. Practice on testnet4 first, and only put on mainnet what you can afford to lose.

<p align="center">
  <img src="docs/images/hax-tour.gif" alt="hax tour" width="820">
</p>

## Features

- **HD wallets:** BIP32/BIP39 with BIP44 (legacy), BIP49 (nested SegWit), BIP84 (native SegWit), and BIP86 (Taproot) accounts
- **Optional BIP39 passphrase**, stored only inside the encrypted wallet file
- **PSBT (BIP174)** create, import, export, sign, finalize, and broadcast, including animated QR (UR / BBQr) for air-gapped signers
- **Hardware wallets** through [HWI](https://github.com/bitcoin-core/HWI): Coldcard, Trezor, Ledger, BitBox02, Jade, KeepKey. Includes on-device address verification
- **Multisig:** BIP48 P2WSH / P2SH-P2WSH / P2SH, with Sparrow-compatible descriptor import/export
- **Coin control:** Branch-and-Bound selection, send-all, batch payments, and dust and fee-anomaly warnings at review
- **Backends:** Electrum (TLS, optional Tor SOCKS5) or your own Bitcoin Core node over JSON-RPC
- **BIP-329 labels**, transaction history, and Koinly CSV export
- **Encrypted at rest:** AES-256-GCM with a PBKDF2-SHA256-derived key. Files are `0600` in a `0700` directory

| Dashboard | Receive |
|---|---|
| ![Dashboard](docs/images/dashboard.png) | ![Receive](docs/images/receive.png) |
| **Send review** | **UTXOs** |
| ![Send review](docs/images/send.png) | ![UTXOs](docs/images/utxos.png) |
| **History** | **Help** |
| ![History](docs/images/history.png) | ![Help](docs/images/help.png) |

## Security model

The full threat model is in [docs/threat-model.md](docs/threat-model.md).

| | |
|---|---|
| **Protects against** | Theft of wallet files at rest (needs the password); wrong-network addresses (strict decoding against the wallet's own network, mainnet or testnet4, no fallback); Electrum server impersonation after first connect (TLS public-key pinning); silent fee/change surprises (review-step warnings); tampered release artifacts (signed checksums + provenance) |
| **Does not protect against** | A compromised host (malware, keyloggers, memory scraping); a malicious Electrum server's view of your addresses (use your own node or Tor); weak passwords |
| **Trust boundaries** | Backends are untrusted for correctness of wallet meaning. Imported PSBTs and descriptors are untrusted input. Hardware wallets are a separate trust domain, so confirm on the device. |
| **Key material** | The seed is generated from `crypto/rand` (128 or 256 bits, BIP39 checksum) and never written to the clipboard or logs. The wallet password is not kept after unlock; only the derived AES key is held, as a byte slice that is zeroed on lock and exit. Signing happens locally or on the hardware device. |

Known gaps are tracked in [TODO.md](TODO.md) and the issue tracker. They include Go's limits on wiping immutable strings (the mnemonic is still held as a `string` while unlocked), idle auto-lock, and a memory-hard KDF.

## Install

### Release binaries (recommended)

Each release publishes archives for Linux, macOS, and Windows, plus `checksums.txt`, a Sigstore signature and certificate for it, an SPDX SBOM per archive, and a SLSA build-provenance attestation.

Verify before running:

```bash
VERSION=v0.1.0   # the release you downloaded
# 1. Verify the checksum manifest was signed by this repo's release workflow
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature   checksums.txt.sig \
  --certificate-identity "https://github.com/BTCBap/hax/.github/workflows/release.yml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Verify your archive against the manifest
sha256sum --ignore-missing -c checksums.txt     # macOS: shasum -a 256 -c checksums.txt

# 3. (optional) Verify build provenance
gh attestation verify hax_*_linux_amd64.tar.gz --repo BTCBap/hax
```

See [docs/release-verification.md](docs/release-verification.md) for details.

### `go install`

```bash
go install github.com/BTCBap/hax/cmd/hax@latest
```

### Build from source

Requires Go 1.26.9+ and a Unicode-capable terminal.

```bash
git clone https://github.com/BTCBap/hax.git
cd hax
make build          # ./hax
make test
make build-release  # -trimpath, version info injected
```

Releases are built with GoReleaser using `-trimpath`, an empty build ID, and commit-derived timestamps. Rebuilding a tag with the same Go version should produce identical archives.

## Usage

```bash
hax
```

On first run, hax asks you to pick a backend (Electrum or Bitcoin Core RPC). Then you can create or restore a wallet:

1. Create a wallet, write the seed phrase down on paper. Never store it digitally.
2. Choose an address type (native SegWit is the default).
3. Fund it from a testnet4 faucet, e.g. [mempool.space/testnet4/faucet](https://mempool.space/testnet4/faucet).
4. Use `r` to receive, `s` to send, `h` for history, `u` for UTXOs, `t` for settings, and `?` for all key bindings.

Set `DEBUG=1` to write a debug log. Never share debug logs publicly without reviewing them first.

## Configuration

All state lives in `~/.hax/`:

| Path | Contents |
|---|---|
| `~/.hax/<name>.wallet` | Encrypted wallet (seed, optional passphrase, accounts, labels, cache) |
| `~/.hax/known_servers.json` | Electrum TLS pins: SHA-256 of each server's public key, recorded on first connect (`0600`) |
| `~/.hax/config.json` | Backend selection, display unit, gap limit, recent servers |
| `~/.hax/*_labels_bip329.jsonl`, `*_koinly.csv` | Exports (`0600`) |

`config.json` is not encrypted. If you use Bitcoin Core RPC, your RPC credentials are stored there in plaintext (mode `0600`). Prefer a dedicated RPC user with minimal permissions.

## Networks

Each wallet records its network: mainnet or [testnet4](https://github.com/bitcoin/bips/blob/master/bip-0094.mediawiki) (BIP94). Testnet4 wallets carry a **TESTNET4** tag on the dashboard and on the send review screen. Mainnet wallets have no tag. A recipient address has to decode under that network's params. `tb1…` is rejected on a mainnet wallet and `bc1…` on a testnet4 wallet, and nothing falls back to another network. Creating a mainnet wallet asks for one extra confirmation.

## Electrum certificate pinning

On the first TLS connection to an Electrum server, hax verifies the certificate chain as usual and records the SHA-256 of the server's public key in `~/.hax/known_servers.json`. Later connections still verify the chain and also require that key to match. If the key changes, hax refuses to connect and shows both fingerprints. A changed key can be a legitimate rotation or an interception. If you have confirmed it out of band, delete that server's entry to re-pin.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow [Conventional Commits](https://www.conventionalcommits.org/). Please report security issues privately as described in [SECURITY.md](SECURITY.md), not in public issues.

## License

[MIT](LICENSE)

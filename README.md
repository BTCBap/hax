<p align="center">
  <img src="hax-logo.jpg" alt="hax" width="200">
</p>

<h1 align="center">hax</h1>

<p align="center">
  A terminal Bitcoin wallet: PSBT-first, hardware-wallet aware, multisig capable.<br>Supports mainnet and testnet4.
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

## Install

hax is a single program. The installer downloads the release archive, verifies its SHA-256 checksum, and installs `hax`. When the install directory is not on your PATH, it prints the lines to add.

### macOS

In Terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/BTCBap/hax/main/install.sh | sh
```

Then type `hax` and press Enter. The installer uses `/usr/local/bin` when it can, uses sudo for it when allowed, otherwise installs to `~/.local/bin`, and clears the macOS quarantine flag from the binary.

Alternatively, download the macOS archive for Apple Silicon or Intel from the [releases page](https://github.com/BTCBap/hax/releases/latest), double-click it to extract, then right-click `Hax.app`, choose Open, and choose Open again. macOS warns that the app is from an unidentified developer because it is unsigned. On newer macOS versions you can also allow it in System Settings > Privacy & Security > Open Anyway. After that, double-click `Hax.app` to start hax in Terminal. `Hax.app` is included starting with v1.5.0.

To read the install script before running it:

```bash
curl -fsSLO https://raw.githubusercontent.com/BTCBap/hax/main/install.sh
less install.sh
sh install.sh
```

### Linux

In a terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/BTCBap/hax/main/install.sh | sh
```

Then type `hax` and press Enter.

The installer uses `/usr/local/bin` if it is writable, uses sudo for `/usr/local/bin` when that is allowed, and otherwise installs to `~/.local/bin`. If it uses `~/.local/bin`, it prints the lines to add to bash, zsh, or fish, and you need to open a new terminal.

Useful options:

```bash
sh install.sh --no-sudo    # always install to ~/.local/bin
sh install.sh --desktop    # add a desktop entry (Linux)
sh install.sh --uninstall  # remove the binary and desktop entry
```

`--uninstall` never touches wallet data in `~/.hax`.

You can also download `install.sh` and run `sh install.sh`. Starting with v1.5.0, `.deb` and `.rpm` packages are on the releases page:

```bash
sudo apt install ./hax_*_linux_amd64.deb
sudo dnf install ./hax_*_linux_amd64.rpm
```

### Windows

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/BTCBap/hax/main/install.ps1 | iex
```

Then open a new terminal and type `hax`.

To read the script first, save it and run it:

```powershell
irm https://raw.githubusercontent.com/BTCBap/hax/main/install.ps1 -OutFile install.ps1
notepad install.ps1
powershell -ExecutionPolicy Bypass -File install.ps1
```

Or download `hax_<version>_windows_amd64.zip` from the releases page, unzip it, and double-click `hax.exe`. If SmartScreen appears, choose More info > Run anyway once. The PowerShell installer adds `%LOCALAPPDATA%\Programs\hax` to your user PATH and can create a Start Menu shortcut:

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1 -Shortcut
```

The Start Menu shortcut option is available from v1.5.0. To remove hax:

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1 -Uninstall
```

### Run it

Type:

```bash
hax
```

### Manual download

Release asset names follow `hax_<version>_<os>_<arch>`. The version in the file name does not include the leading `v`.

| System | Asset |
|---|---|
| macOS Apple Silicon | `hax_<version>_darwin_arm64.tar.gz` |
| macOS Intel | `hax_<version>_darwin_amd64.tar.gz` |
| Linux x86_64 | `hax_<version>_linux_amd64.tar.gz` |
| Linux ARM64 | `hax_<version>_linux_arm64.tar.gz` |
| Windows x86_64 | `hax_<version>_windows_amd64.zip` |

Each archive contains the hax binary (`hax`, or `hax.exe` on Windows). macOS archives also contain `Hax.app`.

### Verify a release download

Each release publishes archives for Linux, macOS, and Windows, plus `checksums.txt`, a Sigstore bundle for it, and a SLSA build-provenance attestation.

Verify before running:

```bash
VERSION=v1.4.0   # the release you downloaded
# 1. Verify the checksum manifest was signed by this repo's release workflow
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/BTCBap/hax/.github/workflows/release.yml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Verify your archive against the manifest
sha256sum --ignore-missing -c checksums.txt     # macOS: shasum -a 256 -c checksums.txt

# 3. (optional) Verify build provenance
gh attestation verify hax_*_linux_amd64.tar.gz --repo BTCBap/hax
```

### Other ways

Install with Go:

```bash
go install github.com/BTCBap/hax/cmd/hax@latest
```

Build from source. Requires Go 1.26.9+ and a Unicode-capable terminal.

```bash
git clone https://github.com/BTCBap/hax.git
cd hax
make build          # ./hax
make test
make build-release  # -trimpath, version info injected
```

Release builds are reproducible; see [docs/release-verification.md](docs/release-verification.md).

## Features

- **HD wallets:** BIP32/BIP39 with BIP44 (legacy), BIP49 (nested SegWit), BIP84 (native SegWit), and BIP86 (Taproot) accounts
- **Optional BIP39 passphrase**, stored only inside the encrypted wallet file
- **PSBT (BIP174)** create, import, export, sign, finalize, and broadcast, including animated QR (UR / BBQr) for air-gapped signers
- **Hardware wallets** through [HWI](https://github.com/bitcoin-core/HWI): Coldcard, Trezor, Ledger, BitBox02, Jade, KeepKey. Includes on-device address verification
- **Multisig:** BIP48 P2WSH / P2SH-P2WSH / P2SH, with Sparrow-compatible descriptor import/export
- **Coin control:** Branch-and-Bound selection, frozen coins, send-all, batch payments, and dust and fee-anomaly warnings at review
- **Fee bumping:** RBF and CPFP for pending transactions, and a PSBT inspector before every signature
- **Backends:** Electrum (TLS, optional Tor SOCKS5) or your own Bitcoin Core node over JSON-RPC (full history needs `-blockfilterindex=1`)
- **BIP21 payment URIs:** paste a `bitcoin:` link to fill the address, amount and label
- **BIP-329 labels** for transactions and addresses, searchable history, and Koinly CSV export
- **Encrypted at rest:** AES-256-GCM with an Argon2id-derived key (64 MiB, t=3, p=4). Wallets from older versions (PBKDF2) are migrated on unlock. Files are `0600` in a `0700` directory

| Dashboard | Receive |
|---|---|
| ![Dashboard with balance, recent activity, fees and menu](docs/images/dashboard.png) | ![Receive with address, QR code and derivation path](docs/images/receive.png) |
| **Send review** | **UTXOs** |
| ![Send review with recipient, amount, fee and change output](docs/images/send.png) | ![UTXOs with amounts, confirmations and a frozen coin](docs/images/utxos.png) |
| **History** | **Help** |
| ![History with confirmed transactions and labels](docs/images/history.png) | ![Help with keyboard shortcuts](docs/images/help.png) |
| **Transaction flow** | **Verify on device** |
| ![Transaction flow visualizer showing inputs, fee and outputs](docs/images/txflow.png) | ![Hardware wallet address verification matching the device](docs/images/verify.png) |

## Usage

```bash
hax
```

On first run, hax asks you to pick a backend (Electrum or Bitcoin Core RPC). Then you can create or restore a wallet:

1. Create a wallet, write the seed phrase down on paper. Never store it digitally.
2. Choose an address type (native SegWit is the default).
3. Pick mainnet or testnet4. For testnet4 coins, use a faucet such as [mempool.space/testnet4/faucet](https://mempool.space/testnet4/faucet).
4. Use `r` to receive, `s` to send, `h` for history, `u` for UTXOs, `t` for settings, and `?` for all key bindings.

Set `DEBUG=1` to write a debug log. Never share debug logs publicly without reviewing them first.

## Configuration

All state lives in `~/.hax/`:

| Path | Contents |
|---|---|
| `~/.hax/<name>.wallet` | Encrypted wallet (seed, optional passphrase, accounts, labels, cache) |
| `~/.hax/known_servers.json` | Trusted Electrum server keys |
| `~/.hax/config.json` | Backend selection, display unit, gap limit, recent servers |
| `~/.hax/*_labels_bip329.jsonl`, `*_koinly.csv` | Exports (`0600`) |

## Security

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)

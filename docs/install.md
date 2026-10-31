# Install

hax ships as a single program. The installers download the release archive, verify its SHA-256 checksum, and install the `hax` binary. On macOS and Linux, when the install directory is not on your `PATH`, the installer prints the lines to add.

For usage after installation, see [usage.md](usage.md). For build-from-source steps, see [building.md](building.md).

## macOS and Linux

```bash
curl -fsSL https://raw.githubusercontent.com/BTCBap/hax/main/install.sh | sh
```

Then type `hax` and press Enter.

The installer uses `/usr/local/bin` if it is writable. Otherwise it asks for your password with sudo to install there, so you may see a password prompt. If sudo or a terminal is unavailable, it installs to `~/.local/bin`. Pass `--no-sudo` to skip the password prompt and install to `~/.local/bin` when you have no administrator rights. On macOS it also clears the quarantine flag from the binary.

If the installer uses `~/.local/bin`, it prints the lines to add to bash, zsh, or fish, and you need to open a new terminal.

To read the install script before running it:

```bash
curl -fsSLO https://raw.githubusercontent.com/BTCBap/hax/main/install.sh
less install.sh
sh install.sh
```

### Options

```bash
sh install.sh --no-sudo    # always install to ~/.local/bin
sh install.sh --desktop    # add a desktop entry (Linux)
sh install.sh --uninstall  # remove the binary and desktop entry
sh install.sh --help       # show all options
```

`--uninstall` never touches wallet data in `~/.hax`.

`--desktop` writes `~/.local/share/applications/hax.desktop` on Linux. On macOS it prints a warning and skips the desktop entry.

### Environment variables

| Variable | Effect |
|---|---|
| `HAX_VERSION` | Release tag to install, for example `v1.5.0` |
| `HAX_INSTALL_DIR` | Directory to install hax into |
| `HAX_NO_SUDO=1` | Same as `--no-sudo`; always use `~/.local/bin` |
| `HAX_DESKTOP=1` | Same as `--desktop` |
| `HAX_SYSTEM_DIR` | System install directory (default `/usr/local/bin`; advanced) |

### macOS app

Starting with v1.5.0, the macOS archives include `Hax.app`. Download the archive for Apple Silicon or Intel from the [releases page](https://github.com/BTCBap/hax/releases/latest), double-click it to extract, then right-click `Hax.app`, choose Open, and choose Open again. macOS warns that the app is from an unidentified developer because it is unsigned. On newer macOS versions you can also allow it in System Settings > Privacy & Security > Open Anyway. After that, double-click `Hax.app` to start hax in Terminal.

### Debian and RPM packages

Starting with v1.5.0, `.deb` and `.rpm` packages are on the releases page:

```bash
sudo apt install ./hax_*_linux_amd64.deb
sudo dnf install ./hax_*_linux_amd64.rpm
```

## Windows

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

The installer writes `hax.exe` to `%LOCALAPPDATA%\Programs\hax`, adds that folder to your user `PATH`, and can create a Start Menu shortcut:

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1 -Shortcut
```

The Start Menu shortcut option is available from v1.5.0. `HAX_VERSION` selects a release tag. To remove hax:

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1 -Uninstall
```

Uninstall removes `hax.exe`, the Start Menu shortcut, and the PATH entry. Wallet data in `%USERPROFILE%\.hax` is not touched.

Alternatively, download `hax_<version>_windows_amd64.zip` from the releases page, unzip it, and double-click `hax.exe`. If SmartScreen appears, choose More info > Run anyway once.

## Manual download

Release asset names follow `hax_<version>_<os>_<arch>`. The version in the file name does not include the leading `v`.

| System | Asset |
|---|---|
| macOS Apple Silicon | `hax_<version>_darwin_arm64.tar.gz` |
| macOS Intel | `hax_<version>_darwin_amd64.tar.gz` |
| Linux x86_64 | `hax_<version>_linux_amd64.tar.gz` |
| Linux ARM64 | `hax_<version>_linux_arm64.tar.gz` |
| Windows x86_64 | `hax_<version>_windows_amd64.zip` |

Each archive contains the hax binary (`hax`, or `hax.exe` on Windows). macOS archives also contain `Hax.app`.

## Verify a release download

Each release publishes archives for Linux, macOS, and Windows, plus `checksums.txt`, a Sigstore bundle for it, and a SLSA build-provenance attestation. Verify before running:

```bash
VERSION=v1.5.0   # the release you downloaded
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

See [release-verification.md](release-verification.md) for the full procedure.

## Install with Go

```bash
go install github.com/BTCBap/hax/cmd/hax@latest
```

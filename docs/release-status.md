# Release Status

`hax` runs from source today.

There are no official GitHub release binaries yet.

Right now the project is:

- source-first
- mainnet and testnet4 supported (beta, unaudited)
- fine to build locally
- not a polished release package yet

## Run Locally From Source

Prerequisites:

- Go 1.25.4 or newer
- a terminal with Unicode support

Build and run:

```bash
git clone https://github.com/BTCBap/hax.git
cd hax
make build
./hax
```

Useful commands:

```bash
make test
make build-release
make build-all
make checksums
DEBUG=1 ./hax
```

Wallet data is stored in `~/.hax/`.

## Simple Summary

- `make build` works
- `make test` works
- `make build-all` exists for local cross-compiles
- tagged releases can come later
- funded send and broadcast validation still needs real testnet4 sats or a local chain

See [release-builds.md](release-builds.md) for the build and release workflow details.
See [release-verification.md](release-verification.md) for checksum verification steps.
See [public-beta-checklist.md](public-beta-checklist.md) and [recovery-drill.md](recovery-drill.md) for beta validation status.

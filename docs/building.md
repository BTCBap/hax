# Building from Source

Build hax from a checkout when you want to test a branch or inspect the code you run.

## Requirements

- Go 1.26.10 or newer. `go.mod` currently declares Go 1.26.10.
- `make`
- A Unicode-capable terminal

## Build

```bash
git clone https://github.com/BTCBap/hax.git
cd hax
make build          # ./hax
make test
make build-release  # -trimpath, version info injected
```

`make build` is a quick development build without version information. `make build-release` injects the version, commit, branch, and build date from the Makefile and Git.

## Make targets

| Target | Action |
|---|---|
| `make build` | Build `./hax` for local development |
| `make build-release` | Build with `-trimpath` and injected version information |
| `make build-all` | Cross-build Linux, macOS, and Windows binaries into `build/` |
| `make checksums` | Build all platforms and write local SHA-256 checksums |
| `make run` | Build and run |
| `make run-debug` | Build and run with `DEBUG=1` |
| `make test` | Run the test suite |
| `make test-race` | Run tests with the race detector |
| `make lint` | Run `golangci-lint` |
| `make fmt` | Format the code |
| `make tidy` | Tidy `go.mod` |
| `make version` | Print version information |
| `make clean` | Remove build artifacts |

`make checksums` produces unsigned local checksums. They are not the release checksums.

## Reproducible builds

Release builds are reproducible. The release workflow builds with `CGO_ENABLED=0`, `-trimpath`, `-buildid=`, and commit-derived timestamps. See [release-verification.md](release-verification.md) for the reproduction steps and verification commands.

For development setup, tests, and commit conventions, see [../CONTRIBUTING.md](../CONTRIBUTING.md).

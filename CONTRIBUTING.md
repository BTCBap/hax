# Contributing to hax

Thanks for your interest. hax is a Bitcoin wallet, so correctness and safety come before features.

## Ground rules

- **Security issues go to [SECURITY.md](SECURITY.md)**, not public issues or PRs.
- Keep PRs small and focused, with one logical change each. Prefer a test-backed fix over a broad rewrite.
- Anything that touches key material, derivation, PSBT construction or signing, fee and change logic, address or network handling, or storage needs tests and a clear explanation in the PR description.
- Don't make mainnet or production-readiness claims that the code doesn't back up.
- Don't add new dependencies without a good reason. Each one is attack surface.

## Development setup

Requirements: Go (version in `go.mod`), `make`, and a Unicode terminal. Optional tools: `golangci-lint` v2, `govulncheck`, `goreleaser`, `vhs`.

```bash
make build                  # ./hax
make test                   # go test ./...
make test-race              # go test -race ./...
go vet ./...
golangci-lint run ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
goreleaser release --snapshot --clean --skip=sign,sbom   # local release dry run
```

Run against testnet4 only. Use a throwaway `HOME` to keep test wallets isolated:

```bash
HOME=$(mktemp -d) ./hax
```

## Tests

- Wallet logic: `internal/wallet`. Backends: `internal/backend`. UI: `internal/ui/models` (Bubble Tea models, plus `teatest` where useful).
- UI golden snapshots: `go test ./internal/ui/models -tags golden -run TestGolden [-update]`.
- README screenshots are generated, not hand-edited:
  ```bash
  HAX_SCREENSHOT_DIR=/tmp/hax-screens go test ./internal/ui/models -tags screenshots -run TestScreenshots
  vhs docs/images/screens.tape
  ```

CI runs vet, race tests with coverage, golangci-lint (new issues only), govulncheck, and a GoReleaser snapshot build on Linux, macOS, and Windows. A PR must be green before merge.

## Commits and PRs

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
fix(wallet): reject foreign-network addresses in ValidateBitcoinAddress
feat(send): add RBF fee bump
chore(deps): bump golang.org/x/crypto to v0.58.0
```

Common types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `style`. Mark breaking changes with `!` and a `BREAKING CHANGE:` footer.

A PR description should say what changed, why, how you verified it, and what is still open.

## Releases

Maintainers tag `vX.Y.Z` on `main`. The release workflow builds reproducible archives, signs `checksums.txt` keyless with cosign, attaches SBOMs, attests build provenance, and opens a **draft** release for manual review before it's published.

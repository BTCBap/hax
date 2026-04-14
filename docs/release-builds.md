# Release Builds

This document describes the current release-build setup for `hax`.

## Current Capability

`hax` now has:

- a local release-style build path
- a cross-platform build path
- a GoReleaser configuration
- a tag-triggered release workflow scaffold
- SHA256 checksum generation in the release config

Official GitHub release binaries just are not the main path yet.

## Local Release-Style Build

Build a single local release binary with version metadata:

```bash
make build-release
```

Build cross-platform binaries locally:

```bash
make build-all
```

Build cross-platform binaries and generate a local checksum manifest:

```bash
make checksums
```

## Local GoReleaser Snapshot

If `goreleaser` is installed locally, you can run a snapshot build without publishing:

```bash
goreleaser release --snapshot --clean
```

This writes release artifacts to `dist/`.

## Tagged Release Flow

The release workflow in `.github/workflows/release.yml` is designed for tags like:

```bash
git tag v1.0.0-testnet
git push origin v1.0.0-testnet
```

That workflow is expected to:

- run GoReleaser
- build platform archives
- generate `checksums.txt`
- create a draft GitHub release

## What Still Needs Human Review

Before leaning on GitHub releases as the normal install path, the project still needs:

- target-platform validation of produced archives
- published verification instructions alongside an actual release
- confirmation that the beta signing decision is still acceptable
- confirmation that the generated version metadata matches expectations

## Signing Decision For Beta

Current decision:

- no signing
- no attestation
- source-first remains the primary distribution story

This is fine for now because the repo source is still the main install path.

Revisit this before positioning tagged binaries as the default install path.

## Current Beta Position

For now:

- source-first
- release pipeline exists but is secondary
- checksums generated once tagged releases are used

See [release-verification.md](release-verification.md) for the checksum workflow users should follow once artifacts are published.

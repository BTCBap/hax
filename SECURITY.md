# Security Policy

hax is experimental software for **Bitcoin testnet4**. Do not use it with keys or seed phrases that control real bitcoin.

## Reporting a vulnerability

**Please do not open public issues, discussions, or PRs for security problems.**

Report privately through GitHub's **[Private Vulnerability Reporting](https://github.com/BTCBap/hax/security/advisories/new)** (Security tab → "Report a vulnerability").

Please include:

- Affected version / commit and platform
- A description of the issue and its impact
- Reproduction steps or a proof of concept (testnet4 or regtest only)
- Any suggested fix

Expected timeline (best effort, single maintainer):

| Step | Target |
|---|---|
| Acknowledgement | 72 hours |
| Initial assessment | 7 days |
| Fix or mitigation | Depends on severity; critical key-material issues are prioritized |
| Disclosure | Coordinated with the reporter, normally within 90 days, via a GitHub Security Advisory (and CVE where applicable) |

Reporters are credited unless they ask not to be.

## Scope

High priority:

- Exposure of seed, private keys, passphrase, or wallet password (disk, memory, logs, clipboard, terminal)
- Wallet file encryption or KDF weaknesses
- Address derivation, network mix-ups, or wrong-network acceptance
- PSBT construction or signing flaws, and hidden fee or change manipulation
- Backend responses that can silently change wallet meaning
- Path traversal, command injection (e.g. HWI invocation), or arbitrary file write
- Release supply chain: build, signing, or provenance

Out of scope:

- Attacks that require a fully compromised host
- Social engineering
- Bugs in third-party hardware wallet firmware or HWI, unless hax triggers them

## Supported versions

Only the latest release and `main` get security fixes.

## Verifying releases

Release archives come with a cosign-signed `checksums.txt`, SBOMs, and SLSA provenance. See [docs/release-verification.md](docs/release-verification.md).

# Security Policy

`hax` is a testnet4 wallet and still experimental.

Use testnet funds only.

## Reporting A Vulnerability

Please do not open a public issue with exploit details.

Preferred path:

1. Use GitHub Private Vulnerability Reporting for the repository, if it is enabled.
2. If that is not enabled yet, do not post exploit details publicly. Open a minimal issue asking for a private contact path without including the vulnerability details.

Include:

- A clear description of the issue
- Affected version or commit
- Reproduction steps
- Expected impact
- Any proof-of-concept material that helps verify the report

## Response Expectations

Response times are best effort.

Before broader public sharing, GitHub Private Vulnerability Reporting should be enabled for the repo.

## Scope

High-priority issues include:

- Wallet encryption or key-material exposure
- PSBT signing or transaction-construction flaws
- Address derivation or network-mixup bugs
- Unsafe backup or recovery behavior
- Remote-code execution or command-injection bugs

Out of scope for now:

- Social engineering
- Denial-of-service from hostile local environments
- Bugs in third-party wallet firmware or external services unless `hax` is the trigger

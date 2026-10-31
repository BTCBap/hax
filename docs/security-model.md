# Security Model

This page records how hax protects local wallet data and what it assumes about the machine it runs on. The broader threat model, including assets, trust boundaries, and scope, is in [threat-model.md](threat-model.md). Report vulnerabilities as described in [../SECURITY.md](../SECURITY.md).

## Audit status

hax has not been independently audited. Try new workflows on testnet4 first.

## Wallet storage

Wallet files are encrypted at rest with AES-256-GCM. The encryption key is derived from your password with Argon2id using 64 MiB of memory, three iterations, and four lanes. Wallets from older versions that used PBKDF2 are migrated on unlock.

Wallet files are written with `0600` permissions in a `0700` directory. The wallet file holds the seed, an optional BIP39 passphrase, accounts, labels, and cached data.

## Servers and logging

Electrum connections use TLS and can route through Tor SOCKS5. The server trust rules, including pinned server keys, are in [../SECURITY.md](../SECURITY.md).

`DEBUG=1` writes a debug log for troubleshooting. Review a log before sharing it, and treat wallet-related logs as sensitive.

## Release integrity

Release archives are signed and attested. The verification steps are in [install.md](install.md#verify-a-release-download) and [release-verification.md](release-verification.md).

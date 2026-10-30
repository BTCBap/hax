# Security Review v1.3 — hax

**Repository:** `github.com/BTCBap/hax`
**Revision reviewed:** `ca87a86aa4c156f88ace4887a0440ae7e284f13e` (2026-10-29, `v1.2.0`)
**Review date:** 2026-10-30
**Reviewer:** adversarial security audit
**Status of source tree:** unchanged; this document is the only file added.

## Scope and method

The review covered every Go package under `cmd/` and `internal/`, the CI and
release workflows under `.github/workflows`, `.goreleaser.yaml`, the
`Makefile`, `go.mod`/`go.sum`, `SECURITY.md`, and the files under `docs/`.
Roughly 57k lines of Go were read. Findings were verified against the code
rather than inferred from comments or documentation. Several suspected issues
were tested against the underlying libraries and discarded (see
[Discarded false positives](#discarded-false-positives)).

Dependency scanning was run with:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

The pre-installed `~/go/bin/govulncheck` is built with Go 1.26 and fails
against the Go 1.27.3 standard library (`unknown field rfd`,
`file requires newer Go version go1.27`), so the module-run form was used.
`go.mod` declares `go 1.26.10`; the local toolchain is `go1.27.3`.

## Threat model used

The wallet is a hot software wallet. The relevant adversaries are:

- a malicious or compromised Electrum server or Bitcoin Core endpoint;
- a malicious PSBT or QR/UR payload supplied to the wallet;
- a local attacker or malware running as the same user, including clipboard
  readers and process-memory readers;
- a thief who obtains the encrypted wallet file and attacks the password
  offline;
- a supply-chain attacker against the build and release path.

Attacks requiring root, a fully compromised host, or physical coercion are out
of scope, consistent with `SECURITY.md`.

## Executive summary

No Critical finding was confirmed. The core cryptographic primitives are sound
(`crypto/rand` entropy, AES-256-GCM with fresh nonces, authenticated
encryption, strict per-network address decoding), and the release workflow is
unusually well hardened. The material risks are concentrated in four places:

1. **Offline password cracking.** PBKDF2-HMAC-SHA256 with 100,000 iterations
   and an 8-character minimum is inexpensive to attack on GPUs once the wallet
   file is stolen.
2. **Backend-controlled economics.** Fee estimates from Electrum and Bitcoin
   Core are used without an upper bound, so a malicious server can turn a
   normal send into a fee drain.
3. **Blind PSBT signing.** The inspection screen is advisory; one keypress
   signs an imported PSBT, including legacy inputs whose amounts can be
   understated to hide the real fee.
4. **Descriptor correctness.** A `multi(...)` descriptor is silently treated
   as `sortedmulti(...)`, deriving addresses that do not match the imported
   wallet.

| Severity | Count |
|---|---:|
| Critical | 0 |
| High | 5 |
| Medium | 16 |
| Low | 23 |
| Info | 5 |
| **Total** | **49** |

## High

### H3-01 — Weak KDF and password policy allow cheap offline cracking

- **Severity:** High
- **Location:** `internal/wallet/storage.go:21-24`, `internal/wallet/storage.go:280-282`, `internal/wallet/validation.go:10`, `internal/wallet/validation.go:38-42`
- **Status:** open
- **Exploit scenario:** An attacker who obtains `~/.hax/<name>.wallet` (stolen
  laptop, backup, sync service, mis-set permissions) runs an offline dictionary
  or brute-force attack. The file format is public and self-describing: a
  32-byte salt, a 12-byte GCM nonce, and ciphertext. Each guess costs one
  PBKDF2-HMAC-SHA256 evaluation at 100,000 iterations. Commodity GPUs and
  dedicated rigs evaluate this at very high rates, and the only password rule
  is "at least 8 characters". A human-chosen 8–10 character password falls
  quickly. The GCM tag then confirms a correct guess.
- **Recommended fix:** Move to Argon2id (or scrypt) with memory-hard parameters
  stored alongside the wallet (for example Argon2id `m=64 MiB, t=3, p=4`), and
  raise PBKDF2 to at least 600,000 iterations only as a migration fallback.
  Store a KDF identifier and parameters in `EncryptedWalletFile`, raise the
  minimum password length to 12–16, and add a simple strength check. Consider
  an optional high-entropy recovery key.

### H3-02 — Uncapped backend fee estimates can drain the wallet through fees

- **Severity:** High
- **Location:** `internal/ui/models/send.go:934-942`, `internal/ui/models/send.go:1051-1061`, `internal/backend/electrum_backend.go:1303-1319`, `internal/backend/electrum_backend.go:1370-1393`, `internal/backend/rpc_backend.go:418-460`
- **Status:** open
- **Exploit scenario:** A malicious Electrum server answers
  `blockchain.estimatefee` (or a compromised Bitcoin Core answers
  `estimatesmartfee`) with an enormous rate. `btcPerKBToSatPerVB` and the RPC
  converter only floor the rate at 0.1 sat/vB; neither imposes an upper bound.
  The send screen copies `FastestFee`/`HalfHourFee`/`HourFee` directly into
  transaction construction. The user sees a fee on the review screen, but only
  the *custom* fee path has a 500/10,000 sat/vB soft/hard cap and a typed
  confirmation. With a preset selected, `s` then `b` broadcasts a transaction
  whose entire input value can become fee. Very large values also overflow the
  `float64`→`int64` conversion in `CreatePSBT`, producing a negative displayed
  fee while the transaction pays nearly everything to miners. A colluding
  miner collects the value.
- **Recommended fix:** Clamp all fee estimates, including backend-provided
  ones, to a hard maximum (for example 1,000 sat/vB) and require the same typed
  confirmation above a soft threshold. Reject non-finite and out-of-range
  values before conversion. Sanity-check that the computed fee does not exceed
  a configurable percentage of the input total. Treat a blocked or absurd
  estimate as unavailable rather than using it.

### H3-03 — PSBT signing is not gated on inspection warnings

- **Severity:** High
- **Location:** `internal/ui/models/send_keys.go:801-842`, `internal/wallet/psbt_inspect.go:62-174`, `internal/ui/models/send_inspect.go:33-52`, `internal/ui/models/send_views.go:480-520`
- **Status:** open
- **Exploit scenario:** An attacker sends the victim a PSBT, for example as a
  "transaction to co-sign" in a trade or multisig coordination flow. The
  wallet imports it and shows an inspection screen. `InspectPSBT` correctly
  detects external outputs, spoofed change, missing input amounts, high fee
  rates, dust, and non-`SIGHASH_ALL` flags, but the warnings are purely
  advisory. Pressing `s` signs immediately; pressing `b` broadcasts once the
  PSBT is complete. There is no "type the fee" or "confirm external output"
  gate. A user who trusts the counterparty, or who does not read the warning
  box, signs a payment to the attacker.
- **Recommended fix:** Require explicit confirmation for any PSBT whose
  inspection has a warning of class "external output", "unknown input",
  "high fee", "spoofed ownership", or "non-ALL sighash". Use a typed
  confirmation (amount and destination) for external outputs, and refuse to
  sign when an input amount is unknown. Make the warning state part of the
  signing precondition in the model, not only the view.

### H3-04 — Imported legacy PSBT input amounts are trusted, hiding the real fee

- **Severity:** High
- **Location:** `internal/wallet/psbt_signer_data.go:18-34`, `internal/wallet/psbt.go:686`, `internal/wallet/psbt.go:1159-1204`, `internal/wallet/psbt_inspect.go:178-196`, `internal/wallet/psbt_manager.go:443-478`
- **Status:** open
- **Exploit scenario:** A legacy P2PKH input's signature does not commit to the
  input amount. An attacker crafts a PSBT that spends a victim UTXO to an
  attacker output, supplies a `WitnessUtxo` whose value is *lower* than the
  real previous output, and omits `NonWitnessUtxo`. `inputPrevOut` prefers
  `WitnessUtxo`; `checkInputUTXOs` returns no warning when `NonWitnessUtxo` is
  nil; `ImportPSBTFromBase64` computes the displayed fee from the same
  understated value; `signLegacyInput` checks only that the script matches the
  derived key and signs with `SIGHASH_ALL`. The signature is valid on chain
  because legacy sighash does not cover the amount. The difference between the
  real input and the outputs is paid as fee. If both fields are present and
  inconsistent, the inspector warns but signing still uses `WitnessUtxo`.
- **Recommended fix:** For any non-witness input, require `NonWitnessUtxo`,
  verify `txid` and output index, and derive the amount and script from it.
  Never use `WitnessUtxo` as the amount source for a legacy input. Refuse to
  sign when the two disagree. Compute the displayed fee from verified previous
  transactions, and block signing when any input amount is unverified.

### H3-05 — `multi(...)` descriptors are silently treated as `sortedmulti(...)`

- **Severity:** High
- **Location:** `internal/wallet/descriptor.go:551`, `internal/wallet/descriptor.go:656-730`, `internal/wallet/storage.go:52-58`, `internal/wallet/hdwallet.go:729-737`, `internal/wallet/hdwallet.go:763-822`
- **Status:** open
- **Exploit scenario:** A user imports an existing multisig descriptor that uses
  `multi(...)`, whose key order is significant. `ParseDescriptor` accepts both
  `multi(` and `sortedmulti(` but stores only `M`, `N`, and script type; there
  is no `Sorted` field. Every address generator unconditionally calls
  `SortPublicKeys`. The wallet therefore derives a different redeem/witness
  script and a different address from the descriptor the user supplied. Funds
  at the real addresses are invisible, and the user may conclude the wallet is
  empty or restore it incorrectly. If the original descriptor or cosigner
  information is lost, access to the funds is at risk.
- **Recommended fix:** Parse and persist whether the descriptor used `multi`
  or `sortedmulti` (and, generally, preserve the exact descriptor). Generate
  scripts in the original key order for `multi`, BIP67 order only for
  `sortedmulti`. Verify the first derived address against the descriptor
  before accepting the import, and warn loudly on any mismatch.

## Medium

### H3-06 — Backend-reported UTXO values drive fee/change; segwit conflicts are only warnings

- **Severity:** Medium
- **Location:** `internal/wallet/tx_sender.go:18-68`, `internal/backend/electrum_backend.go:945-989`, `internal/wallet/psbt_signer_data.go:82-115`
- **Status:** open
- **Exploit scenario:** `resolveUTXOs` takes each input's `value` from the
  Electrum `listunspent` response and uses it to compute change and fee.
  `attachPrevTxs` fetches the previous transaction and compares it with the
  reported value, but for segwit v0 it treats a mismatch as a warning and
  continues. If the server lies about the value and withholds the previous
  transaction, the wallet signs with the false amount. For segwit the amount is
  committed by the signature, so the resulting transaction is invalid and
  cannot be broadcast: the impact is a denial of spending and confusing fee
  display, not theft. Legacy inputs fail closed. Taproot also commits amounts.
- **Recommended fix:** Treat any failure to obtain and verify the previous
  transaction as fatal for every input type, including segwit. Cross-check the
  `listunspent` value against the verified previous output and abort on
  mismatch. Do not build a PSBT from unverified input values.

### H3-07 — Unbounded Electrum response buffering enables memory exhaustion

- **Severity:** Medium
- **Location:** `internal/backend/electrum_backend.go:416-444`, `internal/backend/electrum_backend.go:459`
- **Status:** open
- **Exploit scenario:** `readLoop` uses `bufio.Reader.ReadBytes('\n')` with no
  size limit. On an idle timeout it appends whatever it has to `partial` and
  continues, so a malicious server can send one byte every 59 seconds and grow
  the buffer indefinitely. Even without a timeout, a single very long line is
  buffered in full. The line is then passed to `json.Unmarshal`, which
  allocates further. A malicious or compromised server can exhaust the
  terminal process's memory.
- **Recommended fix:** Cap the line length (for example 1 MiB) with
  `io.LimitReader`/`bufio.Reader.ReadLine` and drop the connection on overflow.
  Cap `partial` as well. Set a maximum JSON depth/size for decoded responses.

### H3-08 — Electrum `GetTransaction` does not verify the returned transaction hash

- **Severity:** Medium
- **Location:** `internal/backend/electrum_backend.go:1090-1176`, `internal/backend/rawtx.go:13-25`
- **Status:** open
- **Exploit scenario:** `GetTransaction(txid)` calls
  `blockchain.transaction.get` and deserializes the returned hex, then sets
  `TxID` to the *requested* `txid` without hashing the transaction. A malicious
  server can return an unrelated transaction. The history screen will show the
  wrong amounts, fee, and outputs under the requested txid; `resolveUTXOs` uses
  the same call to map a UTXO to an address, so the displayed ownership data
  can be spoofed. The codebase already has the correct pattern in
  `decodeRawTx`, which compares `tx.TxHash().String()` to the requested id.
- **Recommended fix:** Compute `msgTx.TxHash()` and reject a response that does
  not match the requested txid, exactly as `decodeRawTx` and `fetchPrevTx` do.
  Also verify the transaction's shape when it is used to resolve an outpoint.

### H3-09 — No SPV proofs; genesis-only network check and fail-open when unreachable

- **Severity:** Medium
- **Location:** `internal/wallet/network_check.go:29-55`, `internal/wallet/network_check.go:73-87`, `internal/backend/electrum_backend.go:1090-1176`, `internal/backend/genesis.go`
- **Status:** open
- **Exploit scenario:** The wallet trusts the Electrum server for headers,
  history, UTXO set, balances, and fee estimates. There are no merkle proofs or
  header-chain validation; the only chain check is that block 0 hashes to the
  expected genesis. A malicious server can present a fabricated history and
  balance, spoof confirmations and timestamps, and return `false, nil` for
  `GenesisHash` so that `checkBackendNetwork` treats the check as
  "unavailable". `ensureNetworkVerified` then returns nil and broadcasting
  proceeds. The server can also claim a correct genesis while serving false
  data for every other block.
- **Recommended fix:** Implement Electrum SPV: fetch and validate the header
  chain with difficulty/checkpoint rules, and verify merkle proofs for each
  transaction used in balances and history. At minimum, treat an unavailable
  genesis check as a hard failure before signing or broadcasting, and document
  clearly that the Electrum backend is trusted for chain state. Prefer a
  user-controlled Bitcoin Core node for high-value wallets.

### H3-10 — No unlock throttling, lockout, or delay

- **Severity:** Medium
- **Location:** `internal/ui/models/unlock.go:113-115`, `internal/ui/models/unlock.go:180-190`
- **Status:** open
- **Exploit scenario:** An attacker with access to a running unlocked terminal
  session, or to the UI through a remote terminal, can submit unlimited
  password guesses. Each failed attempt only clears the input and shakes the
  lock icon. There is no per-wallet delay, exponential backoff, attempt
  counter, lockout, or CAPTCHA-equivalent. With the weak KDF from H3-01 and
  common passwords, online guessing is practical.
- **Recommended fix:** Add per-wallet failed-attempt tracking with exponential
  backoff (for example 1 s, 2 s, 4 s, up to several minutes) and a persistent
  lockout after a configurable threshold. Consider requiring the OS keyring or
  a hardware token for repeated failures. Log failures without logging
  passwords.

### H3-11 — Secret material persists in immutable strings and long-lived UI models

- **Severity:** Medium
- **Location:** `internal/ui/models/wallet_setup.go:92-100`, `internal/ui/models/wallet_setup.go:544`, `internal/ui/models/wallet_setup.go:627`, `internal/ui/models/wallet_setup.go:989`, `internal/ui/models/wallet_setup_backup.go:106-119`, `internal/ui/models/root.go:309-313`, `internal/wallet/hdwallet.go:428`, `internal/wallet/manager.go:672-702`, `internal/wallet/secret.go`
- **Status:** open
- **Exploit scenario:** The mnemonic, BIP39 passphrase, and wallet password are
  held in Go `string` fields in `WalletSetupModel`. `finishCreate` has a value
  receiver and clears only `backupWords` and the backup input, so the copy
  stored in `RootModel.walletSetupModel` keeps `generatedMnemonic`,
  `password`, and `passphrase` after the wallet opens. `NavigateMsg` does not
  nil `walletSetupModel`. The unlock screen's password input is not cleared on
  success, and `RootModel` keeps the unlock model after navigation. The HD
  wallet's `masterKey` has no wipe path. `LockWallet` drops references and
  zeroes the encryption key, but the seed, passphrase, and mnemonic strings
  cannot be erased and remain until garbage collection (and may be copied by
  the runtime). A memory dump, core file, swap, or same-user debugger recovers
  them.
- **Recommended fix:** Minimize secret lifetime: clear `generatedMnemonic`,
  `password`, `passphrase`, and the password inputs immediately after use;
  nil `walletSetupModel` and `unlockModel` on navigation and lock; add a
  `Destroy`/`Wipe` method to `HDWallet` that zeroes the extended private key
  material. Prefer `[]byte` for passphrases and passwords end to end where the
  library allows it, and disable core dumps (`RLIMIT_CORE=0`) and `mlock`
  sensitive pages where the platform permits.

### H3-12 — Clipboard contents are never cleared

- **Severity:** Medium
- **Location:** `internal/ui/models/addresses.go:222`, `internal/ui/models/receive.go:307`, `internal/ui/models/utxos.go:190`, `internal/ui/models/history.go:414`, `internal/ui/models/settings.go:353`, `internal/ui/models/settings.go:553`, `internal/ui/models/send_keys.go:790`, `internal/ui/models/send_keys.go:923`, `internal/ui/models/send_keys.go:971`
- **Status:** open
- **Exploit scenario:** The wallet copies addresses, outpoints, txids, the
  wallet descriptor, an xpub, the full PSBT base64, and raw transaction hex to
  the system clipboard. There is no auto-clear timer and no clear-on-exit.
  Clipboard contents are readable by every application running as the user,
  persist after the terminal closes, and may be synchronized by clipboard
  managers or cloud clipboard features. The descriptor and xpub expose the
  wallet's entire address chain and privacy history. A pasted raw transaction
  or PSBT can be intercepted before broadcast.
- **Recommended fix:** Clear the clipboard after a short timeout (for example
  30–60 seconds), and on lock/exit. Offer a "copy" confirmation with a visible
  countdown. Avoid copying the descriptor and xpub unless the user explicitly
  requests export; prefer a file export with 0600 permissions. Note that
  clearing is best-effort because `atotto/clipboard` has no ownership API.

### H3-13 — Bitcoin Core RPC credentials are sent over cleartext HTTP and stored in plaintext

- **Severity:** Medium
- **Location:** `internal/ui/models/connection_setup.go:586-593`, `internal/backend/rpc_backend.go:125-132`, `internal/backend/config.go:30-34`, `internal/config/config.go:109`
- **Status:** open
- **Exploit scenario:** The connection setup UI hardcodes `UseSSL: false` for
  the RPC backend, so `callRPC` sends the username and password in a Basic Auth
  header over plain HTTP. A network attacker, a malicious Wi-Fi access point,
  or a compromised router between the wallet and a remote Bitcoin Core node
  captures the credentials. Those credentials can then be used to control the
  node (including `sendrawtransaction`, wallet RPCs, or node shutdown), and to
  observe or manipulate the wallet's view of the chain. The credentials are
  also written to `~/.hax/config.json` in plaintext (0600) and appear in the
  UI model.
- **Recommended fix:** Default RPC to HTTPS, expose a TLS toggle and a CA pin
  option, and refuse non-loopback plain HTTP unless the user explicitly
  overrides with a warning. Store credentials in the OS keyring or encrypt them
  with the wallet password. Redact the password from any config dump or debug
  output.

### H3-14 — RPC host is interpolated into a URL, allowing credential redirection

- **Severity:** Medium
- **Location:** `internal/backend/rpc_backend.go:176`, `internal/config/config.go:247-251`, `internal/ui/models/connection_setup.go:441-443`, `internal/ui/models/connection_setup.go:586-591`
- **Status:** open
- **Exploit scenario:** `baseURL` is built with
  `fmt.Sprintf("%s://%s:%d", scheme, host, port)`. The host is validated only
  for being non-empty (the `config.Validate` function is not called on
  load/save, and the UI checks only `hostInput.Value() == ""`). A host such as
  `127.0.0.1@attacker.example` parses as userinfo `127.0.0.1` plus host
  `attacker.example`, so the HTTP request carrying `SetBasicAuth` credentials
  is sent to the attacker. A host containing `/`, `?`, or `#` can similarly
  change the path or query. The same value is written to `config.json`.
- **Recommended fix:** Build the URL with `url.URL{Scheme: scheme,
  Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/"}` and validate
  that the host is a bare hostname/IP (or a bracketed IPv6 literal) with no
  userinfo, path, query, or fragment. Apply the same validation in the config
  loader and UI, and reject credentials in the host field.

### H3-15 — Unbounded RPC response body enables memory exhaustion

- **Severity:** Medium
- **Location:** `internal/backend/rpc_backend.go:140`
- **Status:** open
- **Exploit scenario:** `io.ReadAll(resp.Body)` reads an arbitrary amount from
  the RPC endpoint. A malicious or compromised Bitcoin Core endpoint (or a
  MITM on a plain-HTTP connection) can stream gigabytes and exhaust the
  terminal process's memory. The 30-second client timeout limits duration but
  not the amount read within that window on a fast link.
- **Recommended fix:** Wrap the body in `io.LimitReader` with a sensible cap
  (for example 8–32 MiB, depending on the largest legitimate RPC response) and
  return an error on overflow. Reject non-JSON content types and oversized
  `Content-Length` values.

### H3-16 — Wallet/config/pin writes lack durability and safe temp-file handling

- **Severity:** Medium
- **Location:** `internal/wallet/storage.go:220-228`, `internal/config/config.go:99-109`, `internal/backend/pinning.go:68-80`
- **Status:** open
- **Exploit scenario:** `SaveWalletWithKey` writes to a predictable
  `<wallet>.wallet.tmp` with `os.WriteFile` (which follows symlinks and does
  not use `O_EXCL`), then `os.Rename`s it over the wallet. There is no
  `fsync` of the file or the containing directory. A crash or power loss after
  `Rename` can leave a zero-length or partially persisted wallet; on some
  filesystems the rename itself may not be durable. During wallet creation this
  can destroy the only copy of a newly generated mnemonic before the user has
  written it down. A local attacker with write access to the directory can
  pre-create the `.tmp` path as a symlink to another file, which the wallet
  overwrites. `config.Save` is a direct `os.WriteFile` with no temp+rename, so
  a crash can corrupt the config (including the RPC password). The pin store
  has the same no-fsync pattern.
- **Recommended fix:** Use `os.OpenFile(tmp, O_WRONLY|O_CREATE|O_EXCL|O_NOFOLLOW,
  0600)`, write, `Sync`, close, `Rename`, then open and `Sync` the containing
  directory. Use `os.CreateTemp` with a random name in the same directory.
  Apply the same atomic-write helper to config and pin files. Keep a backup of
  the previous wallet until the new file is durably synced.

### H3-17 — Existing file and directory permissions are not tightened

- **Severity:** Medium
- **Location:** `internal/wallet/storage.go:158-166`, `internal/wallet/storage.go:222`, `internal/config/config.go:99`, `internal/config/config.go:109`, `internal/backend/pinning.go:70-80`
- **Status:** open
- **Exploit scenario:** `os.MkdirAll(..., 0700)` and
  `os.WriteFile(..., 0600)` set permissions only when creating a new file or
  directory. If `~/.hax`, an existing wallet file, `config.json`, or
  `known_servers.json` was previously created with looser permissions (for
  example by an older version, a restore, a copied home directory, or a
  permissive umask), the wallet leaves them world-readable. A local user or a
  backup/sync process can then read the encrypted wallet and attack it offline
  (see H3-01) or read the RPC password from the config.
- **Recommended fix:** On every load/save, `os.Chmod` the wallet directory to
  0700 and sensitive files to 0600; refuse to operate if the directory or
  wallet file is group/world-accessible. Verify ownership where possible.
  Document the expected permissions.

### H3-18 — Auto-lock fails open and leaves setup/connection state resident

- **Severity:** Medium
- **Location:** `internal/ui/models/autolock.go:20-36`, `internal/ui/models/autolock.go:41-55`, `internal/ui/models/autolock.go:59-87`, `internal/ui/models/connection_setup.go:589-591`
- **Status:** open
- **Exploit scenario:** `autoLockAfter` returns `0` (disabled) when the config
  manager is nil or `Load` fails. A corrupted or unreadable `config.json`
  therefore silently disables auto-lock, leaving an unlocked wallet on screen
  indefinitely. `shouldAutoLock` explicitly excludes `StateConnectionSetup`,
  so opening the connection screen from an unlocked wallet (settings →
  connection) suspends auto-lock while the wallet remains unlocked. Auto-lock
  nils the dashboard, send, receive, history, addresses, UTXOs, settings, and
  help models, but not `walletSetupModel` or `connectionSetupModel`; the latter
  can hold the RPC username/password in its text inputs. A passer-by can use
  the unlocked wallet or read the secrets from process memory.
- **Recommended fix:** Treat a config load error as "use the default timeout"
  and log it, rather than disabling auto-lock. Apply auto-lock in every state
  where the wallet is unlocked, including connection setup (or lock the wallet
  before entering that screen). Nil and recreate all secret-bearing models on
  lock. Clear the RPC password input and wallet-setup inputs when leaving those
  screens.

### H3-19 — Cosigner seed harvesting across local wallets without per-wallet consent

- **Severity:** Medium
- **Location:** `internal/wallet/cosigner_manager.go:259-330`, `internal/wallet/cosigner_manager.go:407-426`, `internal/ui/models/settings_multisig.go:337-363`
- **Status:** open
- **Exploit scenario:** `ScanLocalWalletsForCosigners` takes one password and
  attempts to decrypt *every* other wallet in `~/.hax` with it, then derives
  each seed and matches it against the current multisig cosigners. The UI
  `scanAndImportCosigners` imports every match after a single password prompt,
  with no per-wallet confirmation. As a result, a user who intends to sign one
  multisig wallet can copy the seed phrases of unrelated single-sig wallets
  into that wallet's encrypted cosigner metadata (`MultisigCosignerConfig.Mnemonic`).
  This expands the blast radius: compromise of the multisig wallet now exposes
  seeds for other wallets, and the copied seeds are persisted in a second
  encrypted container. The scan also decrypts wallets the user may have
  forgotten about.
- **Recommended fix:** Require explicit per-wallet selection and confirmation
  before decrypting or importing each seed. Show the wallet name, fingerprint,
  and matched cosigner. Default to importing only the seed the user explicitly
  chose. Do not persist cosigner mnemonics by default; store xpubs and require
  the seed at signing time, or keep it in the OS keyring.

### H3-20 — Plain TCP Electrum connections are permitted

- **Severity:** Medium
- **Location:** `internal/backend/electrum_backend.go:216-223`, `internal/ui/models/connection_setup.go:531-545`
- **Status:** open
- **Exploit scenario:** The custom-server UI exposes an SSL toggle. With SSL
  off, the wallet speaks Electrum JSON-RPC over plain TCP. A network attacker
  can read all queried scripthashes (privacy), alter balances, history, UTXO
  values, and fee estimates (H3-02/H3-06), and return a fake broadcast txid.
  The `.onion` path also disables SSL, which is acceptable over Tor but
  confusing if the proxy is misconfigured.
- **Recommended fix:** Require TLS for all clearnet Electrum servers; allow
  plain TCP only for loopback, a Unix socket, or an explicitly confirmed
  Tor-only connection. Show a prominent warning and require confirmation when
  SSL is disabled. Consider certificate pinning as mandatory for non-loopback
  servers.

### H3-21 — Multisig signing does not verify the supplied script against the prevout

- **Severity:** Medium
- **Location:** `internal/wallet/psbt.go:1206-1390` (script selection at `1315-1360`), `internal/wallet/psbt.go:894-1096`
- **Status:** open
- **Exploit scenario:** `SignMultisigPSBT` and
  `SignMultisigPSBTFromDerivation` take `WitnessScript` or `RedeemScript` from
  the PSBT, derive the cosigner key, and sign that script. For legacy P2SH the
  partial signature is appended directly because `updater.Sign` would reject
  it. The signer never recomputes `SHA256(script)`/`HASH160(script)` and
  compares it with the UTXO's `PkScript`. A malicious PSBT can present a
  different multisig script that contains the same cosigner keys (for example a
  lower threshold, or a different key subset) and obtain signatures over it.
  The resulting transaction is invalid for the actual UTXO, so the immediate
  impact is an integrity/availability failure and wasted signing work rather
  than theft. It also weakens the guarantee that the wallet only signs scripts
  it derived.
- **Recommended fix:** Before signing, reconstruct the expected script from the
  wallet's stored cosigner xpubs and the input's derivation, and require it to
  equal the supplied `WitnessScript`/`RedeemScript` and to hash to the
  `PkScript` in the verified prevout. Reject unknown or mismatched scripts.
  Verify the script hash for both legacy and segwit paths.

## Low

### H3-22 — Debug logging records wallet addresses, paths, and transaction data

- **Severity:** Low
- **Location:** `cmd/hax/main.go:24-31`, `internal/wallet/psbt.go:470-478`, `internal/backend/electrum_backend.go:490-500`
- **Status:** open
- **Exploit scenario:** With `DEBUG` set, Bubble Tea writes `debug.log` in the
  current directory (mode 0600, which is correct). `psbtDebugLog` records UTXO
  txids, addresses, derivation paths, and scripts; the Electrum reader logs
  scripthash prefixes. No mnemonic, seed, xprv, password, or passphrase was
  found in any log path, but the log is a privacy-sensitive record of the
  wallet's addresses and activity that persists after the session.
- **Recommended fix:** Keep debug logging off by default and document it as
  sensitive. Avoid logging full addresses and derivation paths; use truncated
  values. Write the log under `~/.hax` with 0600, add rotation/expiry, and
  consider redaction in release builds.

### H3-23 — No top-level panic recovery

- **Severity:** Low
- **Location:** `cmd/hax/main.go:90-100`, `internal/ui/models/root.go:235`, `internal/backend/electrum_backend.go:401`
- **Status:** open
- **Exploit scenario:** The Electrum read loop has a local `recover`, but the
  Bubble Tea update/view path and the wallet code do not. A panic in a decoder,
  a malformed server response, or a UI index bug terminates the program. The
  alt screen is restored, and Go prints a stack trace to the terminal. Stack
  traces generally do not include secret values, but they can expose paths,
  function names, and partial state. A panic that unwinds the main goroutine
  still runs `defer walletManager.LockWallet()`, but a panic in a Bubble Tea
  worker goroutine or a fatal runtime error (out of memory, a fatal signal)
  terminates the process without running main's deferred cleanup, leaving the
  decrypted seed in the address space at exit.
- **Recommended fix:** Install a top-level `defer recover()` in `main` that
  restores the terminal, writes a minimal diagnostic to a 0600 file, and exits
  without dumping memory. Add targeted recover blocks around PSBT/UR/CBOR
  decoding. Ensure key material is wiped in a deferred cleanup that runs on
  panic.

### H3-24 — Silent trust-on-first-use for Electrum TLS

- **Severity:** Low
- **Location:** `internal/backend/pinning.go:95-118`
- **Status:** open
- **Exploit scenario:** The first successful TLS connection to a server records
  its SPKI hash with no user confirmation. An attacker who can intercept the
  first connection to a hostname for which they hold a valid CA-issued
  certificate (DNS hijack, hostile network with a corporate CA, or a
  compromised public server) becomes the pinned server. Later key changes are
  blocked, which is good, but the initial trust decision is automatic. The pin
  file can also be deleted by the same user to force a re-pin.
- **Recommended fix:** On first use, show the SPKI fingerprint and ask the user
  to confirm, or ship a small set of known pins for the default public servers.
  Record the pin with a timestamp and server name. Treat deletion of the pin
  file as a trust reset that requires confirmation.

### H3-25 — Unauthenticated Electrum notifications and header timestamps

- **Severity:** Low
- **Location:** `internal/backend/electrum_backend.go:479-524`, `internal/backend/electrum_backend.go:1051-1088`
- **Status:** open
- **Exploit scenario:** The server can send `blockchain.scripthash.subscribe`
  and `blockchain.headers.subscribe` notifications at any time, and the wallet
  accepts header status/full headers without validating the chain or header
  length beyond 80 bytes. `getBlockTimestamp` trusts the server-provided header
  time. A malicious server can trigger spurious refreshes, display arbitrary
  block heights/timestamps, and distort confirmation times and transaction
  ordering.
- **Recommended fix:** Validate notification shapes strictly, rate-limit them,
  and treat header data as untrusted until a header chain is verified (H3-09).
  Bound notification sizes and ignore timestamps outside a plausible range.

### H3-26 — Broadcast transaction ID is not verified locally

- **Severity:** Low
- **Location:** `internal/backend/electrum_backend.go:1275-1293`, `internal/backend/rpc_backend.go:463-482`
- **Status:** open
- **Exploit scenario:** Both backends return the server's txid string after
  broadcast without comparing it to the locally computed transaction hash. A
  malicious server can return a fake txid. The UI shows it as the sent
  transaction and marks the change address as used; the user may believe a
  payment succeeded when it did not.
- **Recommended fix:** Compute `tx.TxHash()` locally from the finalized
  transaction and require the server's returned txid to match. If it does not,
  report an error and do not mark change as used.

### H3-27 — Unknown backend types silently fall back to testnet4 Electrum

- **Severity:** Low
- **Location:** `internal/backend/factory.go:16-20`
- **Status:** open
- **Exploit scenario:** `NewBackend` maps any unrecognised `Type` to a
  `DefaultElectrumConfig("testnet4")`. A config with a legacy or corrupted
  type (including `"api"`) silently connects to a testnet4 public server even
  for a mainnet wallet. The genesis check may catch the mismatch when the
  server is reachable, but until then the user may see testnet data or a
  confusing failure.
- **Recommended fix:** Return an explicit error for unknown backend types.
  Do not substitute a network-specific default. Surface the error in the UI
  and let the user choose a valid backend.

### H3-28 — Descriptor checksum mismatch is only a warning

- **Severity:** Low
- **Location:** `internal/wallet/descriptor.go:535-547`
- **Status:** open
- **Exploit scenario:** When a descriptor carries a checksum that does not
  match its body, `ParseDescriptor` appends a warning and continues parsing.
  A typo or a tampered descriptor will be accepted and may derive addresses
  the user did not intend. This compounds H3-05 for multisig.
- **Recommended fix:** Treat a checksum mismatch as a hard error. Treat a
  missing checksum as a warning that requires explicit confirmation. Verify
  the first derived address against the descriptor before creating a wallet.

### H3-29 — Electrum TLS does not set a minimum protocol version

- **Severity:** Low
- **Location:** `internal/backend/electrum_backend.go:190-207`
- **Status:** open
- **Exploit scenario:** The `tls.Config` sets `ServerName` and a pinning
  `VerifyConnection` callback but not `MinVersion`. Go's current default is TLS
  1.2, so this is not exploitable today, but an explicit floor prevents a
  future default change or a custom build from negotiating an obsolete
  protocol. The RPC backend already sets `MinVersion: tls.VersionTLS12`.
- **Recommended fix:** Set `MinVersion: tls.VersionTLS12` (or 1.3 for
  non-legacy servers) in both Electrum TLS configurations.

### H3-30 — HWI discovery trusts PATH; no timeout; PIN passed in argv

- **Severity:** Low
- **Location:** `internal/wallet/hardware.go:134`, `internal/wallet/hardware.go:296-315`, `internal/wallet/hardware.go:330`
- **Status:** open
- **Exploit scenario:** The wallet runs the first `hwi` found in `PATH` (plus
  a few fixed locations) with `exec.Command`, no context/timeout, and no
  absolute-path or signature check. A malicious `hwi` earlier in `PATH` (for
  example in a user-writable directory) runs with the user's privileges and
  can read the wallet directory or the terminal. A hung or malicious `hwi` can
  block the UI indefinitely. `SendPin` passes the hardware PIN as a command
  argument, visible in the process table to other local users; it is currently
  unused.
- **Recommended fix:** Resolve `hwi` to an absolute path, verify ownership and
  mode, and prefer a configured path. Run with a context timeout. Pass the PIN
  on stdin or through HWI's supported secure channel; never in argv. Validate
  all HWI output before rendering.

### H3-31 — Exports overwrite files without confirmation; modes not enforced

- **Severity:** Low
- **Location:** `internal/ui/models/settings.go:382`, `internal/ui/models/settings.go:397`, `internal/ui/models/settings.go:650`, `internal/ui/models/send_psbt.go:227`
- **Status:** open
- **Exploit scenario:** Descriptor, multisig JSON, Coldcard config, and PSBT
  exports write to fixed paths and overwrite silently. An existing file with
  the same name is destroyed. `os.WriteFile(..., 0600)` does not change the
  mode of an existing file, so a pre-existing 0644 export stays
  world-readable. The Coldcard export writes to `~/coldcard-multisig.txt`,
  outside `~/.hax`.
- **Recommended fix:** Refuse to overwrite without confirmation, or use a
  uniquified filename. `Chmod(0600)` after writing (the CSV export already does
  this). Keep exports inside a dedicated 0700 directory by default. Treat
  descriptor/xpub exports as privacy-sensitive and warn accordingly.

### H3-32 — Config is loaded without schema validation

- **Severity:** Low
- **Location:** `internal/config/config.go:75-93`, `internal/config/config.go:127-278`, `internal/backend/factory.go:16-20`
- **Status:** open
- **Exploit scenario:** `Load` uses plain `json.Unmarshal` with no
  `DisallowUnknownFields`, no size limit, and no call to the existing
  `ConfigManager.Validate` (which is effectively dead code outside tests). A
  hand-edited or corrupted `config.json` can contain unknown fields, invalid
  networks, out-of-range ports, or an unknown backend type. Combined with
  H3-27 and auto-lock fail-open (H3-18), a corrupted config silently changes
  behavior.
- **Recommended fix:** Call `Validate` after `Load` and on `Save`; reject or
  repair invalid values. Use `json.Decoder.DisallowUnknownFields` for the
  config (with a versioned migration path), cap the file size, and default to
  safe values on error. Remove or wire up `RawConfigJSON`.

### H3-33 — Network alias inconsistency can derive mainnet keys for a "testnet4" wallet

- **Severity:** Low
- **Location:** `internal/wallet/manager.go:626`, `internal/wallet/network.go:19-27`, `internal/ui/models/wallet_setup_network.go:27-29`
- **Status:** open
- **Exploit scenario:** `ParamsForNetwork` accepts both `"testnet"` and
  `"testnet4"`, but `UnlockWalletBytes` passes `data.Network == "testnet"` to
  `NewHDWallet`. A wallet file whose `Network` is `"testnet4"` (legacy file,
  manual edit, or an external restore tool) is loaded with *mainnet* HD
  parameters while the rest of the wallet (`getNetwork()`, PSBT, multisig,
  explorer) treats it as testnet4. Addresses and xpubs are then mainnet-encoded
  and can diverge from the addresses the user expects. Normal UI creation
  stores `"testnet"`, so the default path is unaffected.
- **Recommended fix:** Use `ParamsForNetwork(data.Network)` (or a single
  `IsMainnet` helper) consistently when constructing `HDWallet`. Normalize
  `"testnet4"` to `"testnet"` on load, or reject it with a migration message.
  Add a test that loads a `"testnet4"` wallet and checks the derived address
  prefix.

### H3-34 — Testnet3 addresses are accepted by testnet4 wallets

- **Severity:** Low
- **Location:** `internal/wallet/testnet4.go`, `internal/wallet/network.go:46-56`
- **Status:** open
- **Exploit scenario:** Testnet3 and testnet4 use the same base58 version bytes
  and the same `tb` bech32 HRP, so `ValidateAddressForNetwork` cannot
  distinguish them. A testnet3 recipient address passes validation in a
  testnet4 wallet. Funds sent to it are not spendable through the testnet4
  backend, and the user may not notice the network mismatch. This is
  testnet-only and does not affect mainnet.
- **Recommended fix:** Document the ambiguity and warn when an address was
  likely created for testnet3 (for example by checking the connected server's
  genesis or a user-selected testnet variant). Consider offering an explicit
  testnet3/testnet4 selector and validating against the selected chain.

### H3-35 — Negative PSBT fee is clamped to zero instead of rejected

- **Severity:** Low
- **Location:** `internal/wallet/psbt_manager.go:454-478`
- **Status:** open
- **Exploit scenario:** `ImportPSBTFromBase64` computes
  `fee = totalInput - totalOutput` and replaces a negative result with zero.
  A malformed PSBT whose outputs exceed its inputs is displayed with a zero
  fee rather than rejected. The inspector may warn that outputs exceed inputs,
  but the import succeeds and the user can proceed toward signing.
- **Recommended fix:** Reject PSBTs with negative fees as invalid. Report the
  discrepancy explicitly and do not populate a `PSBTResult` that can be
  signed.

### H3-36 — Terminal escape injection through server-supplied txids

- **Severity:** Low
- **Location:** `internal/ui/models/history.go:1228`, `internal/ui/models/textutil.go:41-49`
- **Status:** open
- **Exploit scenario:** Labels are passed through `sanitizeLine`, which strips
  control characters, but the transaction detail row renders `tx.TxID`
  directly. `TxID` can come from an Electrum history response (and
  `GetTransaction` does not verify it, H3-08), so a malicious server can
  include ANSI escape sequences. Rendering them can restyle the terminal,
  clear the screen, or produce misleading output; the alt screen limits but
  does not eliminate the impact.
- **Recommended fix:** Sanitize every server- and user-supplied string before
  rendering, including txids, addresses, labels, and error text. Validate
  txids as 64 hex characters at the API boundary.

### H3-37 — BIP21 sanitizer does not strip Unicode bidirectional controls

- **Severity:** Low
- **Location:** `internal/wallet/bip21.go:108-121`
- **Status:** open
- **Exploit scenario:** `sanitizeURIText` removes `unicode.IsControl` runes
  (C0/C1 controls) but not Unicode format characters such as U+202A–U+202E
  (bidirectional embedding/override) or U+2066–U+2069 (isolates). A malicious
  BIP21 label or message can visually reorder text in the confirmation screen,
  for example to make a payment description look like a different amount or
  recipient. The address itself is parsed and validated separately, so direct
  address substitution is not possible.
- **Recommended fix:** Strip Unicode category `Cf` (format) characters in
  addition to controls, or allow only a conservative set of letters, digits,
  spaces, and common punctuation in labels/messages. Render bidirectional text
  with an explicit direction where supported.

### H3-38 — UR/PSBT decoder limits exist only at the UI layer

- **Severity:** Low
- **Location:** `internal/ur/decode.go`, `internal/ur/psbt.go`, `internal/ui/models/send_keys.go:893`, `internal/ui/models/send_psbt.go:191-195`, `internal/wallet/psbt.go:1797-1804`
- **Status:** open
- **Exploit scenario:** `DecodeSinglePart`, `decodeCBORByteString`, and
  `ImportPSBT` perform bounds checks but have no explicit maximum payload
  size. The UI limits UR input to 10,000 characters and PSBT files to 10 MB,
  so the shipped paths are bounded, but any future caller (CLI, IPC, test
  harness, or a new import surface) can pass an arbitrarily large string and
  cause large allocations. `DecodeMinimal` allocates `len(encoded)/2` before
  any semantic check.
- **Recommended fix:** Enforce a maximum encoded and decoded size inside the
  `ur` and PSBT import functions themselves. Return a clear error before
  allocating. Keep the UI limits as defense in depth.

### H3-39 — Seed backup verification can be skipped

- **Severity:** Low
- **Location:** `internal/ui/models/wallet_setup_backup.go:64-78`, `internal/ui/models/wallet_setup_backup.go:106-119`
- **Status:** open
- **Exploit scenario:** After creating a wallet, the user can press `Ctrl+S`
  and then `y` to skip the word-check entirely. The warning explains the risk,
  but there is no enforced verification. A user who copied the phrase
  incorrectly (or not at all) proceeds to fund a wallet whose seed cannot
  restore it. The skip path also calls `finishCreate` without clearing the
  generated mnemonic from the model (H3-11).
- **Recommended fix:** Require the word check for newly generated wallets, or
  gate the skip behind a stronger confirmation and a persistent "backup not
  verified" warning on the dashboard. Clear the mnemonic and password fields
  after completion.

### H3-40 — Privacy leakage to Electrum servers and the system browser

- **Severity:** Low
- **Location:** `SECURITY.md` ("Privacy notes"), `internal/backend/electrum_backend.go` (scripthash subscriptions), `internal/ui/models/history.go:1379-1393`
- **Status:** open
- **Exploit scenario:** The Electrum server learns every address/scripthash
  the wallet queries, which links the wallet's entire address set and activity.
  Tor is optional, not default. The "open in explorer" action sends the txid to
  `mempool.space` through the system browser, revealing the user's IP and
  interest in that transaction. Electrum subscriptions are also a timing
  side-channel. These are disclosed in `SECURITY.md`, so this is a residual
  risk rather than a hidden flaw.
- **Recommended fix:** Offer Tor as a first-class, easy default for public
  servers, add randomized query delays, and warn before opening an explorer.
  Consider a bundled/embedded block explorer or a user-configured one. Avoid
  subscribing to addresses that are not needed for the current screen.

### H3-41 — Module-level dependency advisories remain

- **Severity:** Low
- **Location:** `go.mod:23-24`, `go.sum`
- **Status:** open
- **Exploit scenario:** `govulncheck` reports no vulnerabilities in called
  code, but three module-level advisories remain: GO-2026-6866 and
  GO-2026-6860 in `golang.org/x/net@v0.61.0` (fixed in v0.62.0), and
  GO-2026-5932 in `golang.org/x/crypto@v0.58.0` (`openpgp` unmaintained, no
  fix). The vulnerable packages are not reachable from hax today, so there is
  no confirmed exploit path. A future import or a transitive upgrade could
  make them reachable.
- **Recommended fix:** Bump `golang.org/x/net` to v0.62.0 or later. Confirm
  that `golang.org/x/crypto/openpgp` is not imported (it is not) and consider
  replacing any future use. Keep Dependabot enabled (it is) and run
  `govulncheck` in CI (it does).

### H3-42 — Release pipeline gaps

- **Severity:** Low
- **Location:** `.github/workflows/release.yml`, `.goreleaser.yaml:51-62`, `Makefile:50-56`
- **Status:** open
- **Exploit scenario:** The release workflow triggers directly on a `v*` tag
  push and does not depend on the CI workflow succeeding. A tag can be created
  from a commit that fails `go vet`, race tests, or `govulncheck`, and the
  release job will still build and sign it. The goreleaser binary is selected
  with the floating constraint `~> v2`, and `cosign-installer` is pinned by
  action SHA but installs the latest cosign release by default, so the exact
  signing tool version is not reproducible. No SBOM is produced. The
  `Makefile` `checksums` target generates unsigned checksums, which can be
  mistaken for release checksums.
- **Recommended fix:** Gate the release job on a successful CI run (for
  example `workflow_run` or a required check), pin the goreleaser and cosign
  versions, and generate an SBOM (Syft or `go version -m` based). Label the
  Makefile checksums as development-only or sign them. Consider requiring a
  protected tag/environment approval before publishing.

### H3-43 — Wallet format version and salt/nonce lengths are not validated

- **Severity:** Low
- **Location:** `internal/wallet/storage.go:148-154`, `internal/wallet/storage.go:246-278`
- **Status:** open
- **Exploit scenario:** `LoadWalletKey` unmarshals `EncryptedWalletFile` and
  ignores `Version`; it passes `Salt` and `Nonce` directly to PBKDF2 and GCM
  without checking their lengths. A truncated or hand-edited file with an
  empty salt is accepted by PBKDF2 (using an empty salt), and a wrong nonce
  length fails only at `gcm.Open`. An attacker who can modify the file cannot
  decrypt it without the password, but confusing error paths and future format
  migrations are harder to reason about. The version field is not
  authenticated.
- **Recommended fix:** Require `Version == 1` (or a supported set), a 32-byte
  salt, and a 12-byte nonce before deriving the key. Return a specific
  "unsupported wallet version" error and never silently reinterpret the file.
  Include the format version in the GCM AAD.

### H3-44 — Wallet metadata is not bound as AEAD associated data

- **Severity:** Low
- **Location:** `internal/wallet/storage.go:205-215`, `internal/wallet/storage.go:359`
- **Status:** open
- **Exploit scenario:** `gcm.Seal(nil, nonce, plaintext, nil)` and
  `gcm.Open(nil, nonce, ciphertext, nil)` use no additional authenticated
  data. The outer `Version`, `Salt`, and `Nonce` are not authenticated.
  Changing `Version` is undetected (H3-43), and a future format that relies on
  metadata could be confused. The ciphertext itself is authenticated, so this
  is not a direct plaintext-forgery issue.
- **Recommended fix:** Pass a canonical encoding of the format version (and
  any wallet identity that should be bound) as AAD to both `Seal` and `Open`.
  Reject unknown versions before decryption.

### H3-45 — Dead `testnet` field with a misleading "always testnet" comment

- **Severity:** Info
- **Location:** `internal/ui/models/root.go:92`, `internal/ui/models/root.go:209`
- **Status:** open
- **Exploit scenario:** `RootModel.testnet` is set to `true` with the comment
  "Always testnet for safety" but is never read. Mainnet wallets are fully
  supported and selected in wallet setup. A future reader could mistake the
  dead field for an enforced safety control and weaken a real check.
- **Recommended fix:** Remove the field and comment, or wire it to an actual
  policy if one is intended.

### H3-46 — `RawConfigJSON` is unused but would expose the RPC password if surfaced

- **Severity:** Info
- **Location:** `internal/config/config.go:290-296`
- **Status:** open
- **Exploit scenario:** `RawConfigJSON` marshals the whole config, including
  `backend.rpc.password`, and is currently unused. If a future UI or debug
  feature displays or logs it, the RPC password is disclosed. The same risk
  applies to any generic config dump.
- **Recommended fix:** Remove the method or redact secret fields before
  marshaling. Add a `Redacted()` representation for logging and UI display.

### H3-47 — Custom TestNet4 parameters have no checkpoints

- **Severity:** Info
- **Location:** `internal/wallet/testnet4.go`
- **Status:** open
- **Exploit scenario:** `TestNet4Params` is a hand-written `chaincfg.Params`
  (magic `0x1C163F28`, testnet address prefixes, coin type 1, genesis hash,
  proof-of-work limit, deployments). It defines no checkpoints. The wallet does
  not perform header-chain validation (H3-09), so checkpoints would not be used
  today, but the custom parameters deserve independent review against Bitcoin
  Core's testnet4 definition as the network matures.
- **Recommended fix:** Add a test that compares the parameters against a
  canonical source (Bitcoin Core's `chainparams.cpp`), and add checkpoints if
  header validation is implemented. Document the provenance of the genesis and
  magic values.

### H3-48 — Transactions always signal RBF

- **Severity:** Info
- **Location:** `internal/wallet/psbt.go:438`
- **Status:** open
- **Exploit scenario:** `assemblePSBT` sets `Sequence = 0xfffffffd` on every
  input, making every transaction replaceable by default. This is intentional
  (it enables the fee-bump feature) and is generally desirable, but it means a
  recipient cannot rely on finality before confirmation, and a third party can
  attempt a replacement. This is a design property, not a vulnerability.
- **Recommended fix:** Document the behavior and consider an opt-out for
  recipients that require non-replaceable transactions. Keep the fee-bump
  checks (H3-02) strict.

### H3-49 — No automatic wallet backup or recovery reminder

- **Severity:** Info
- **Location:** `internal/wallet/backup.go`, `internal/ui/models/wallet_setup_backup.go`
- **Status:** open
- **Exploit scenario:** The wallet encrypts and stores the seed in one file and
  relies on the user's written seed phrase for recovery. There is no automatic
  encrypted backup, no reminder to verify the backup later, and no warning if
  the wallet file is the only copy. A disk failure after creation loses the
  wallet unless the seed was recorded correctly (and H3-39 allows skipping the
  check).
- **Recommended fix:** Offer an optional encrypted backup export (with a
  separate password), a persistent "backup verified" state, and a reminder on
  the dashboard until verification is complete. Document the recovery model
  clearly.

## Discarded false positives

The following were investigated and are **not** reported as vulnerabilities:

- **Native SegWit signatures are not unprotected.** `signSegWitInput` uses
  btcd's `updater.Sign`, which rebuilds the P2WPKH script from the pubkey and
  compares it to `WitnessUtxo.PkScript`; btcd's sighash code expands P2WPKH to
  the correct P2PKH scriptCode. The nested P2SH-P2WPKH path performs an
  explicit script-hash check.
- **Local legacy PSBT creation verifies previous transactions.**
  `attachPrevTxs` fetches the full previous transaction for non-witness inputs,
  verifies the txid and output value/script, and fails closed. H3-04 applies to
  *imported* PSBTs, not the local send path.
- **Cosigner xpub network validation exists.** `CreateMultisigWallet` calls
  `key.IsForNet(params)` for every cosigner; the earlier validation layer is
  permissive, but the final check is enforced.
- **The fee-bump UI does not need a separate cap.** Custom fee input is
  validated by the same `send_keys.go` soft/hard caps before `checkBumpRate`,
  so the fee-bump path inherits the custom-fee limit. The missing cap is on
  backend-supplied preset estimates (H3-02).
- **BIP48 script indexes 0/1/2 are correct.** Legacy = 0, nested = 1, native =
  2 matches BIP48; no funds-loss finding.
- **No `InsecureSkipVerify` and no shell invocation.** Electrum and RPC TLS
  verify the peer; HWI and the browser opener use argv, not a shell.
- **`debug.log` is created 0600.** Bubble Tea's `LogToFile` uses
  `os.OpenFile(..., 0600)`, so the earlier suspicion of world-readable debug
  logs was incorrect. The remaining issue is privacy-sensitive content
  (H3-22), not file mode.

## What is already done well

- **Entropy:** `crypto/rand` only; 128/256-bit BIP39 entropy; entropy buffer
  wiped after mnemonic generation; no `math/rand` in non-test code.
- **Encryption:** AES-256-GCM, 32-byte random salt per wallet save, fresh
  12-byte random nonce per encryption, authenticated decryption, key and
  plaintext byte slices wiped after use.
- **File modes:** wallet directory 0700, wallet/config/pin files 0600 at
  creation; CSV export explicitly `Chmod`s 0600.
- **Key lifecycle:** `LockWallet` stops background work, waits for in-flight
  persistence, wipes the session key and salt, drops data/wallet/cache
  references, and resets wallet state. `UnlockWallet` wipes the password copy.
- **Secrets not logged:** no mnemonic, seed, xprv, password, or passphrase was
  found in any log or error message.
- **Clipboard:** the seed phrase is deliberately not copyable; only public or
  transaction data is copied (the missing auto-clear is H3-12).
- **Password input:** all password/passphrase fields use `EchoPassword`.
- **Address validation:** strict `DecodeAddress` + `IsForNet` per network;
  wallet-name regex and `filepath.Rel` prevent path traversal; BIP21 parsing
  validates the address against the wallet network and strips control
  characters.
- **Derivation:** BIP44/49/84/86 purposes, coin type 0 for mainnet and 1 for
  testnet, correct hardened/non-hardened path structure, BIP67 sorting for
  `sortedmulti`, BIP48 script indexes.
- **PSBT inspection:** strong, independent inspection that recomputes
  ownership from wallet keys, cross-checks previous transactions against
  outpoints and witness UTXOs, detects external outputs, spoofed change, dust,
  high fee rates, high fee share, and non-`SIGHASH_ALL` flags.
- **Signing policy:** ECDSA signatures force `SIGHASH_ALL`; Taproot uses
  `SIGHASH_DEFAULT` and clears the PSBT sighash field. The signer checks that
  derived keys match the scripts it signs (except the multisig script-hash gap
  in H3-21).
- **Local transaction construction:** `attachPrevTxs` fetches and verifies
  previous transactions for legacy inputs; `fetchPrevTx` verifies the txid;
  `decodeRawTx` verifies the txid for RPC/raw transaction paths.
- **Fees and dust:** custom fee input has a 500 sat/vB soft cap with typed
  confirmation and a 10,000 sat/vB hard cap; recipient dust is rejected in the
  UI; change dust is absorbed with a visible warning.
- **Electrum transport:** TLS peer verification with a TOFU SPKI pin, hard
  failure on pin mismatch, `Trust` checks the old pin, 10s dial / 30s request /
  60s idle timeouts, panic recovery in the read loop, and bounded notification
  channels.
- **Bitcoin Core:** TLS 1.2 minimum when enabled, 30s client timeout, Basic
  Auth, no `InsecureSkipVerify`, txid verification via `decodeRawTx`.
- **UR/QR:** CRC32 verification and CBOR bounds checks; no BBQr decoder
  (encode only); QR generation only; UI payload caps.
- **Bitcoin amounts:** exact decimal parsing with `MaxMoney` and 8-decimal
  limit; no float rounding.
- **Release hardening:** all GitHub Actions pinned by commit SHA,
  `persist-credentials: false`, least-privilege permissions, keyless cosign
  signature over `checksums.txt`, SLSA build provenance, draft releases,
  `-trimpath`, `-buildid=`, `mod_timestamp`, CGO disabled.
- **CI:** `go vet`, race tests, golden UI tests, and `govulncheck` run on every
  push/PR; Dependabot is configured for Go modules and Actions.
- **No phone-home:** fee estimates come only from the configured backend; no
  third-party HTTP fee API; no auto-update mechanism.
- **HWI:** invoked with argv (no shell), PIN format validated to digits.
- **Backups/exports:** seed phrases and private keys are excluded from backup
  and multisig export formats (only xpubs/descriptors/labels).

## Summary table

| ID | Severity | Area | Location | Status |
|---|---|---|---|---|
| H3-01 | High | Crypto / storage | `internal/wallet/storage.go:21-24,280-282`; `internal/wallet/validation.go:10,38-42` | open |
| H3-02 | High | Fees / backend | `internal/ui/models/send.go:934-942,1051-1061`; `internal/backend/electrum_backend.go:1303-1319`; `internal/backend/rpc_backend.go:418-460` | open |
| H3-03 | High | PSBT signing | `internal/ui/models/send_keys.go:801-842`; `internal/wallet/psbt_inspect.go:62-174` | open |
| H3-04 | High | PSBT / legacy inputs | `internal/wallet/psbt_signer_data.go:18-34`; `internal/wallet/psbt.go:686,1159-1204`; `internal/wallet/psbt_manager.go:443-478` | open |
| H3-05 | High | Descriptors / multisig | `internal/wallet/descriptor.go:551,656-730`; `internal/wallet/hdwallet.go:729-737,763-822` | open |
| H3-06 | Medium | UTXO trust | `internal/wallet/tx_sender.go:18-68`; `internal/wallet/psbt_signer_data.go:82-115` | open |
| H3-07 | Medium | Electrum DoS | `internal/backend/electrum_backend.go:416-444,459` | open |
| H3-08 | Medium | Electrum integrity | `internal/backend/electrum_backend.go:1090-1176` | open |
| H3-09 | Medium | SPV / network trust | `internal/wallet/network_check.go:29-55,73-87`; `internal/backend/genesis.go` | open |
| H3-10 | Medium | Unlock / brute force | `internal/ui/models/unlock.go:113-115,180-190` | open |
| H3-11 | Medium | Memory handling | `internal/ui/models/wallet_setup.go:92-100,544,627,989`; `internal/ui/models/wallet_setup_backup.go:106-119`; `internal/ui/models/root.go:309-313` | open |
| H3-12 | Medium | Clipboard | `internal/ui/models/addresses.go:222`; `internal/ui/models/receive.go:307`; `internal/ui/models/settings.go:353,553`; `internal/ui/models/send_keys.go:790,923,971` | open |
| H3-13 | Medium | RPC credentials | `internal/ui/models/connection_setup.go:586-593`; `internal/backend/rpc_backend.go:125-132`; `internal/backend/config.go:30-34` | open |
| H3-14 | Medium | RPC URL injection | `internal/backend/rpc_backend.go:176`; `internal/config/config.go:247-251` | open |
| H3-15 | Medium | RPC DoS | `internal/backend/rpc_backend.go:140` | open |
| H3-16 | Medium | Storage durability | `internal/wallet/storage.go:220-228`; `internal/config/config.go:109`; `internal/backend/pinning.go:68-80` | open |
| H3-17 | Medium | File permissions | `internal/wallet/storage.go:158-166,222`; `internal/config/config.go:99,109` | open |
| H3-18 | Medium | Auto-lock | `internal/ui/models/autolock.go:20-36,41-55,59-87` | open |
| H3-19 | Medium | Cosigner seeds | `internal/wallet/cosigner_manager.go:259-330,407-426`; `internal/ui/models/settings_multisig.go:337-363` | open |
| H3-20 | Medium | Electrum transport | `internal/backend/electrum_backend.go:216-223`; `internal/ui/models/connection_setup.go:542` | open |
| H3-21 | Medium | Multisig signing | `internal/wallet/psbt.go:1206-1390` | open |
| H3-22 | Low | Debug logging | `cmd/hax/main.go:24-31`; `internal/wallet/psbt.go:470-478`; `internal/backend/electrum_backend.go:490-500` | open |
| H3-23 | Low | Panic handling | `cmd/hax/main.go:90-100`; `internal/ui/models/root.go:235` | open |
| H3-24 | Low | TOFU | `internal/backend/pinning.go:95-118` | open |
| H3-25 | Low | Electrum notifications | `internal/backend/electrum_backend.go:479-524,1051-1088` | open |
| H3-26 | Low | Broadcast integrity | `internal/backend/electrum_backend.go:1275-1293`; `internal/backend/rpc_backend.go:463-482` | open |
| H3-27 | Low | Backend factory | `internal/backend/factory.go:16-20` | open |
| H3-28 | Low | Descriptor checksum | `internal/wallet/descriptor.go:535-547` | open |
| H3-29 | Low | TLS configuration | `internal/backend/electrum_backend.go:190-207` | open |
| H3-30 | Low | HWI | `internal/wallet/hardware.go:134,296-315,330` | open |
| H3-31 | Low | Exports | `internal/ui/models/settings.go:382,397,650`; `internal/ui/models/send_psbt.go:227` | open |
| H3-32 | Low | Config parsing | `internal/config/config.go:75-93,127-278` | open |
| H3-33 | Low | Network alias | `internal/wallet/manager.go:626`; `internal/wallet/network.go:19-27` | open |
| H3-34 | Low | Testnet ambiguity | `internal/wallet/testnet4.go`; `internal/wallet/network.go:46-56` | open |
| H3-35 | Low | PSBT fee | `internal/wallet/psbt_manager.go:454-478` | open |
| H3-36 | Low | Terminal injection | `internal/ui/models/history.go:1228` | open |
| H3-37 | Low | BIP21 text | `internal/wallet/bip21.go:108-121` | open |
| H3-38 | Low | Decoder limits | `internal/ur/decode.go`; `internal/ur/psbt.go`; `internal/wallet/psbt.go:1797-1804` | open |
| H3-39 | Low | Backup flow | `internal/ui/models/wallet_setup_backup.go:64-78,106-119` | open |
| H3-40 | Low | Privacy | `internal/ui/models/history.go:1379-1393`; `SECURITY.md` | open |
| H3-41 | Low | Dependencies | `go.mod:23-24` | open |
| H3-42 | Low | Release pipeline | `.github/workflows/release.yml`; `.goreleaser.yaml:51-62`; `Makefile:50-56` | open |
| H3-43 | Low | Wallet format | `internal/wallet/storage.go:148-154,246-278` | open |
| H3-44 | Low | AEAD metadata | `internal/wallet/storage.go:205-215,359` | open |
| H3-45 | Info | Dead field | `internal/ui/models/root.go:92,209` | open |
| H3-46 | Info | Config redaction | `internal/config/config.go:290-296` | open |
| H3-47 | Info | Testnet4 params | `internal/wallet/testnet4.go` | open |
| H3-48 | Info | RBF policy | `internal/wallet/psbt.go:438` | open |
| H3-49 | Info | Backup policy | `internal/wallet/backup.go`; `internal/ui/models/wallet_setup_backup.go` | open |

## Dependency scan detail

Command:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Result:

```text
=== Symbol Results ===
No vulnerabilities found.

Your code is affected by 0 vulnerabilities.
This scan also found 0 vulnerabilities in packages you import and 3
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
```

Module-level advisories:

| Advisory | Module | Found | Fixed | Reachable |
|---|---|---|---|---|
| GO-2026-6866 | `golang.org/x/net` | v0.61.0 | v0.62.0 | No |
| GO-2026-6860 | `golang.org/x/net` | v0.61.0 | v0.62.0 | No |
| GO-2026-5932 | `golang.org/x/crypto` | v0.58.0 | N/A | No |

`go.mod` declares `go 1.26.10`; the review used Go 1.27.3. No unsupported Go
version or language-feature issue was found. The three module advisories are
tracked as H3-41.

## Suggested remediation order

1. **H3-02, H3-03, H3-04** — close the fund-loss paths first: cap backend
   fees, gate signing on inspection warnings, and verify legacy input amounts.
2. **H3-01, H3-10** — upgrade the KDF to Argon2id, raise the password policy,
   and add unlock throttling.
3. **H3-05, H3-21** — fix descriptor key ordering and multisig script
   verification before advertising multisig interoperability.
4. **H3-06, H3-07, H3-08, H3-09** — harden the Electrum trust boundary and
   response handling.
5. **H3-11 through H3-20** — reduce secret lifetime, clear the clipboard, fix
   RPC transport/URL handling, make writes durable, tighten permissions, fix
   auto-lock, and require explicit cosigner-seed consent.
6. **H3-22 through H3-44** — hygiene, defense in depth, and release
   reproducibility.

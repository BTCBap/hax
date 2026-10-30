# hax v1.4 Roadmap

**Status:** Planning draft. Documentation only; this file changes no code.
**Baseline:** hax v1.2.0 plus the unreleased work in this tree (`CHANGELOG.md`).
**Sources:** `README.md`, `TODO.md`, `CHANGELOG.md`, `docs/`, `cmd/`, and `internal/` in this tree.
**Date:** 2026-10-30.

hax already has the hard wallet primitives: HD accounts across four script types, PSBT creation and signing, hardware wallets, multisig, coin control, fee bumping, Electrum and Bitcoin Core backends, encrypted storage, and labels. The v1.4 opportunity is the last mile: make those capabilities visible, safe, and pleasant to use, and close the interoperability gaps that show up when a real user moves between hax, a hardware signer, and another wallet.

This roadmap is deliberately conservative about new protocol surface. PSBTv2, silent payments, MuSig2, miniscript, payjoin, and BIP47 are assessed here and scheduled after v1.4. The release should ship a smaller number of complete, tested improvements.

**Legend**

| Label | Meaning |
|---|---|
| **Effort S** | One sitting to about two days. |
| **Effort M** | About one focused week, including tests. |
| **Effort L** | Multi-week, cross-cutting, or dependent on external device support. |
| **P0** | v1.4 must-ship. |
| **P1** | v1.4 should-ship; can slip to v1.5 with a written reason. |
| **P2** | Opportunistic; take it when it lands naturally in touched code. |
| **P3** | Later. Keep the design compatible; do not build it in v1.4. |

Support status in this document is **Shipped**, **Partial** (partially exists in the tree), or **Absent**. Citations use `path:line` and refer to this tree.

---

## 1. Summary and top 10 priorities

### 1.1 Where hax stands

- **Accounts and derivation:** BIP32/BIP39 with BIP44, BIP49, BIP84, and BIP86 accounts. Taproot uses the BIP86 path (`internal/wallet/hdwallet.go:104`) with key-path internal and output keys (`internal/wallet/hdwallet.go:967`, `:983`).
- **Passphrase:** optional BIP39 passphrase on create and restore (`internal/wallet/manager.go:233`, `:305`), stored inside the encrypted wallet (`internal/wallet/storage.go:33`).
- **PSBT:** BIP174 v0 create, inspect, sign, finalize, export, and import (`internal/wallet/psbt.go:331`, `:513`, `:1430`, `:1789`, `:1798`), including BIP371 Taproot fields for hardware signing.
- **Hardware and multisig:** HWI device detection, address verification, and signing (`internal/wallet/hardware.go`); BIP48 P2SH, P2SH-P2WSH, and P2WSH multisig (`internal/wallet/multisig.go`); fixed-script descriptor import and export (`internal/wallet/descriptor.go:213`, `:526`).
- **Coin control and fees:** Branch-and-Bound then largest-first selection (`internal/wallet/transaction.go:116`, `:184`, `:288`), frozen coins (`internal/ui/models/utxos.go:213`), send-all, batch payments, RBF and CPFP (`internal/wallet/fee_bump.go:298`, `:455`), and a pre-sign PSBT inspector (`internal/wallet/psbt_inspect.go:66`).
- **Backends:** Electrum over TLS with Tor SOCKS5 support, and Bitcoin Core JSON-RPC with `scanblocks` history (`internal/backend/rpc_backend.go:914`). Electrum certificates use trust-on-first-use SHA-256 SPKI pins with an explicit mismatch error (`internal/backend/pinning.go:15`).
- **Labels and import/export:** BIP329 export and import in the wallet layer (`internal/wallet/label_manager.go:131`, `:189`), history export key `E` (`internal/ui/models/history.go:485`), Koinly CSV (`internal/ui/models/history.go:518`), and descriptor export with UR (`internal/wallet/descriptor.go:286`).
- **Storage:** AES-256-GCM with PBKDF2-SHA256 at 100,000 iterations (`internal/wallet/storage.go:23`, `:338`), `0600` files in a `0700` directory.
- **UI:** Bubble Tea screens for splash, connection setup, wallet setup, unlock, dashboard, receive, send, history, addresses, UTXOs, settings, and help (`internal/ui/models/root.go:32`). Layout stability is an explicit project rule and is covered by tests (`internal/ui/models/layout_stability_test.go`).

### 1.2 The v1.4 bet

Ship the missing inspection and safety layers around the existing engine:

1. A user can see exactly what a transaction did after it confirms.
2. Labels move in both directions and attach to the coins they describe.
3. Coin selection considers privacy, and the wallet explains the linkability it created.
4. Fee bumping lives on the pending transaction itself.
5. Confirmation and warning flows preserve context through focused trays.
6. A backup can be exported and verified without leaving the terminal.

### 1.3 Top 10 priorities

| # | Priority | Item | Why now | Effort |
|---|---|---|---|---|
| 1 | P0 | **Transaction viewer** for confirmed and pending transactions | The PSBT inspector covers pre-sign review (`psbt_inspect.go:66`); history detail stops at summary fields. Users need inputs, outputs, addresses, script types, fee, vsize, and linkage after the fact. This is the largest Sparrow parity gap. | M |
| 2 | P0 | **BIP329 import UI and complete label coverage** | Import exists in the wallet layer (`label_manager.go:189`) with no screen; history only exports (`history.go:485`). Add a paste/file import flow, a summary tray, and `input` records for spend attribution. | S–M |
| 3 | P0 | **Privacy-aware coin selection and a transaction privacy check** | BnB and largest-first optimize fee and shape (`transaction.go:116`). Add address-reuse avoidance, script-type consistency, unconfirmed-change handling, and a review warning that names the linkage created. | M |
| 4 | P0 | **Inline RBF on pending Activity rows** | The RBF/CPFP engine is complete (`fee_bump.go:298`, `:455`) and currently reached through history key `b` (`history.go:436`). Put the affordance on the pending row and open a bump tray anchored there. | S–M |
| 5 | P0 | **Tray interaction system, Continue→Confirm, and layout-stability extensions** | Confirmations and warnings currently use screen-level states. Focused trays preserve context, make progression visible, and give every rare screen the same polish. Extend `layout_stability_test.go` for each new flow. | M |
| 6 | P1 | **Stealth mode** | A terminal is a shoulder-surfing surface. One key hides balances and amounts, with a short shimmer and a clear indicator; confirmation screens always show real values. | S |
| 7 | P1 | **Unified backup export and verification celebration** | `ExportUnifiedBackup` exists (`internal/wallet/backup.go:209`) with no UI. Add descriptor, config, and labels export, a restore-check flow, and a small completion moment. | S–M |
| 8 | P1 | **UTXO screen upgrade** | Current columns are Date, Output, Label, and Value (`internal/ui/models/utxos.go:60`). Add address, confirmation status, script type, filter/search, and freeze toggling inside the send UTXO step. | M |
| 9 | P1 | **Multi-part UR send and receive** | UR encoding is single-part (`internal/ur/psbt.go:11`, `internal/ur/ur.go:21`); BBQr already animates multiple frames (`internal/ui/models/send_psbt.go:438`). Add fountain encoding and a multi-frame decoder for QR-only signers. | M |
| 10 | P1 | **Fee and mempool view** | Fee choices are static targets; the backend already exposes estimates. Add a fee histogram from the Electrum server or a sampled summary from Core's mempool, target selection, and a clear source label. No third-party HTTP APIs. | M |

### 1.3.1 Delivered on the v1.4 design branch

| Item | Status | Notes |
|---|---|---|
| 1. Transaction viewer | **Done (visualizer).** | Flow view (inputs → fee junction → outputs; markers for yours/change/external/fee/OP_RETURN, line weight by value, detail panel) opened with `v` from send Review, the PSBT inspector, and History. |
| 2. BIP329 import UI | **Done.** | Settings screen: path → preview counts → result, 10 MiB limit. `input` record coverage for spend attribution not added. |
| 3. Privacy coin selection | **Not started** | Protocol and selection work, out of scope for the design branch. |
| 4. Inline RBF | **Done.** | ⇡ marker on pending outgoing History rows; `b` opens a speed-up tray (Fastest / 30 min / 1 hour / Custom), signing still goes through review. |
| 5. Trays, Continue→Confirm, layout stability | **Done.** | Two-stage trays with fixed height; send hints morph from "enter continue" to "enter confirm & broadcast" on the final step with reserved width; layout_stability and 80x24 tests for every new screen. |
| 6. Stealth mode | **Done.** | `z` masks amounts keeping units; review/PSBT/bump/visualizer screens show real values; "◌ hidden" badge. No shimmer animation. |
| 7. Unified backup export | **Partial.** | Settings export of descriptors plus labels to an owner-only file; restore-check flow and completion moment not added. |
| 8. UTXO upgrade | **Partial.** | Summary line (total, mempool, frozen), Value/Age/Confs/Address/Label/frozen columns, sort keys, trailing helper; script type column and filter/search not added. |
| 9. Multi-part UR | **Not started** | |
| 10. Fee view | **Partial.** | Fees screen (`F` from the Dashboard) with tiered estimates from the selected backend only, ETA, source label, refresh; no mempool histogram. |
| Also delivered (not in top ten) | **Done.** | Hardware-wallet on-device address verification panel (`v` in Receive/Addresses; match/mismatch with differing groups highlighted); passphrase badge on the dashboard; grey helper text right of the selected item across menus; progress percentages derived from completed steps (never 100% early, monotonic, fixed width). |

### 1.4 Also planned (P1–P2, outside the top ten)

- Descriptor export for the selected account; the settings path always uses `accounts[0]` (`internal/ui/models/settings.go:566`).
- BIP322 message signing for address ownership proofs.
- Argon2id wallet-key derivation: **Done in v1.3.** Version-2 envelope, Argon2id with time=3, 64 MiB, and 4 threads, bounded parameters on read, and PBKDF2 v1 files migrated on unlock (`storage.go:23`).
- Bitcoin Core cookie authentication, alongside the current user/password form.
- Configurable or disabled block explorer links; history currently opens mempool.space (`internal/ui/models/history.go:1379`).
- Correct mainnet derivation paths in the settings account card (`internal/ui/models/settings_views.go:298`).
- BIP129 BSMS import/export for multisig coordination.
- Hardware signing progress that names which cosigner signed (`CountPSBTSigners`, `internal/wallet/psbt.go:187`).

### 1.5 Deferred to after v1.4

PSBTv2, silent payments (BIP352), payjoin (BIP77/78), BIP47 PayNym, MuSig2/FROST, miniscript policies, BIP388 wallet policies, BIP85, SLIP-39/codex32, v3 TRUC and package relay, and a native BIP158 client. Section 2 explains each decision.

---

## 2. Modern BIPs assessment

The table answers four questions for each proposal: what hax does today, whether it fits this product, why, and what it costs. Priority reflects v1.4, so a low priority can still be a good long-term idea.

| BIP / proposal | Current hax support | Fit | Why | Effort | Priority |
|---|---|---|---|---|---|
| **BIP 86** — Taproot single-sig | **Shipped.** BIP86 path (`internal/wallet/hdwallet.go:104`), P2TR addresses, key-path signing, `tr(...)` descriptors (`descriptor.go:526`). | Yes | This is the modern default for single-sig. Keep it as the default new-wallet script type and focus effort on Taproot multisig later. | — | Maintain |
| **BIP 352** — Silent payments | **Absent.** | Later | Receiver scanning needs tweaked-key derivation and per-output matching; the Electrum and Core interfaces in hax expose address history, so a full scanner is a new indexing surface. High complexity for a terminal wallet with a small user base. | L | P3 |
| **BIP 329** — Labels export/import | **Partial.** Export and import for `tx`, `addr`, and `output` in the wallet layer (`label_manager.go:131`, `:189`). History exposes export only (`history.go:485`). `input` records are skipped on import; `output` records are emitted only for frozen coins. | Yes | Labels are the wallet's memory. Import is the missing half, and `input` records let hax attribute spends to the receiving address. This is the cheapest high-value interoperability win. | S–M | P0 |
| **BIP 380–386** — Output descriptors | **Partial.** Checksums (`descriptor.go:16`), export (`:213`), UR export (`:286`), and a fixed parser (`:526`) for `pkh`, `wpkh`, `sh(wpkh)`, `tr`, and `wsh`/`sh(wsh)` `multi`/`sortedmulti`. The parser strips `/<0;1>/*` and handles fixed shapes; generic expressions, multipath (BIP389), and miniscript are absent. | Yes | Descriptors are the interchange format for watch-only wallets and hardware signers. Export already works; complete the parser and export all accounts so imports from Core and Sparrow round-trip. | M | P1 |
| **BIP 388** — Wallet policies | **Absent.** | Later | Useful for registering multisig policies on hardware that supports them, but it depends on device firmware and a miniscript-aware descriptor layer. Build after the generic descriptor parser. | L | P3 |
| **BIP 174 / BIP 370** — PSBTv0 and PSBTv2 | **PSBTv0 shipped** (`psbt.go:331`, `:513`, `:1430`, `:1789`, `:1798`), including BIP371 fields. **PSBTv2 absent.** | Yes for v0; later for v2 | The HWI, UR, and BBQr ecosystem in hax's target devices is v0-first. PSBTv2 adds construction flexibility with little user-visible benefit today. Revisit when a supported device or coordinator emits v2. | L | P3 |
| **BIP 21 + BIP 321** — Payment URIs | **BIP21 partial.** `ParseBIP21` reads address, `amount`, `label`, and `message`; unknown parameters such as `lightning` and `pj` are ignored; `req-` parameters invalidate the URI (`internal/wallet/bip21.go:32`). **BIP321 absent.** | Yes for BIP21; later for BIP321 | BIP21 is the common clipboard format and already works. BIP321 adds reusable payment codes and routing hints. Accepting unknown parameters already prevents breakage; surfacing `pj` or `lightning` can wait until those features exist. | S–M | P2 |
| **BIP 125** — RBF and CPFP | **Shipped.** BIP125 rule checks (`fee_bump.go:224`), RBF builder (`:298`), CPFP builder (`:455`), sequence `0xfffffffd` in created PSBTs (`psbt.go:438`). | Yes | This is a core safety feature. v1.4 moves the entry point onto the pending transaction and improves the confirmation tray. | S | P1 |
| **BIP 431** — v3 TRUC and package relay | **Absent.** | Later | v3 is aimed at Lightning and package fee-bumping. A single-wallet send rarely needs it, and a backend must expose package submission. Add only when the fee-bump path can submit and track packages end to end. | M–L | P3 |
| **BIP 85** — Deterministic entropy | **Absent.** | Later | Deriving multiple wallets or applications from one seed is useful, but it adds a second recovery surface and a new set of test vectors. Low demand relative to backup export. | S–M | P3 |
| **BIP 39 passphrase** | **Shipped.** Create and restore with double-entry confirmation (`manager.go:233`, `:305`; `wallet_setup.go:487`); stored encrypted (`storage.go:33`). No persistent passphrase indicator. | Yes | Keep the optional passphrase. Add a persistent "passphrase attached" indicator and a derived-fingerprint check at unlock, because a wrong passphrase opens a different, empty wallet. | S | P1 |
| **BIP 47** — PayNym | **Absent.** | Later | Requires notification transactions, a scanning strategy, and a public directory. That conflicts with hax's selected-server-only posture and adds a large privacy surface for low adoption. | L | P3 |
| **Payjoin** — BIP 77 and BIP 78 | **Absent.** | Later | A strong privacy improvement, but receiving needs an HTTP endpoint and asynchronous BIP77 coordination is still maturing. Build after the privacy heuristics show users where payjoin helps. | L | P3 |
| **MuSig2 (BIP 327) / FROST** | **Absent.** Listed as "Someday" (`TODO.md:39`, `:40`). | Later | Taproot script-path multisig is the prerequisite. MuSig2 adds nonce management and device coordination; FROST is a separate threshold-signature protocol. Both are large projects with thin terminal demand. | L | P3 |
| **Miniscript** | **Absent.** Listed as "Someday" (`TODO.md:38`). | Later | A policy compiler and miniscript-aware PSBT analysis need a generic descriptor parser and a much larger validation surface. Useful after BIP380–386 completeness. | L | P3 |
| **BIP 322** — Message signing | **Absent.** | Yes | Cheap ownership proofs and a standard way to sign with an address. Reuses existing derivation; needs careful message hashing and device support for legacy and nested SegWit. | S–M | P2 |
| **BIP 129** — BSMS multisig setup | **Absent.** hax has multisig config JSON export (`backup.go:104`) and descriptor import. | Yes | A standard encrypted setup exchange fits hax's existing multisig flow and improves coordinator interoperability. | M | P2 |
| **BIP 93 codex32 / SLIP-39** | **Absent.** | Later / optional | Both add seed-backup formats with new recovery rules. BIP39 remains the norm. Ship codex32 first if advanced users ask; SLIP-39 later. | M–L | P3 |
| **BIP 324** — v2 transport | **Absent.** | No for v1.4 | hax talks to Electrum servers over TLS and to Bitcoin Core over JSON-RPC. BIP324 applies to direct peer-to-peer connections, which hax does not make. Revisit only with a P2P backend. | L | P3 |
| **BIP 158** — Compact block filters | **Partial.** Bitcoin Core history uses `scanblocks` with `-blockfilterindex=1` (`rpc_backend.go:28`, `:914`); a missing index produces a clear limited-history warning. No native filter client. | Later | A native client would enable trustless history without an indexer, but it needs header sync, filter download, and a persistence layer. Core already serves node users. Treat it as a separate light-client project. | L | P3 |

### 2.1 Reading the table

The v1.4 investment is concentrated in four rows: BIP329, BIP380–386, BIP39 passphrase safety, and BIP322. BIP125 and BIP86 are already shipped and only need interface work. Everything that adds a new cryptographic protocol or a new peer-to-peer surface stays on the later list.

Two rows deserve a design note:

- **Descriptors:** complete the parser before adding BSMS or BIP388. A generic descriptor AST also unlocks miniscript, multipath, and policy display without three separate parsers.
- **Labels:** BIP329 input records are the bridge between labels and privacy analysis. Implement the record type and the spend attribution together.

---

## 3. Feature list

Each item lists **what**, **why**, effort, priority, and a source tag where the idea comes from Sparrow Wallet or Family Values. **Partial** marks an item that extends code already in the tree; the citation names the function or screen.

### 3.1 Privacy and coin control

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Privacy-aware coin selection | **Partial.** Extend `SelectUTXOs` (`transaction.go:116`) with preferences for single-input spends, consistent script types, confirmed coins, and avoiding change that links unrelated UTXOs. Keep BnB as the fee-shape default. | Inputs are the strongest on-chain link. A small selection policy reduces accidental clustering without taking control away from the user. | M | P0 | Inspired by Sparrow |
| Transaction privacy check | After building a transaction, run heuristics: common-input ownership, likely change, address reuse, round-number payment, script-type mix, and whether a change output is below the dust threshold. Show findings in the review tray and keep them in transaction details. | Users learn what their transaction reveals while they can still change it. This replaces the missing mixing workflow with honest analysis. | M | P0 | Inspired by Sparrow |
| BIP329 import UI | **Partial.** Add a paste-or-file import screen, parse the existing `tx`, `addr`, and `output` records (`label_manager.go:189`), show a summary tray with counts and skipped lines, then save. | Import exists in the wallet layer and has no user-facing path. This is the fastest interoperability win in the release. | S–M | P0 | Inspired by Sparrow |
| BIP329 input records and spend attribution | Export and import `input` records, and show "spent from <label>" in transaction details. | Input labels explain where a coin came from and make the privacy check legible. | S–M | P1 | Inspired by Sparrow |
| Per-outpoint labels | Label a specific `txid:vout` in addition to the transaction label, and use it in the UTXO list and send selection. | A transaction can contain several economically different coins. Per-coin labels are the unit users reason about. | S–M | P2 | Inspired by Sparrow |
| Freeze inside send selection | **Partial.** Add `f` to the UTXO selection step of send, mirroring the UTXO screen (`utxos.go:213`), and refresh the selection totals. | The moment a user notices a coin they want to avoid is the moment they are choosing inputs. | S | P1 | Inspired by Sparrow |
| Fresh-address enforcement | Warn when a receive address is reused, and offer the next unused address. Track "last copied" and "last received" per address. | Address reuse links payments permanently. A small warning prevents the most common privacy mistake. | S | P1 | Inspired by Sparrow |
| Stealth mode | One key hides balances, amounts, and fiat equivalents across screens, with a short shimmer and a persistent eye indicator. Confirmations always show real values. | Terminal windows are visible in shared spaces. Hiding values is a frequent, low-cost comfort feature. | S | P1 | Inspired by Family Values |
| Privacy tags for UTXOs | Optional tags such as `kyc`, `coinjoin`, `gift`, and `mining`, stored with the wallet, filterable in the UTXO screen. | Tags let users keep coin provenance without a mixing service. They also feed selection preferences. | S–M | P2 | Inspired by Sparrow |
| Payjoin receive (BIP78) | Add a receive mode that produces a payjoin URI and accepts a sender's PSBT through a user-provided endpoint. | Payjoin breaks the common-input assumption. It is a meaningful upgrade after the privacy check exists. | L | P3 | Inspired by Sparrow |

### 3.2 Transactions and fees

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Transaction viewer | A read-only detail screen for any history row: inputs with values and addresses, outputs with ownership and script type, fee, feerate, vsize, weight, locktime, version, confirmation state, and replacement links. Reuse the ownership logic from `psbt_inspect.go:66`. | Users need to audit what happened after signing, including transactions built elsewhere. This is the main missing inspection surface. | M | P0 | Inspired by Sparrow |
| Inline RBF on pending rows | **Partial.** In history and dashboard activity, pending rows show `↻ speed up`. The key opens a bump tray anchored to that row. | Fee bumping already works (`fee_bump.go:298`); placing it on the pending transaction matches where the user sees the problem. | S–M | P0 | Inspired by Family Values |
| Bump confirmation tray | Show old fee and feerate, new fee and feerate, delta in sats, replacement vs CPFP, and the effect on descendants before confirming. | Replacing a transaction is irreversible and easy to misread. A compact comparison prevents expensive mistakes. | S | P1 | Inspired by Family Values |
| Fee and mempool view | **Partial.** A screen with fee targets, current estimates, mempool congestion, and a histogram from the connected Electrum server or a sampled summary from Core (`getmempoolinfo`, `getmempoolentry`). Label the source and timestamp. | Users choose fees blind today. A local, privacy-preserving view improves every send. | M | P1 | Inspired by Sparrow |
| Fee source transparency | **Partial.** Show whether the estimate came from Electrum or Core, the target blocks, and the last refresh time on the fee step. | The number is only trustworthy if its origin is visible. | S | P2 | Inspired by Sparrow |
| Transaction cancel | Build a self-transfer at a higher fee to replace a pending payment, with a clear warning that cancellation is best-effort. | Users sometimes need to abort a stuck payment. The RBF builder supplies the replacement rules; cancellation adds a self-transfer output. | M | P2 | Inspired by Sparrow |
| Bump chain display | Group a replacement with its original and show `replaces <txid>` and `replaced by <txid>` in details. | Replacement history is otherwise invisible in a flat list. | S | P2 | Inspired by Sparrow |
| Batch recipient editing | **Partial.** Allow editing or removing a recipient from the list before fee selection, so a correction does not require restarting the send. | Batch payments are supported; the editing loop is the rough edge. | S | P2 | — |
| PSBTv2 import/export | Parse and emit BIP370 PSBTv2. | Ecosystem support is still thin. Keep the reader tolerant, but do not make v2 the default. | L | P3 | — |
| v3 TRUC and package relay | Build and submit v3 transactions with package-aware fee bumping (BIP431). | Valuable for Lightning and contract protocols; limited value for ordinary sends. | M–L | P3 | — |

### 3.3 Multisig and hardware

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Real-device validation matrix | **Partial.** Complete `docs/hardware-wallet-compatibility.md` with real devices for single-sig, 2-of-3, RBF, CPFP, and address verification. | Emulator coverage is strong; the release claim needs real-device evidence. This is a release gate. | M | P1 | Inspired by Sparrow |
| Per-account descriptor export | **Partial.** Export the descriptor for the account currently in focus; the menu currently always uses `accounts[0]` (`settings.go:566`). | Multi-account wallets currently export the wrong descriptor from the menu. | S | P1 | Inspired by Sparrow |
| Multi-part UR send and receive | **Partial.** Add fountain encoding for UR PSBTs and descriptors, and a decoder that accepts multiple frames. Keep BBQr for devices that use it (`send_psbt.go:438`). | Single-part UR limits PSBT size; QR-only signers need multi-frame support. | M | P1 | Inspired by Sparrow |
| Signing progress for multisig | **Partial.** Show which cosigner fingerprints have signed, which are missing, and the next required action (`CountPSBTSigners`, `psbt.go:187`). | Multisig signing is a coordination task; a progress view removes guesswork. | S | P1 | Inspired by Sparrow |
| BIP129 BSMS | Export and import BSMS setup records, with the existing multisig config as the fallback format (`backup.go:104`). | Standardizes coordinator and cosigner exchange beyond hax's JSON. | M | P2 | Inspired by Sparrow |
| Animated QR controls | **Partial.** Pause, step, and change frame rate for BBQr and UR playback. | Large PSBTs animate quickly; users need to control the stream for slower cameras. | S | P2 | Inspired by Sparrow |
| Cosigner verification surface | **Partial.** Show fingerprint, derivation path, xpub prefix, and device model for each cosigner, with a copy action. | Users can compare values against the device screen before trusting a multisig quorum. | S | P2 | Inspired by Sparrow |
| Hardware wallet policy registration | Register wallet policies with supporting devices (BIP388). | Improves multisig UX on devices that support policies; depends on device firmware. | L | P3 | Inspired by Sparrow |
| Taproot script-path multisig, MuSig2, and FROST | Add script-path spending, MuSig2 key aggregation (BIP327), and FROST threshold signatures. | The current Taproot support is key-path single-sig. This is a large protocol project. | L | P3 | — |

### 3.4 Backup and recovery

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Unified backup export UI | **Partial.** Expose `ExportUnifiedBackup` (`backup.go:209`): descriptor, wallet config, and labels in one file, with a clear statement that it contains no secrets. | The function exists with no screen. Users should leave with a restorable package. | S | P1 | Inspired by Sparrow |
| Backup verification flow | **Partial.** After wallet creation, ask the user to re-enter selected seed words and confirm the descriptor. Mark the wallet "backup verified" only after success. | A written seed is not a tested backup. A verification checkpoint catches transcription errors early. | S–M | P1 | Inspired by Family Values |
| Completion celebration | After successful verification, show a short ASCII burst and "Backup verified" in the tray, then return to the dashboard. Skippable and shown once. | Wallet creation is a rare, high-stakes moment. A small reward makes the step feel finished. | S | P2 | Inspired by Family Values |
| Passphrase checkpoint | **Partial.** Create and restore already ask for the passphrase twice (`wallet_setup.go:487`). Add a persistent "passphrase attached" badge and a derived-fingerprint check at unlock. | A wrong passphrase opens a valid but different wallet, which looks like lost funds. | S | P1 | Inspired by Sparrow |
| Argon2id key derivation | **Done (v1.3).** Version-2 envelope with Argon2id (time=3, 64 MiB, 4 threads), bounded parameters on read, and PBKDF2 v1 files migrated on unlock (`storage.go:23`). | Argon2id raises the cost of offline password guessing. Sparrow's choice is a useful reference. | — | Done | Inspired by Sparrow |
| Encrypted wallet file export/import | Export the encrypted wallet file and import it on another machine, with a version and network check. | Makes migration possible without exposing the seed. | M | P2 | — |
| Seed QR export | Render the BIP39 seed as a QR for offline storage or air-gapped import, behind the existing warning flow. | Useful for users who keep paper backups and want a machine-readable copy. | S | P2 | Inspired by Sparrow |
| Recovery drill in-app | **Partial.** A guided "test restore" flow that opens a temporary watch-only wallet from the backup and compares addresses. | Proves the backup restores the same wallet without risking the live one. | S–M | P2 | Inspired by Sparrow |
| Backup reminder | After the first receive, show a one-line reminder if the wallet is not marked backup-verified. | The risk of loss starts when funds arrive. | S | P2 | Inspired by Family Values |
| SLIP-39 and codex32 | Add optional seed-share and checksummed-paper backup formats. | Advanced recovery options; keep them out of the default path. | M–L | P3 | — |

### 3.5 Backends and networking

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Connection diagnostics | **Partial.** A screen with backend type, server host, protocol version, block height, last sync, latency, and certificate fingerprint. | When sync stalls, users need one place to see what is wrong. | S | P1 | Inspired by Sparrow |
| Configurable explorer | **Partial.** Add a setting to disable the mempool.space link or set a self-hosted explorer. History currently opens mempool.space directly (`history.go:1379`). | Opening a third-party explorer leaks the txid and the user's IP. | S | P1 | Inspired by Sparrow |
| Core cookie authentication | Read the `.cookie` file or accept a cookie path in addition to user/password. | Cookie auth is the default for a local Bitcoin Core node and avoids storing RPC credentials. | S | P2 | Inspired by Sparrow |
| Certificate details and re-trust | **Partial.** Show the pinned fingerprint, first-seen date, and an explicit re-trust action with a warning (`pinning.go:83`). | TOFU is only safe when the user can inspect and deliberately reset the pin. | S | P2 | Inspired by Sparrow |
| Tor stream isolation and proxy auth | **Partial.** Allow separate SOCKS credentials per backend and isolate streams where the proxy supports it. | Reduces correlation between wallet traffic and other Tor use. | M | P2 | Inspired by Sparrow |
| Server list management | **Partial.** Add, edit, and remove servers per network, with the selected server shown in Settings. No automatic fallback. | hax deliberately connects only to the chosen server; users still need to manage that list. | S | P2 | Inspired by Sparrow |
| Fee estimate source | Record and display whether an estimate came from Electrum or Core, and when it was fetched. | Makes fee decisions auditable. | S | P2 | — |
| Reconnect detail | **Partial.** Show attempt count, next retry, and the last error in the status line and diagnostics screen. | Reconnect already exists; the state deserves to be legible. | S | P2 | — |
| Native BIP158 client | **Partial.** Core history already uses `scanblocks`; add direct header and compact-filter sync with a local cache for Electrum-free operation. | Removes the indexer dependency for users who want trustless history without a full node. | L | P3 | — |
| BIP324 v2 transport | Encrypt direct P2P connections. | hax makes no P2P connections, so this has no place in v1.4. | L | P3 | — |

### 3.6 Watch-only and import/export

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| First-class descriptor import | **Partial.** Move single-sig descriptor import out of the multisig flow (`wallet_setup_multisig.go:791`) into a clear "Watch-only or descriptor" path with review and network validation. | Descriptor import is the standard way to watch a hardware or external wallet. | S | P1 | Inspired by Sparrow |
| Export all account descriptors | **Partial.** Offer a descriptor bundle for every account, plus the multisig descriptor when applicable. | Users with multiple accounts currently export one at a time, and the selected account is ignored. | S | P1 | Inspired by Sparrow |
| Generic descriptor parser | **Partial.** Parse arbitrary `pkh`, `wpkh`, `sh`, `wsh`, `tr`, multipath, and key-origin expressions into a shared AST (`descriptor.go:526`). | Unlocks complete imports, clearer errors, and a path to miniscript and BIP388. | M | P1 | Inspired by Sparrow |
| BIP329 import summary | **Partial.** Show imported, updated, skipped, and unknown records before saving, with a dry-run option. | Imports touch labels and frozen state; users should see the effect first. | S | P1 | Inspired by Sparrow |
| Generic CSV export | **Partial.** Add a neutral CSV alongside the Koinly format (`label_manager.go:249`), with configurable columns. | Users have different accounting tools. | S | P2 | Inspired by Sparrow |
| Address book | Save named recipients with network and script type, and offer them during send. | Reduces paste errors and address reuse. | M | P2 | Inspired by Sparrow |
| PSBT import improvements | **Partial.** Accept pasted base64, file paths, and multi-frame UR in one flow, with a preview before signing. | Import is split across screens today; one entry point is easier to teach. | S | P2 | Inspired by Sparrow |
| Export bundle for coordinators | Package PSBT, descriptor, and labels for a coordinator in one action. | Multisig coordination involves several artifacts; a bundle reduces mistakes. | S | P2 | — |
| Electrum wallet import | Import an Electrum-format wallet file. | Low demand for a terminal wallet; descriptor import covers most watch-only cases. | M | P3 | Inspired by Sparrow |

### 3.7 UX and TUI

| Item | What | Why | Effort | Priority | Tag |
|---|---|---|---|---|---|
| Tray interaction system | A reusable overlay for confirmations, warnings, and short education, drawn over the current screen with the background dimmed. One primary action, `Esc` to dismiss, title with a back affordance, step counter, and stepped heights. | Keeps context, makes progression visible, and gives rare flows a consistent shape. | M | P0 | Inspired by Family Values |
| Continue→Confirm label morph | On send review, the primary hint reads `Enter Continue`; the final tray reads `Enter Confirm & broadcast`, with a color and weight change. | Users notice the irreversible step without a second screen. | S | P1 | Inspired by Family Values |
| Layout-stability extension | **Partial.** Add tests for each tray, label morph, digit grouping, stealth mode, and empty state at 80x24 and 120x40. Reuse `layout_stability_test.go`. | The project rule is that text never shifts. New UI needs the same guarantee. | S | P1 | Inspired by Family Values |
| Helper text to the right | **Partial.** Keep the selected option's one-line description on the same row, as the choice menu already does (`choice_menu.go`). Extend it to fee targets and UTXO rows. | Preserves alignment and reduces vertical jumps. | S | P1 | Inspired by Family Values |
| Pending indicator moves to Activity | **Partial.** After broadcast, show a pending chip in the completion tray, then in the header and the new Activity row. A one-time highlight marks the landing. | Connects the action to its consequence and makes the pending state findable. | S | P1 | Inspired by Family Values |
| Digit grouping while typing | Insert display-only separators in the amount field (`1,234,567` sats, `0.01234567` BTC), keep the cursor on the same digit, and parse by stripping separators. | Large amounts are hard to read; grouping is a frequent, low-cost polish item. | S | P1 | Inspired by Family Values |
| Directional tab transitions | Slide the content pane 2–3 columns over about 80 ms when switching dashboard, history, and UTXOs; keep the header fixed. Disable under `NO_MOTION` or narrow terminals. | Direction communicates place without a full redraw. | M | P2 | Inspired by Family Values |
| Friendly empty states | **Partial.** "No payments yet. Press r to receive your first one." and "No coins yet. Your first receive will appear here." with a next action. | Empty screens are a user's first impression. | S | P2 | Inspired by Family Values |
| Rare-screen polish | **Partial.** Apply the same spacing, borders, key hints, and error placement to settings, help, descriptor, and error screens. | Consistency is most visible on the screens people visit least. | S–M | P2 | Inspired by Family Values |
| UTXO columns and filters | **Partial.** Add Address, Status, and Script columns; add `/` search and a frozen/all filter (`utxos.go:60`). | The UTXO screen is the coin-control home; it currently shows the minimum. | M | P1 | Inspired by Sparrow |
| Derivation path display | **Partial.** The settings account card formats paths with coin type `1'` for every network (`settings_views.go:298`); derive the coin type from the wallet network. | A wrong path undermines address verification and descriptor comparison. | S | P1 | Inspired by Sparrow |
| Global section switcher | A persistent tab or key map across dashboard, receive, send, history, addresses, UTXOs, and settings. | Users navigate by memorized keys today; a visible map shortens the learning curve. | M | P2 | Inspired by Family Values |
| Contextual help | **Partial.** `?` opens a tray with the keys for the current screen, with a link to the full help screen. | Keeps help close without adding permanent text. | S | P2 | Inspired by Family Values |

---

## 4. Sparrow Wallet parity

Sparrow is the reference desktop wallet for PSBT-first, standards-based Bitcoin custody. Its feature page and source show a consistent philosophy: encrypt everything, verify on the device, prefer a private server, and expose the transaction at byte level. hax shares the PSBT-first and self-custody values; the table compares the surfaces a terminal wallet can realistically match.

| Capability | Sparrow | hax status | v1.4 decision |
|---|---|---|---|
| **Coin control** | Branch-and-Bound and knapsack selectors, freeze, send-all, and a visual input/output diagram. | **Partial.** BnB then largest-first (`transaction.go:116`, `:184`, `:288`), freeze on the UTXO screen (`utxos.go:213`), send-all, batch payments. No diagram, no freeze inside send, no privacy preference. | **Adopt.** Add freeze to send selection, privacy-aware preferences, and a compact text diagram of inputs and outputs. P0–P1. |
| **UTXO labels and BIP329** | Labels for transactions, addresses, outputs, and inputs; export and import. | **Partial.** BIP329 export/import for `tx`, `addr`, `output` (`label_manager.go:131`, `:189`); no import UI; `input` records skipped; `output` records emitted only for frozen coins. | **Adopt.** Import UI, `input` records, per-outpoint labels, and a dry-run summary. P0–P1. |
| **Output descriptor export** | Descriptors for every script type and account, editable wallet configuration, Core/Sparrow round-trip. | **Partial.** Fixed-shape parser and checksum (`descriptor.go:16`, `:526`), export (`:213`), UR (`:286`); settings exports `accounts[0]` (`settings.go:566`); no generic or multipath parser. | **Adopt.** Per-account export, bundle export, and a generic parser. P1. |
| **BIP129 BSMS** | Standard multisig setup exchange with encrypted records. | **Absent.** Multisig JSON export exists (`backup.go:104`). | **Adopt after descriptors.** P2. |
| **PSBT QR, UR, and BBQr** | UR fountain codes for any PSBT size; BBQr support; animated playback. | **Partial.** Single-part UR (`ur/psbt.go:11`, `ur/ur.go:21`), multi-frame BBQr send (`send_psbt.go:438`), single-part UR decode (`ur/decode.go:39`). | **Adopt.** Multi-part UR send and receive, plus playback controls. P1. |
| **Payjoin (BIP78)** | Sender and receiver payjoin. | **Absent.** | **Later.** Receiving needs an endpoint and coordination. Do the privacy check first. P3. |
| **Privacy analysis** | Historically Whirlpool mixing plus coin control and transaction inspection. Whirlpool is defunct (see 4.1). | **Absent.** No heuristics, linkage view, or change analysis outside the pre-sign inspector. | **Adopt heuristics.** Common-input ownership, change detection, address reuse, round amounts, script-type mix, and UTXO age. P0–P1. |
| **Transaction viewer and editor** | Editable input/output diagram, byte-level fields, script and signature inspection, PSBT and hex export. | **Partial.** PSBT inspector before signing (`psbt_inspect.go:66`); history details are summary-level; no post-confirmation viewer or editor. | **Adopt the viewer.** Ship read-only inspection in v1.4; keep transaction editing out of scope. P0. |
| **Fee and mempool views** | Fee-rate charts, block targets, mempool state. | **Partial.** Static targets and estimates; no histogram or mempool screen. | **Adopt.** Local fee histogram from the connected backend, plus source and timestamp. P1. |
| **BIP322 message signing** | Sign and verify messages with wallet addresses. | **Absent.** | **Adopt.** P2. |
| **Watch-only wallets** | xpub, descriptor, hardware, and multisig watch-only; can stay unencrypted. | **Partial.** Hardware-backed watch-only (`manager.go:360`, `:462`), multisig xpub and descriptor import, seed upgrade (`manager.go:547`); single-sig descriptor import is hidden in the multisig flow. | **Adopt.** First-class descriptor watch-only flow and export bundle. P1. |
| **Hardware wallets over USB and QR** | Many devices; USB and air-gapped QR signing. | **Partial.** HWI USB detection and signing (`hardware.go`); address verification; PSBT QR export and import. QR is file/animation based, which fits a terminal. | **Adopt.** Improve UR and document device-specific flows; real-device validation is the gate. P1. |
| **Tor and proxy** | Built-in Tor, proxy configuration, private-server-first guidance. | **Partial.** Optional Tor SOCKS5 with Electrum; no stream isolation or per-backend proxy auth. | **Keep and polish.** Add proxy configuration UX and diagnostics; keep selected-server-only. P2. |
| **Server TOFU and certificate handling** | CA trust when configured; otherwise first-use certificate prompt, pinned per host, changed certificate raises an error. | **Shipped.** SHA-256 SPKI pin per `host:port`, mismatch error, explicit re-trust (`pinning.go:15`, `:83`). | **Keep.** Add fingerprint details and re-trust UI; keep the selected-server-only rule. P2. |
| **PayNym (BIP47)** | Reusable payment codes and a directory. | **Absent.** | **Later.** Notification infrastructure and a public directory conflict with the privacy posture. P3. |
| **Wallet encryption** | Argon2id key derivation, encrypted wallet file, encrypted seed, owner-only directories. | **Shipped (v1.3).** AES-256-GCM with Argon2id, PBKDF2 v1 files migrated on unlock (`storage.go:23`, `:338`), `0600`/`0700`. | **Done in v1.3.** |
| **PSBT-first design** | PSBTs are the transaction model from keystore to editor. | **Shipped.** PSBT create, inspect, sign, finalize, export, import (`psbt.go`), HWI and QR flows. | **Keep.** Extend the viewer and signing-progress surfaces. P1. |

### 4.1 Whirlpool and privacy analysis

Whirlpool was a coordinated coinjoin implementation associated with Samourai Wallet. The service became defunct in 2024, and hax should not plan an integration: there is no coordinator to connect to, and operating a mixer carries legal, operational, and liquidity burdens that are outside a terminal wallet's scope.

The useful replacement is analysis and control:

- **Common-input ownership:** flag inputs that likely belong to one entity; this is the link a payjoin breaks.
- **Change detection:** identify the likely change output by script type, derivation, and value, and warn when change is ambiguous or toxic (below dust or a round amount).
- **Address reuse:** count prior receives on the payment address and on the change address.
- **Round-number payments:** note when the payment amount is a round value that narrows the plausible wallet.
- **Script-type mixing:** flag a transaction that spends P2PKH and P2TR inputs together, which reveals a common owner.
- **UTXO age and clustering:** show the age of spent coins and whether they were previously spent together.
- **Post-hoc view:** keep the findings in transaction details so users can learn from confirmed history.

These checks are local, deterministic, and cheap. They fit the terminal and do not require a third-party service.

### 4.2 What parity means for a terminal wallet

A terminal cannot match Sparrow's editable diagram or its full hardware-wallet matrix on day one. It can match the parts that matter most: complete transaction inspection, label round-trip, descriptor interoperability, fee transparency, and explicit privacy feedback. The visual diagram becomes a compact text map:

```
Inputs                          Outputs
0.00400000 BTC  bc1q…7d2f  ──▶  0.00250000 BTC  bc1q…9x2k  (payment)
0.00120000 BTC  bc1q…4m1a  ──▶  0.00260000 BTC  bc1q…1p8v  (change, ours)
                                fee 0.00010000 BTC
```

The map is stable, copyable, and works over SSH. It also gives the privacy check a natural place to annotate ownership and linkage.

### 4.3 Security practices from Sparrow's implementation

Sparrow's source exposes several practices that sit behind the feature list:

- **Argon2 key derivation.** Sparrow tunes Argon2 to about 500 ms. hax now uses Argon2id since v1.3, with PBKDF2 v1 files migrated on unlock (`storage.go:23`); the gap is closed.
- **Encrypted wallet envelope.** Sparrow derives an EC key and encrypts the wallet with ECIES, with the seed encrypted inside. hax encrypts the whole wallet with AES-256-GCM (`storage.go:338`) and keeps the seed inside the same envelope. Both protect confidentiality; the v1.4 migration must preserve atomic writes and backup compatibility.
- **Consistency checks.** Sparrow refuses states such as "wallet was not encrypted, but seed is." Add explicit invariant checks alongside the Argon2 migration so a partially upgraded file cannot open.
- **Owner-only files.** Sparrow creates owner-only directories; hax uses `0700` directories and `0600` files.
- **TLS trust.** Sparrow uses CA trust when configured and otherwise TOFU pinning per host with a hard error on change. hax pins the SPKI per `host:port` and requires explicit re-trust (`pinning.go:15`, `:83`).
- **Address verification.** Sparrow shows the address, derivation path, and cosigner fingerprints for comparison with the device. hax verifies addresses on the device (`hardware.go`) and shows derivation details; the path display fix in 3.7 keeps those details correct on mainnet.

---

## 5. Family Values UX adapted to a terminal

Family Values describes three principles: simplicity through gradual revelation, fluidity through continuity, and delight through selective emphasis. hax already follows part of this: the choice menu puts the selected description on the same line, screens reserve space for errors, and layout-stability tests enforce no-shift rendering. v1.4 extends that foundation.

### 5.1 Principles mapped to hax

| Principle | Terminal expression | v1.4 examples |
|---|---|---|
| **Simplicity** | Show the balance, the next action, and one line of context. Advanced detail appears in a tray or a detail screen on demand. | Send review shows address, amount, fee, and change; `?` reveals scripts and derivation paths. |
| **Fluidity** | Keep the header and status bar fixed; change only the content pane. Use direction and labels to show progress. | Directional transitions between dashboard, history, and UTXOs; Continue→Confirm; pending chip moves to Activity. |
| **Delight** | Spend animation and color on rare moments; use tiny, frequent feedback elsewhere. | Backup-verified burst; digit grouping; copy badge; freeze toggle; stealth shimmer. |

### 5.2 Focused trays

A tray is a bordered overlay for one decision. It preserves the screen behind it, so the user keeps the context that produced the question.

**Tray rules**

- Width: `min(78, terminal width)`. Horizontal padding: 2 cells.
- Placement: anchored above the status bar, over the lower part of the content pane. The background is dimmed, and the originating row stays visible.
- Title row: `‹ Back  <Title>` on the left; `1 of 2` on the right when there is a sequence.
- One primary action and one escape. Primary actions use a verb: `Continue`, `Confirm & broadcast`, `Replace`, `Freeze`.
- Heights step by two rows across a sequence (`5`, `7`, `9`) so progress is visible without a progress bar.
- `Esc` closes the tray and returns focus to the exact row that opened it. No nested trays; a tray can grow into a full flow only for multi-step setup.
- Critical values never animate. The confirmation target and fee stay still.

**Where trays replace full-screen states**

| Flow | Tray content | Primary action |
|---|---|---|
| Send review | Address, amount, fee, change, warnings | `Continue` |
| Final send confirmation | Same values, plus "This cannot be undone" | `Confirm & broadcast` |
| RBF or CPFP | Original tx, old and new feerate, delta, descendant effect | `Replace` |
| Freeze coin | Outpoint, value, current label, effect on selection | `Freeze` |
| BIP329 import | Counts by record type, skipped lines, dry-run result | `Import` |
| Descriptor export | Account, script type, fingerprint, copy/QR choices | `Export` |
| Seed reveal | Warning, hold time, reveal action | `Reveal` |
| Certificate change | Host, old fingerprint, new fingerprint, risk sentence | `Re-trust` |
| Server change | Network-filtered server list, selected server | `Connect` |

**Example: bump tray**

```
┌─ ‹ Back  Speed up transaction ──────────────────── 1 of 2 ─┐
│ bc1q…9x2k   -0.00120000 BTC   pending 14m                 │
│ Current 8.1 sat/vB   New 14.0 sat/vB   +412 sats          │
│                                                            │
│ Enter  Continue          Esc  Cancel                       │
└────────────────────────────────────────────────────────────┘
```

### 5.3 Fluidity details

**Continue→Confirm.** The send review tray's primary hint reads `Enter Continue`. The final tray changes the label to `Enter Confirm & broadcast` and shifts the accent color from blue to amber. The two-step shape gives the user a chance to reread the destination and fee. The label change is the signal, so no extra paragraph is needed.

**Directional transitions.** When the user moves between dashboard, history, and UTXOs, slide the content pane two or three columns over about 80 ms in the direction of the destination. Keep the header and status bar fixed. Skip the animation when the terminal is narrower than 80 columns, when `NO_MOTION=1`, or when a sync or broadcast is running. Bubble Tea can drive this with a short ticker and an offset; the cost stays local to the root model.

**Persistent components.** The balance header, network badge, sync state, and pending count belong to the frame. They stay in place while the content pane changes. The selected row's description sits to its right, matching `choice_menu.go`.

**Pending indicator.** After a broadcast, the completion tray shows `◐ pending`. When the tray closes, the same chip appears in the header and on the new Activity row. A one-time 300 ms highlight marks the row. The chip persists until confirmation and then becomes a check. This is the Family Values idea of the pending indicator moving to where the transaction lives.

**RBF attached to the pending transaction.** Pending rows show `↻ b speed up`. Pressing `b` opens the bump tray anchored to that row. After replacement, the new transaction appears directly beneath the original with `replaces <short txid>`, and the original is marked `replaced`. The relationship stays visible in the flat list.

**Digit grouping while typing.** Format the amount field as the user types:

```
Amount   1,234,567 sats   (0.01234567 BTC)
```

Separators are display-only. Parsing strips them, the cursor stays on the same digit, and switching between BTC and sats preserves the value. For BTC, group the integer part only (`1,234.56789012`). Do not group the editable buffer in a way that changes cursor position or line length.

**Empty states.** Use one friendly sentence and one action:

```
No payments yet.
Press r to receive your first one.
```

```
No coins yet.
Your first receive will appear here.
```

### 5.4 Delight details

**Backup celebration.** After the seed-word check succeeds, draw a three-frame ASCII burst inside the tray for about 400 ms, then show `Backup verified.` and return to the dashboard. Show it once per wallet, allow `Esc` to skip, and never block the next action.

**Stealth mode.** `z` toggles hidden values. Balances render as blocks with a 600 ms shimmer, then settle. An eye icon in the header shows the state. Confirmation and review screens always show real values, because hiding them there would defeat the safety check.

**Micro-feedback.** Keep it small and frequent:

- A copy badge that fades after two seconds.
- A freeze toggle that changes the row symbol and the summary count in place.
- A fee target row whose description updates without moving the list.
- A label save that shows `Saved` next to the field.

**Rare-screen polish.** Settings, help, descriptor detail, error trays, and the unlock screen use the same border, padding, key-hint row, and error reserve as the main screens. This is where consistency is most visible because users have no muscle memory to fall back on.

### 5.5 Layout and accessibility rules

- **Baseline:** 80x24. Every new screen and tray must render without clipping or scroll at that size.
- **No shift:** reserve rows for hints and errors; truncate long values with an ellipsis; keep menus fixed-width. Extend `layout_stability_test.go` for every new flow.
- **Helper text:** one line, placed to the right of the selected item. Longer explanations live behind `?`.
- **Color:** never the only signal. Pair color with a symbol or a word (`✓`, `!`, `pending`).
- **Motion:** cap animations at about 12 fps and 120 ms; stop them during sync, signing, and broadcast; honor `NO_MOTION=1`, `NO_COLOR`, and `TERM=dumb`.
- **Focus:** `Esc` always returns to the row that opened a tray. `Enter` always performs the labeled primary action. No hidden modes.

### 5.6 Decisions this section preserves

- **Network first, Mainnet preselected.** Onboarding keeps the network choice as the first step (`wallet_setup_network.go`). Trays do not insert a "recommended server" step before it.
- **Selected-server-only.** Connection errors name the selected server and offer an explicit `Change server` action. The list is filtered to the wallet's network. There is no automatic failover and no backup-server suggestion.
- **Minimal helper text.** One line at a time. Details are one key away.
- **Stable layout.** Text never shifts. New interactions must pass the same layout tests as the existing seed quiz, unlock form, and send amount field.

---

## 6. Non-goals and rejected ideas

These are deliberate exclusions for v1.4. Some are permanent product decisions; others are timing decisions with a stated revisit trigger.

| Idea | Decision | Reason | Revisit |
|---|---|---|---|
| Automatic server failover or a backup-server list | **Rejected** | hax connects only to the server the user selected for the wallet's network (`backend/config.go`). Silent failover changes the trust and privacy model without consent. | Only as an explicit, per-wallet opt-in with a visible switch. |
| Built-in Whirlpool or another coinjoin coordinator | **Rejected** | Whirlpool is defunct. Running a coordinator adds legal, operational, and liquidity burden. Privacy heuristics and payjoin cover the user need without a service. | No. |
| Third-party explorer or fee API as a default | **Rejected** | Requests leak txids and IP addresses. Fee estimates come from the connected Electrum server or Core. The mempool.space link becomes configurable or disabled. | Opt-in only, with a warning. |
| P2P, SPV, or full-node implementation | **Rejected for v1.4** | Electrum and Core already cover the backend spectrum. Header sync, filter storage, and peer management are a separate product. | Separate light-client project. |
| Lightning, Ark, or other L2 integration | **Deferred** | Different security model, liquidity management, and a much larger surface. Listed as "Someday" in `TODO.md`. | After v1.4, as a separate module. |
| Mobile app, GUI, or browser build | **Rejected** | hax's identity is a terminal wallet with a small attack surface and SSH-friendly operation. | No. |
| Cloud backup, label sync, or telemetry | **Rejected** | Contradicts self-custody and the privacy posture. Backups are local files the user controls. | No. |
| Custodial or hosted watch-only service | **Rejected** | The wallet must not depend on a third party holding keys or viewing history. | No. |
| PSBTv2 in v1.4 | **Deferred** | The supported hardware and QR ecosystem is v0-first. v2 adds work with little user-visible benefit. | When a target device or coordinator emits v2. |
| Silent payments (BIP352) | **Deferred** | Requires a new scanning and indexing model across both backends. High complexity for the current user base. | After the backend interface exposes the needed data. |
| MuSig2 and FROST | **Deferred** | Taproot script-path multisig is the prerequisite. Both need nonce management and device coordination. | After Taproot multisig. |
| Miniscript policy compiler | **Deferred** | Needs a generic descriptor AST and a large validation surface. | After BIP380–386 completeness. |
| BIP47 PayNym | **Deferred** | Notification transactions and a public directory conflict with selected-server-only privacy. Low adoption. | If a private notification path exists. |
| Payjoin in v1.4 | **Deferred** | Receiving needs an HTTP endpoint; BIP77 coordination is still maturing. | After privacy heuristics. |
| BIP324 v2 transport | **Not applicable** | hax uses Electrum TLS and Core JSON-RPC; BIP324 applies to direct P2P connections. | With a P2P backend. |
| BIP85 deterministic entropy | **Deferred** | Adds a second recovery surface for low demand. | After backup export and verification. |
| SLIP-39 and codex32 | **Deferred** | Optional backup formats with new recovery rules. BIP39 remains the default. | After the backup verification flow ships. |
| Address book cloud sync | **Rejected** | Same privacy objection as label sync. A local address book is fine. | No. |
| Mouse-first interaction and animations that move text | **Rejected** | Violates the no-shift rule and the keyboard-first terminal model. | No. |
| Hiding server errors or retrying silently | **Rejected** | Users need to know when trust or connectivity changed. Errors are explicit and actionable. | No. |
| Changing onboarding order or the Mainnet default | **Rejected** | Network first with Mainnet preselected is an existing product decision (`wallet_setup_network.go`). | No. |

### 6.1 Decisions kept

- Network is chosen first, with Mainnet preselected.
- The wallet connects only to the selected server for its network.
- Helper text stays minimal and sits to the right of the selected item.
- Text never shifts; layout-stability tests are part of the definition of done.
- Fee data comes from the connected backend.
- PSBT remains the transaction boundary for hardware and air-gapped signing.

---

## 7. Suggested release sequence

This sequencing keeps P0 work together and leaves interop work for the end, when the UI surfaces that consume it exist.

| Milestone | Contents | Exit condition |
|---|---|---|
| **A. Interaction foundation** | Tray system, Continue→Confirm, layout tests, stealth mode, digit grouping, pending chip, inline RBF row | Every new flow renders at 80x24 without shifting; tests cover the seed quiz, unlock, send amount, send review, and bump tray. |
| **B. Transaction visibility** | Transaction viewer, bump tray, fee and mempool view, configurable explorer, UTXO columns and filters | A confirmed send can be inspected end to end; a pending send can be bumped from its row. |
| **C. Privacy and safety** | Privacy-aware selection, transaction privacy check, BIP329 import UI and input records, backup export and verification, passphrase checkpoint | A user can import labels, see linkage warnings, export a restorable backup, and verify it. |
| **D. Interoperability** | Per-account descriptor export, generic descriptor parser, multi-part UR, BSMS, BIP322, Argon2id (done in v1.3), Core cookie auth | Descriptor and PSBT round-trip against Core and Sparrow; a QR-only signer completes a multi-frame PSBT. |
| **Release gate** | Docs, real-device matrix, funded testnet4 smoke test, recovery drill | All P0 items complete; P1 items complete or explicitly deferred with a note in `CHANGELOG.md`. |

The release is complete when a new user can create a wallet, receive, send, inspect the result, bump a stuck payment, export and verify a backup, and move a descriptor to another wallet without leaving the terminal or reading source code.

# Public Beta Checklist

This document tracks the source-first public beta for `hax`.

It is meant to be executed and updated, not read as a promise.

## Launch Stance

- Network target: `testnet4`
- Distribution target: source-first
- Publishing gate: the source repo can go public before funded beta validation is complete
- Binary trust model: no public release artifacts until checksums and verification instructions exist
- Mainnet scope: explicitly out of scope for this beta
- Bitcoin Core scope: connectable but not yet validated as a finished beta backend

## Pre-Push Repo Checklist

- [x] Project, module, and binary naming are consistent
- [x] Internal-only repo files are removed
- [x] Public baseline docs exist
- [x] GitHub CI and templates exist
- [x] README includes a screenshot
- [x] Social preview image exists in the repo
- [x] README uses a GitHub-ready clone command for `hax`

## Validation Environment

Fill this in before running the beta checks.

| Field | Value |
|------|-------|
| Date |  |
| Operator |  |
| OS |  |
| Terminal |  |
| Go version |  |
| hax commit |  |
| Backend used |  |
| Faucet source |  |
| Local chain used |  |

## Smoke Test Runbook

Use a clean wallet directory or a clean machine.

Suggested prep:

1. Remove or move aside any existing `~/.hax` directory.
2. Build from the current commit with `make build`.
3. Start `./hax`.
4. Keep one note file open while testing so failures are logged immediately.

Do not mark the send path as validated unless the wallet has actually been funded.

### Smoke Test Matrix

| Flow | Result | Notes |
|------|--------|-------|
| Create wallet | Not run |  |
| Restore wallet | Not run |  |
| Unlock wallet | Not run |  |
| Receive address | Not run |  |
| Send transaction | Not run |  |
| Export PSBT | Not run |  |
| Import PSBT | Not run |  |
| Sign PSBT | Not run |  |
| Broadcast transaction | Not run |  |
| Export labels | Not run |  |
| Export Koinly CSV | Not run |  |

### Validation Split

- Unfunded validation covers create, restore, unlock, receive, navigation, and PSBT/UI behavior.
- Funded validation is required for balance updates, UTXO discovery, send, sign, and broadcast.
- Funded validation requires either testnet4 sats or a controlled local chain workflow.

### Smoke Test Steps

#### 1. Create Wallet

- Create a fresh software wallet
- Record wallet name and password
- Confirm the app returns to an expected post-create state

Pass if:

- wallet creation succeeds
- mnemonic is shown
- wallet can be unlocked afterward

#### 2. Restore Wallet

- Start from the recorded mnemonic
- Restore into a separate wallet name
- Confirm restore succeeds without manual file edits

Pass if:

- restore completes
- restored wallet unlocks

#### 3. Unlock Wallet

- Lock or restart the app
- Unlock both the created and restored wallet

Pass if:

- password input works
- unlock succeeds consistently
- dashboard loads without broken state

#### 4. Receive Address

- Open the receive screen
- record the first address
- optionally advance or refresh address state if the flow allows

Pass if:

- address is shown
- derivation/path display is coherent
- copy flow works if used

#### 5. Send Transaction

- Fund the wallet from a testnet4 faucet first
- If a faucet is not available, use a controlled local-chain workflow and record it above
- wait for wallet state to update
- send a small amount to a controlled testnet address

Pass if:

- amount and fee selection work
- review screen is coherent
- send flow reaches completion or expected broadcast state

#### 6. Export PSBT

- Create a PSBT from the send flow
- export it using the available export path

Pass if:

- export succeeds
- exported artifact is saved or copied as expected

#### 7. Import PSBT

- import the exported PSBT back into the app

Pass if:

- PSBT parses
- signature counts and state look coherent

#### 8. Sign PSBT

- sign with the local key path if available
- if hardware flow is being tested, log that separately in the hardware doc

Pass if:

- signing succeeds
- signature state updates correctly

#### 9. Broadcast Transaction

- finalize and broadcast the signed transaction

Pass if:

- txid is returned or shown
- history/dashboard state updates as expected

#### 10. Export Labels

- add at least one transaction label first
- export labels

Pass if:

- export succeeds
- output file is readable

#### 11. Export Koinly CSV

- export transaction history in Koinly format

Pass if:

- export succeeds
- output file exists and is parseable as CSV

## Recovery Drill

Track this with a fresh wallet and a separate clean restore.

### Recovery Matrix

| Step | Result | Notes |
|------|--------|-------|
| Record mnemonic correctly | Not run |  |
| Lock app and restart | Not run |  |
| Restore from mnemonic | Not run |  |
| Wallet balance matches expected state | Not run |  |
| Labels and metadata expectations are documented | Not run |  |

### Recovery Procedure

1. Create a fresh software wallet.
2. Record the mnemonic exactly once, outside the app.
3. Fund the wallet and wait for a stable visible balance.
4. Close the app.
5. Move aside the existing `~/.hax` directory or use a clean environment.
6. Restore the wallet from the mnemonic only.
7. Unlock the restored wallet.
8. Compare balance, receive flow, and transaction history against expectations.
9. Record what did and did not come back.

Minimum notes to capture:

- whether balance restored correctly
- whether transaction history repopulated correctly
- whether labels came back
- whether settings or metadata were lost
- whether any manual step was required

## Hardware Wallet And Multisig Validation

Use [hardware-wallet-compatibility.md](hardware-wallet-compatibility.md) to record device-specific results.

| Workflow | Result | Notes |
|----------|--------|-------|
| HWI device detection | Not run |  |
| Address display verification | Not run |  |
| PSBT export to hardware flow | Not run |  |
| PSBT import from hardware flow | Not run |  |
| Multisig descriptor export | Not run |  |
| Multisig signing round-trip | Not run |  |

## Sign-Off

| Area | Status | Notes |
|------|--------|-------|
| Public repo quality | Pending |  |
| Unfunded beta validation | Pending |  |
| Funded beta validation | Pending |  |
| Recovery confidence | Pending |  |
| Release artifact trust story | Pending |  |

## Final Decision

- Public beta approved: `yes / no`
- Blocking issues found:  |
- Follow-up issue links:  |

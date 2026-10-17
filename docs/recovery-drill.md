# Recovery Drill

This is the operator runbook for validating software-wallet recovery in `hax`.

It is intentionally narrow:

- software wallet only
- run on testnet4 (the procedure is identical on mainnet)
- mnemonic-based restore only

Use this alongside the [smoke test](smoke-test.md).

## Goal

Answer one practical question:

Can a user recover access to their wallet from the mnemonic alone without undocumented steps?

## Preconditions

- a fresh software wallet exists
- the wallet has been funded on testnet4
- the mnemonic has been recorded
- the current wallet state is known before the drill starts

Record before starting:

| Field | Value |
|------|-------|
| Wallet name |  |
| Expected balance |  |
| Expected tx count |  |
| Labels present |  |
| Commit tested |  |

## Procedure

1. Create a fresh software wallet and record the mnemonic.
2. Fund the wallet and wait until the expected balance is visible.
3. Note the wallet’s transaction count and whether any labels were added.
4. Exit the app.
5. Move aside `~/.hax` or switch to a clean environment.
6. Build and launch the same `hax` commit.
7. Restore the wallet from the mnemonic.
8. Unlock the restored wallet.
9. Wait for state sync if needed.
10. Compare the restored wallet against the pre-drill notes.

## Pass Criteria

- restore completes without manual file editing
- unlock succeeds
- wallet regains the expected balance
- transaction history repopulates as expected
- any missing metadata is documented clearly

## Failure Conditions

Mark the drill as failed if any of these happen:

- mnemonic restore does not complete
- restored wallet cannot unlock
- balance remains incorrect after a reasonable sync wait
- transaction history does not repopulate as expected
- undocumented manual repair steps are required

## Result Log

| Check | Result | Notes |
|------|--------|-------|
| Restore completed | Not run |  |
| Unlock completed | Not run |  |
| Balance matched expected state | Not run |  |
| Transaction history matched expectations | Not run |  |
| Labels matched expectations | Not run |  |
| No undocumented repair steps required | Not run |  |

## Notes To Preserve

- If labels do not restore, state that directly.
- If history takes time to repopulate, record the wait time.
- If backend choice affects the outcome, record it.

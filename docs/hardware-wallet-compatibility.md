# Hardware Wallet And Multisig Compatibility

This document is the public beta validation matrix for `hax`.

It is intentionally conservative. A workflow should not be described as compatible until it has been exercised and recorded here.

## Status Legend

- `Implemented`: the code path exists in `hax`
- `Validated`: exercised end-to-end and considered working for beta
- `Partial`: works with caveats
- `Not validated`: code exists but the workflow has not been signed off yet
- `Unsupported`: not currently expected to work

## Current Beta Position

- HWI integration exists in the app
- PSBT import and export flows exist
- Multisig descriptor export exists
- Broad device-by-device beta validation is still outstanding
- Mainnet claims should not be made from this document

## Single-Sig Hardware Wallet Matrix

| Workflow | App Support | Validation Status | Notes |
|----------|-------------|-------------------|-------|
| Detect HWI devices | Implemented | Not validated | Depends on local HWI setup and device support |
| Import xpub from hardware wallet | Implemented | Not validated | Needs real-device confirmation per vendor |
| Display receive address on device | Implemented | Not validated | Path policy differs across vendors |
| Export PSBT for device signing | Implemented | Not validated | Base64, file, and QR paths exist in app |
| Import signed PSBT back into `hax` | Implemented | Not validated | Needs round-trip validation with real devices |
| Finalize and broadcast after device signing | Implemented | Not validated | Needs end-to-end beta test |

## Multisig Matrix

| Workflow | App Support | Validation Status | Notes |
|----------|-------------|-------------------|-------|
| Create multisig wallet metadata | Implemented | Not validated | Needs broader workflow validation |
| Export multisig config for other wallets | Implemented | Not validated | Export path exists; interoperability needs real testing |
| Export multisig PSBT | Implemented | Not validated | Needs signing round-trip validation |
| Import partially signed multisig PSBT | Implemented | Not validated | Needs threshold-signing verification |
| Finalize fully signed multisig PSBT | Implemented | Not validated | Needs end-to-end test coverage |

## Device Validation Table

Fill this in only after a real workflow run.

| Device | Connection Mode | Detect | Address Verify | PSBT Sign | Multisig | Notes |
|--------|------------------|--------|----------------|-----------|----------|-------|
| Trezor | HWI | Not run | Not run | Not run | Not run |  |
| Ledger | HWI | Not run | Not run | Not run | Not run |  |
| Coldcard | File or airgap | Not run | Not run | Not run | Not run |  |
| BitBox02 | HWI | Not run | Not run | Not run | Not run |  |
| Jade | HWI or QR | Not run | Not run | Not run | Not run |  |
| Passport | QR or file | Not run | Not run | Not run | Not run |  |
| Keystone | QR | Not run | Not run | Not run | Not run |  |
| SeedSigner | QR | Not run | Not run | Not run | Not run |  |

## Validation Procedure

For each real device test, try to keep the flow order the same:

1. confirm the device is visible to the host or expected transport
2. confirm `hax` can detect or interoperate with it
3. verify at least one receive-address flow
4. export a PSBT
5. sign or partially sign the PSBT on-device
6. import the signed PSBT back into `hax`
7. finalize or document the exact point where the flow stopped

## Per-Run Log Template

Copy this block once per actual device run.

```md
### Device Run

- Device:
- Firmware:
- Host OS:
- HWI version:
- Wallet type: single-sig / multisig
- Address type:
- Transport: HWI / file / QR
- Result: validated / partial / failed

Steps:
- Detect:
- Address verify:
- Export PSBT:
- Sign PSBT:
- Import PSBT:
- Finalize:

Notes:
- 
```

## Known Constraints To Validate

- HWI behavior varies by vendor and firmware
- Non-standard derivation paths may be rejected or warned on by devices
- Address-display support can be stricter than PSBT-signing support
- Multisig interoperability depends on descriptor, key-origin, and PSBT compatibility
- Air-gapped QR workflows need frame-size and import/export validation

## Validation Notes

When recording a device result, capture:

- device model
- firmware version
- host OS
- HWI version
- single-sig or multisig
- address type
- whether the flow used HWI, file, or QR
- exact failure point if the flow did not complete

## Related Docs

- [public-beta-checklist.md](public-beta-checklist.md)
- [recovery-drill.md](recovery-drill.md)
- [references.md](references.md)

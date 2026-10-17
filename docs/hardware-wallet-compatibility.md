# Hardware Wallet And Multisig Compatibility

This document is the hardware wallet and multisig validation matrix for `hax`.

It is intentionally conservative. A workflow should not be described as compatible until it has been exercised and recorded here.

## Status Legend

- `Implemented`: the code path exists in `hax`
- `Validated`: exercised end-to-end and considered working
- `Partial`: works with caveats
- `Not validated`: code exists but the workflow has not been signed off yet
- `Unsupported`: not currently expected to work

## Current Position

- HWI integration exists in the app
- PSBT import and export flows exist
- Multisig descriptor export exists
- Broad device-by-device validation is still outstanding
- Mainnet claims should not be made from this document

## Single-Sig Hardware Wallet Matrix

| Workflow | App Support | Validation Status | Notes |
|----------|-------------|-------------------|-------|
| Detect HWI devices | Implemented | Not validated | Depends on local HWI setup and device support |
| Import xpub from hardware wallet | Implemented | Not validated | Needs real-device confirmation per vendor |
| Display receive address on device | Implemented | Not validated | Path policy differs across vendors |
| Export PSBT for device signing | Implemented | Not validated | Base64, file, and QR paths exist in app |
| Import signed PSBT back into `hax` | Implemented | Not validated | Needs round-trip validation with real devices |
| Finalize and broadcast after device signing | Implemented | Not validated | Needs an end-to-end real-device test |

## Multisig Matrix

| Workflow | App Support | Validation Status | Notes |
|----------|-------------|-------------------|-------|
| Create multisig wallet metadata | Implemented | Not validated | Needs broader workflow validation |
| Export multisig config for other wallets | Implemented | Not validated | Export path exists; interoperability needs real testing |
| Export multisig PSBT | Implemented | Not validated | Needs signing round-trip validation |
| Import partially signed multisig PSBT | Implemented | Not validated | Needs threshold-signing verification |
| Finalize fully signed multisig PSBT | Implemented | Not validated | Needs end-to-end test coverage |

## Emulator Validation

Run on a Trezor Model T emulator (firmware 2.7.0, debug build) with
Bitcoin Core 29.0 on regtest. This shows the PSBTs `hax` builds are
complete and signable; it is not a substitute for real-device runs.

| Flow | HWI 3.1.0 | HWI 2.1.1 |
|------|-----------|-----------|
| Single-sig P2PKH, P2SH-P2WPKH, P2WPKH, P2TR: build, device sign, finalize, Core accepts | Pass | Pass |
| 2-of-3 P2WSH and P2SH-P2WSH (local seed + device) | Pass | Pass |
| 2-of-3 legacy P2SH with change | Pass | Fails: HWI 2 sends change as a multisig address output, which the firmware rejects |
| Fee bumps, single-sig all four types: device signs a payment, its RBF replacement and a CPFP child; Core accepts each | Pass | Not run |
| Receive address verification, single-sig all four types: the device derives the address at the path `hax` passes and it matches; a wrong address is reported as a mismatch | Pass | Pass |

Each run also checks that Core's `analyzepsbt` reports the unsigned PSBT
as ready for the signer, that `finalizepsbt` produces the same
transaction as `hax`, and that previous transactions are fetched from a
node without `-txindex`.

Descriptor exports (software, hardware watch-only and multisig, every
script type) are checked against Core alone: `deriveaddresses` must
give the same first five receive and change addresses as `hax`, and
`importdescriptors` into a watch-only wallet must own them on the right
chain (`TestRegtestDescriptorExport`). Sparrow has not been checked.

To repeat it:

1. Start a Trezor emulator from
   [data.trezor.io](https://data.trezor.io/dev/firmware/releases/emulators/)
   or `trezor-user-env`, and load the test mnemonic
   `abandon ×11 about` with no passphrase.
2. Run:

   ```sh
   HAX_BITCOIND=/path/to/bitcoind HAX_HWI=/path/to/hwi \
     go test -tags emulator -run TestEmulator -v ./internal/wallet/
   ```

The test starts its own regtest node in a temporary directory.
`-run TestRegtest` needs only `HAX_BITCOIND`: it checks RBF and CPFP
with a seed wallet against Core's own replacement and package rules,
and the descriptor exports against Core's derivation.

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

- [smoke-test.md](smoke-test.md)
- [recovery-drill.md](recovery-drill.md)
- [references.md](references.md)

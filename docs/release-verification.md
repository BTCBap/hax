# Release Verification

This document describes how to verify `hax` release artifacts once binaries are published.

For now, `hax` is still source-first. These instructions are here so the verification story is ready before artifacts are promoted as a normal install path.

## Local Checksum Generation

Generate local cross-platform build outputs and a checksum manifest:

```bash
make checksums
```

This writes binaries and `checksums.txt` into `build/`.

## Verify A Downloaded Artifact

After a real release is published, download:

- the artifact you want
- the matching `checksums.txt`

Then verify it from the directory containing both files.

On Linux:

```bash
sha256sum -c checksums.txt --ignore-missing
```

On macOS:

```bash
shasum -a 256 -c checksums.txt
```

Expected result:

- the selected artifact reports `OK`
- the checksum matches the published manifest exactly

## Current Limits

- Beta releases are not signed
- Beta releases are not attested
- Checksums help detect accidental corruption or mismatch
- Checksums alone do not replace a stronger signing or attestation story

## Before Treating Artifacts As Trusted

The project should still complete these steps:

- publish real tagged artifacts
- publish the matching checksum manifest
- test the released artifacts on target platforms
- revisit whether unsigned beta artifacts remain acceptable

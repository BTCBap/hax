# Release Verification

Every tagged release (`vX.Y.Z`) is built by `.github/workflows/release.yml` with GoReleaser and publishes:

| Asset | Purpose |
|---|---|
| `hax_<ver>_<os>_<arch>.tar.gz` / `.zip` | Binaries + README, LICENSE, SECURITY |
| `checksums.txt` | SHA-256 of every archive |
| `checksums.txt.sigstore.json` | Keyless [Sigstore](https://www.sigstore.dev/) bundle (signature and certificate) over the checksum manifest |
| `*.sbom.json` | SPDX SBOM per archive (syft) |
| GitHub attestation | SLSA build provenance for the archives |

## 1. Verify the signature on `checksums.txt`

```bash
VERSION=vX.Y.Z
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/BTCBap/hax/.github/workflows/release.yml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

`Verified OK` proves the manifest was produced by this repository's release workflow for that tag. No long-lived maintainer key is involved.

## 2. Verify your archive

```bash
sha256sum --ignore-missing -c checksums.txt   # Linux
shasum -a 256 -c checksums.txt                # macOS (ignore "No such file" lines)
```

## 3. Verify build provenance (optional)

```bash
gh attestation verify hax_<ver>_linux_amd64.tar.gz --repo BTCBap/hax
```

## 4. Reproduce the build (optional)

Builds use `CGO_ENABLED=0`, `-trimpath`, `-buildid=`, and commit-derived timestamps. With the Go version from `go.mod`:

```bash
git checkout ${VERSION}
goreleaser release --snapshot --clean --skip=sign,sbom
sha256sum dist/*.tar.gz dist/*.zip   # compare against the published checksums.txt
```

Snapshot builds use a `-snapshot` version string, so file names (and the embedded version) differ from a tagged build. To compare byte-for-byte, build the tag itself with `goreleaser build --clean` and compare the binaries.

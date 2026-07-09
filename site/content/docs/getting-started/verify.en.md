The APROINT Copilot CLI release host is GitHub Releases at `https://github.com/aproint/copilot-cli/releases`.

Release artifacts are built and published by
`.github/workflows/release.yml` to GitHub Releases. Each release includes raw
binaries, `SHA256SUMS`, `sbom.spdx.json`, Sigstore keyless signature bundles,
and GitHub artifact attestations. Do not use the upstream Amazon ECS public key
to verify APROINT fork releases; this fork is not affiliated with, endorsed by,
or supported by Amazon Web Services.

```sh
version=v1.34.0
asset=copilot-linux
base="https://github.com/aproint/copilot-cli/releases/download/${version}"
curl -LO "${base}/${asset}" -LO "${base}/SHA256SUMS" -LO "${base}/${asset}.sigstore.json"
grep " ${asset}$" SHA256SUMS | shasum -a 256 -c -
cosign verify-blob \
  --bundle "${asset}.sigstore.json" \
  --certificate-identity-regexp 'https://github.com/aproint/copilot-cli/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "${asset}"
```

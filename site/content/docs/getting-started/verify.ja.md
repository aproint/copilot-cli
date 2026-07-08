APROINT Copilot CLI のリリースホストは `https://github.com/aproint/copilot-cli/releases` の GitHub Releases です。

リリース成果物は `aproint/copilot-cli` の GitHub Releases から公開されます。
各リリースには、バイナリ、`SHA256SUMS`、`sbom.spdx.json`、
Sigstore keyless signature bundle、GitHub artifact attestation が含まれます。
APROINT フォークのリリース検証に、上流の Amazon ECS 公開キーを使用しないでください。
このフォークは Amazon Web Services と提携しておらず、承認またはサポートもされていません。

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

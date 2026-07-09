You can install APROINT Copilot CLI through [Homebrew](https://brew.sh/) or by downloading the binaries directly.

## Homebrew 🍻

```sh
brew install aproint/tap/copilot-cli
```

??? info "Are you using Rosetta on a Mac machine with Apple silicon?"
    If your homebrew was installed with [Rosetta](https://developer.apple.com/documentation/apple-silicon/about-the-rosetta-translation-environment), then the `brew install` will install the amd64 build.
    If this is not what you want, please either reinstall homebrew without Rosetta, or use the manual installation option below.

## Manually
Copy and paste the command into your terminal.

=== "macOS"

    | Command to install    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-darwin && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |

=== "Linux x86 (64-bit)"

    | Command to install    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |

=== "Linux (ARM)"

    | Command to install    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux-arm64 && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |


=== "Windows"

    | Command to install    |
    | :---------- |
    | `Invoke-WebRequest -OutFile 'C:\Program Files\copilot.exe' https://github.com/aproint/copilot-cli/releases/latest/download/copilot-windows.exe` |

    !!! tip
        Please use the [Windows Terminal](https://github.com/microsoft/terminal) to have the best user experience. If you encounter permissions issues, ensure that you are running your terminal as an administrator.


!!! info
    To download a specific version, replace "latest" with the specific version. For example, to download v0.6.0 on macOS, type:
    ```
    curl -Lo copilot https://github.com/aproint/copilot-cli/releases/download/v0.6.0/copilot-darwin && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help
    ```

## Verify a release artifact

Release artifacts are built and published by
`.github/workflows/release.yml` to GitHub Releases. Each release includes raw
binaries, `SHA256SUMS`, `sbom.spdx.json`, Sigstore keyless signature bundles,
and GitHub artifact attestations.

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

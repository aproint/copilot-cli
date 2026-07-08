# Copilot のインストール

APROINT Copilot CLIは、[Homebrew](https://brew.sh/) を使ってインストールするか、バイナリを直接ダウンロードしてインストールすることができます。

## Homebrew でインストール 🍻

```sh
brew install aproint/tap/copilot-cli
```

## 手動でインストール
以下のコマンドをターミナルにコピー＆ペーストして実行します。

=== "macOS"

    | インストール用コマンド    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-darwin && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |

=== "Linux x86 (64-bit)"

    | インストール用コマンド    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |

=== "Linux (ARM)"

    | インストール用コマンド    |
    | :---------- |
    | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux-arm64 && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |


=== "Windows"

    | インストール用コマンド    |
    | :---------- |
    | `Invoke-WebRequest -OutFile 'C:\Program Files\copilot.exe' https://github.com/aproint/copilot-cli/releases/latest/download/copilot-windows.exe` |

    !!! tip
        より快適にご利用いただくために、[Windows Terminal](https://github.com/microsoft/terminal) をご利用ください。権限の問題が発生した場合は、ターミナルを管理者として実行していることを確認してください。


!!! info
    特定のバージョンをダウンロードするには、"latest" を特定のバージョンに置き換えてください。例えば、macOS で v0.6.0 をダウンロードするには、次のように入力します。
    ```
    curl -Lo copilot https://github.com/aproint/copilot-cli/releases/download/v0.6.0/copilot-darwin && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help
    ```

## リリース成果物の検証

リリース成果物は `aproint/copilot-cli` の GitHub Releases から公開されます。
各リリースには、バイナリ、`SHA256SUMS`、`sbom.spdx.json`、
Sigstore keyless signature bundle、GitHub artifact attestation が含まれます。

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

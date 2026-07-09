##  <img align="left" alt="APROINT Copilot CLI" src="./site/content/assets/images/copilot-logo-48-light.svg" width="85" /> APROINT Copilot CLI
###### _Build, Release and Operate Containerized Applications on AWS._

![latest version](https://img.shields.io/github/v/release/aproint/copilot-cli)

* **Documentation**: [https://aproint.github.io/copilot-cli/](https://aproint.github.io/copilot-cli/)

The APROINT Copilot CLI is an APROINT-maintained fork of AWS Copilot CLI.
It is not affiliated with, endorsed by, or supported by Amazon Web Services.
It helps developers build, release and operate production-ready containerized
applications on AWS App Runner or Amazon ECS on AWS Fargate.

Use Copilot to:
* Deploy production-ready, scalable services on AWS from a Dockerfile in one command.
* Add databases or inject secrets to your services.
* Grow from one microservice to a collection of related microservices in an application.
* Set up test and production environments, across regions and accounts.
* Set up CI/CD pipelines to release your services to your environments.
* Monitor and debug your services from your terminal.

<p align="center">
    <img alt="init" src="./site/content/assets/images/init-cropped.gif" width="600"/>
</p>

## Installation

To install with homebrew:
```sh
$ brew install aproint/tap/copilot-cli
```
To install manually, we're distributing binaries from our GitHub releases:

<details>
  <summary>Instructions for installing Copilot for your platform</summary>


| Platform | Command to install |
|---------|---------
| macOS | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-darwin && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |
| Linux x86 (64-bit) | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |
| Linux (ARM) | `curl -Lo copilot https://github.com/aproint/copilot-cli/releases/latest/download/copilot-linux-arm64 && chmod +x copilot && sudo mv copilot /usr/local/bin/copilot && copilot --help` |
| Windows | `Invoke-WebRequest -OutFile 'C:\Program Files\copilot.exe' https://github.com/aproint/copilot-cli/releases/latest/download/copilot-windows.exe` |

</details>

Release artifacts are built and published by
[`.github/workflows/release.yml`](.github/workflows/release.yml) to
[GitHub Releases](https://github.com/aproint/copilot-cli/releases). Each
release includes raw binaries, `SHA256SUMS`, `sbom.spdx.json`, Sigstore
keyless signature bundles, and GitHub artifact attestations.

To verify a downloaded binary:

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


## Getting started

Make sure you have the AWS command line tool installed and have already run `aws configure` before you start.

To get a sample app up and running in one command, run the following:

```sh
$ git clone git@github.com:aws-samples/aws-copilot-sample-service.git demo-app
$ cd demo-app
$ copilot init --app demo                \
  --name api                             \
  --type 'Load Balanced Web Service'     \
  --dockerfile './Dockerfile'            \
  --deploy
```

This will create a VPC, Application Load Balancer, an Amazon ECS Service with the sample app running on AWS Fargate.
This process will take around 8 minutes to complete - at which point you'll get a URL for your sample app running! 🚀

## Development requirements

Local builds and CI use Go 1.26 and Node.js 24 LTS. The Go module declares
`toolchain go1.26.0`; developers can use the version files in the repository
root with common version managers, or install matching runtimes directly.

## Learning more

Want to learn more about what's happening? Check out our documentation [https://aproint.github.io/copilot-cli/](https://aproint.github.io/copilot-cli/) for a getting started guide, learning about Copilot concepts, and a breakdown of our commands.

## Feedback

Have any feedback at all? Drop us an [issue](https://github.com/aproint/copilot-cli/issues/new).

We're happy to hear feedback or answer questions, so reach out, anytime!

## Security disclosures

If you think you've found a potential security issue, please do not post it in the Issues. Instead, use GitHub private vulnerability reporting for this repository.

## License
This library is licensed under the Apache 2.0 License.

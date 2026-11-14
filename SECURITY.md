# Security policy

cloudssh runs with your own cloud credentials and opens SSH sessions, so security reports are taken seriously.

## Reporting a vulnerability

Please do **not** open a public issue. Use GitHub's private reporting instead: go to the repository's **Security** tab and choose **Report a vulnerability**. Include what you found, how to reproduce it and the version (`cloudssh --version`).

You can expect an acknowledgement within a few days. Fixes are released as a patch version and credited in the release notes unless you prefer otherwise.

## Supported versions

Only the latest release receives security fixes.

## What cloudssh does with your credentials

- It uses the standard AWS credential chain on your machine and never stores or transmits credentials anywhere but to AWS.
- It makes read-only API calls (see [docs/iam.md](docs/iam.md)). The one write is EC2 Instance Connect's `SendSSHPublicKey`, only when you choose that connection method.
- SSH is the system `ssh` binary; cloudssh does not implement its own SSH client.
- It makes no network calls of its own (no telemetry, no update checks).
- `--demo` mode never touches the network or runs a real `ssh`.

## Verifying a download

Each release ships `checksums.txt`, signed with [cosign](https://github.com/sigstore/cosign) (keyless) and an SBOM per archive. See "Verify your download" in the [README](README.md#verify-your-download).

# Changelog

## [0.1.2](https://github.com/cloudssh-cli/cloudssh/compare/v0.1.1...v0.1.2) (2026-11-15)


### Features

* **config:** name the default ssh method after the cloud's service (instance-connect) ([#41](https://github.com/cloudssh-cli/cloudssh/issues/41)) ([7873ba7](https://github.com/cloudssh-cli/cloudssh/commit/7873ba7570d2b06fb0b447b6afef6d37b26909b1))


### Fixes

* **deps:** bump the go-deps group with 2 updates ([#39](https://github.com/cloudssh-cli/cloudssh/issues/39)) ([78390a1](https://github.com/cloudssh-cli/cloudssh/commit/78390a17ab5fc817d1a09608a20fbac29020d19d))

## [0.1.1](https://github.com/cloudssh-cli/cloudssh/compare/v0.1.0...v0.1.1) (2026-11-15)

A maintenance release: dependency updates, a tidier Homebrew tap and supply-chain hardening. There are no changes to cloudssh's features. Upgrade with `brew upgrade cloudssh`, or run the install script again.

### Dependencies

* AWS SDK for Go v2: `service/ec2` 1.341.0 → 1.345.0 and `config` 1.33.10 → 1.33.18
* `charmbracelet/x/ansi` 0.11.6 → 0.11.9

### Distribution

* The Homebrew formula is now published under `Formula/` in [cloudssh-cli/homebrew-tap](https://github.com/cloudssh-cli/homebrew-tap)
* README: the tap is live

### Security and supply chain

* CodeQL code scanning and OpenSSF Scorecard run on the repository; secret scanning with push protection and Dependabot security updates are enabled
* GitHub Actions are pinned to commit SHAs, and the release workflow's token is read-only by default
* Release archives keep their cosign-signed `checksums.txt` and per-archive SBOMs (see "Verify your download" in the README)

## [0.1.0](https://github.com/cloudssh-cli/cloudssh/releases/tag/v0.1.0) (2026-11-14)

The first public release of **cloudssh**: a [k9s](https://k9scli.io)-style terminal UI for browsing your cloud resources and SSHing into them. AWS is supported today; the provider layer is cloud-agnostic, so other clouds can follow.

### Highlights

- **Browse AWS in the terminal:** EC2 instances, ECS (clusters → services → tasks), EKS (clusters → node groups), Auto Scaling Groups and VPCs (→ subnets), plus a dashboard overview.
- **Drill down:** press `enter` to follow a resource to the instances behind it, with back/forward through the views you visited.
- **SSH with the real `ssh`:** cloudssh suspends its UI, hands your terminal to `ssh` and resumes when you exit, so `~/.ssh/config`, agents and ProxyJump keep working.
- **EC2 Instance Connect:** connect with a short-lived key pushed through AWS instead of your own key, and reuse an already-running Instance Connect Endpoint tunnel for private instances.
- **Search and filter:** one small query language (`web&api`, `a|b`, `!canary`, groups) for in-app search and server-side filters, with paging and background loading for large fleets.
- **Sign-in that explains itself:** uses the standard AWS credential chain (profiles, SSO, environment keys, instance roles), always shows how you are signed in, and opens a Profiles screen when credentials are missing or expired.
- **Themes:** automatic light/dark, or your own YAML themes.
- **Try it without an account:** `cloudssh --demo` runs on built-in sample data with no cloud access.

### Install

```sh
brew install cloudssh-cli/tap/cloudssh
# or
curl -fsSL https://raw.githubusercontent.com/cloudssh-cli/cloudssh/main/scripts/install.sh | sh
```

Binaries for macOS and Linux (amd64 and arm64) are attached below, with `checksums.txt` (signed with cosign) and an SBOM per archive. cloudssh needs the `ssh` client installed. See the README for how to verify a download.

### Notes

- Release binaries are not code-signed yet. Installing with brew or the script avoids the macOS Gatekeeper warning; a browser download may show it.
- Windows is not supported natively; use WSL with the Linux build.
- Feedback and issues are welcome on the [issue tracker](https://github.com/cloudssh-cli/cloudssh/issues).

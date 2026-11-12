# cloudssh

A [k9s](https://k9scli.io)-style terminal UI for browsing cloud resources and SSHing into them. AWS today: EC2 instances, ECS (clusters → services → tasks), EKS (clusters → node groups), Auto Scaling Groups and VPCs (→ subnets). Other clouds are on the roadmap.

SSH is the real `ssh` binary: cloudssh suspends its UI, hands your terminal to `ssh`, and resumes when you exit, so `~/.ssh/config`, agents and ProxyJump all work.

## Install

Not distributed yet. Build and run from source via Docker:

```sh
make build   # ./bin/cloudssh; linux by default, see "Build target" below
make run     # run the TUI in a container
```

## Try it without an account

```sh
cloudssh --demo
```

Runs on built-in sample data (three realistic accounts: EC2, Auto Scaling, VPC, ECS, EKS, profiles to switch between, enough machines to page through). No cloud account or network is used, and SSH opens a harmless local shell that shows the command it would have run. Try `--demo --page-size 5` to see paging and search limits more clearly.

## Usage

```sh
cloudssh --profile prod --region us-east-1
```

cloudssh uses the standard AWS credential chain (`--profile`, `AWS_PROFILE`, SSO, environment keys, instance role) and always shows how it is signed in at the top (`Auth: SSO (prod)`, `environment keys`, `instance role`, ...). If you use environment keys or a role there is nothing to set up.

If it cannot sign in (profile not found, no credentials yet, expired SSO session) the app still starts. It explains what is missing and opens the **Profiles** screen, a table of the profiles on your machine (auth type, region, details): pick one with `enter`, or press `r` to try again after `aws sso login` in another terminal. With no profiles at all it says so kindly and tells you how to create one. You can switch profile at any time with `p`.

Start on a specific screen with `--view dashboard|ec2|ecs|eks|asg|vpc`. Press `?` in the app for the full list of keys, including the sort key of every column on the current screen.

| Key | Action |
|---|---|
| `d` `0` | dashboard |
| `1`–`5` | EC2 · ECS · EKS · ASG · VPC (each view keeps its own drill-down stack) |
| `↑↓` `j k` · `pgup pgdn` · `g G` | move |
| `enter` | drill into the row (ASG → instances, VPC → subnets → instances, ECS cluster → services → tasks → instance, EKS cluster → node groups → instances); on instances, SSH |
| `esc` | clear search, then go back one level |
| `S` | SSH dialog: user, private/public IP, command preview, and a *Method* choice (`tab` to the row, `←/→` to change) to connect with [EC2 Instance Connect](#ec2-instance-connect) instead of your own key |
| `/` `f` `s` | search loaded rows (`↑↓` in the box recall previous searches) |
| `F` | server-side filters: every filter (name, state, type, AZ, IPs, ASG, …) is listed, fill in the ones you want (`R` resets them all) |
| `shift`+letter | sort by a column, again to reverse (letters listed in `?`) |
| `r` · `R` | refresh · reset filters, search and sort to defaults |
| `n` `shift+→` | load next page |
| `p` | profiles: switch the sign-in profile at any time (`enter` uses it, `esc` goes back) |
| `?` | help |
| `q` | quit |

Config (`$XDG_CONFIG_HOME/cloudssh/config.yaml`, or `--config`):

```yaml
cloud: aws
profile: default
region: us-east-1
pem_files_path: ~/.cloudssh/pems   # <key-pair-name>.pem is passed with -i when present
default_ip_type: private           # or public
ssh_method: ssh                    # or temporary-key (a short-lived key pushed through the cloud, e.g. EC2 Instance Connect)
page_size: 50
view: ec2                          # dashboard, ec2, ecs, eks, asg or vpc
skip_splash: false
```

## EC2 Instance Connect

No key pair needed: in the SSH dialog move to *Method* with `tab` and press `→` (or set `ssh_method: temporary-key` / `--ssh-method temporary-key`). cloudssh generates a throwaway ed25519 key with `ssh-keygen`, pushes the public half to the instance with `SendSSHPublicKey` (valid for about 60 seconds), runs your system `ssh` with only that key, and deletes it when the session ends. No AWS CLI is needed.

Requirements: the IAM permission `ec2-instance-connect:SendSSHPublicKey`, an AMI with Instance Connect installed (Amazon Linux 2/2023, Ubuntu 20.04+), port 22 reachable from your machine, and the right user (the dialog pre-fills the AMI's default). Private-only instances need a network path (VPN, bastion); EC2 Instance Connect Endpoint tunnels are not supported yet.

## Private instances: use your own tunnel

cloudssh does not manage tunnels. If you already run one for an instance behind an [EC2 Instance Connect Endpoint](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-using-eice.html), for example

```sh
aws ec2-instance-connect open-tunnel --instance-id i-0abc123 --local-port 2222
```

the SSH dialog notices it (it looks for `open-tunnel --local-port` processes that lead to the selected instance's SSH port) and offers a **Route: Running tunnel** choice (`tab` to it, `→` to switch). It is never chosen automatically, and the option does not appear when no matching tunnel runs. It combines with the Method choice (temporary key). The host key is stored under the instance id, so `known_hosts` stays valid whichever local port you pick.

Detection reads your local process list, so run the cloudssh binary directly (not `make run`, where the container cannot see your tunnel or `127.0.0.1`). Not available on Windows.

## Development

Everything runs in Docker; no Go toolchain is needed on your machine.

```sh
make check   # fmt + vet + lint + tests
make run     # run the TUI in a container (mounts ~/.aws and ~/.ssh read-only)
make help
```

### Build target

`make build` cross-compiles inside the container. With nothing configured it builds for the container's platform (linux). To always build for your own machine, set the target once, either in your shell profile:

```sh
export CLOUDSSH_BUILD_GOOS=darwin CLOUDSSH_BUILD_GOARCH=arm64
```

or in a gitignored `local.mk` at the repo root (`CLOUDSSH_BUILD_GOOS ?= darwin` / `CLOUDSSH_BUILD_GOARCH ?= arm64`). Variables set in the environment win over `local.mk`. They are intentionally not `GOOS`/`GOARCH`, which would also affect `make test` and `make lint`.

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Use at your own risk

cloudssh acts with your own credentials: it lists real resources and opens SSH sessions to real machines (possibly production), and can push a short-lived key to a machine when you choose that method. Check the `Auth:` and `Cloud:` lines in the header (they show which account and region you are in) before connecting. You are responsible for what you connect to. The software comes with no warranty, as stated in the MIT license below.

## License

MIT

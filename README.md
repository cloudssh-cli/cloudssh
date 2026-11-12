# cloudssh

A [k9s](https://k9scli.io)-style terminal UI for browsing cloud resources and SSHing into them. AWS today: EC2 instances, ECS (clusters → services → tasks), EKS (clusters → node groups), Auto Scaling Groups and VPCs (→ subnets). Other clouds are on the roadmap.

SSH is the real `ssh` binary: cloudssh suspends its UI, hands your terminal to `ssh`, and resumes when you exit, so `~/.ssh/config`, agents and ProxyJump all work.

## Install

Not distributed yet. Build and run from source via Docker:

```sh
make build   # ./bin/cloudssh (linux)
make run     # run the TUI in a container
```

## Usage

```sh
cloudssh --profile prod --region us-east-1
```

Start on a specific screen with `--view dashboard|ec2|ecs|eks|asg|vpc`. Press `?` in the app for the full list of keys, including the sort key of every column on the current screen.

| Key | Action |
|---|---|
| `d` `0` | dashboard |
| `1`–`5` | EC2 · ECS · EKS · ASG · VPC (each view keeps its own drill-down stack) |
| `↑↓` `j k` · `pgup pgdn` · `g G` | move |
| `enter` | drill into the row (ASG → instances, VPC → subnets → instances, ECS cluster → services → tasks → instance, EKS cluster → node groups → instances); on instances, SSH |
| `esc` | clear search, then go back one level |
| `S` | SSH dialog: user, private/public IP, command preview |
| `/` `f` `s` | search loaded rows (`↑↓` in the box recall previous searches) |
| `F` | server-side filters: every filter (name, state, type, AZ, IPs, ASG, …) is listed, fill in the ones you want; `ctrl+x` clears all |
| `shift`+letter | sort by a column, again to reverse (letters listed in `?`) |
| `r` · `R` | refresh · reset filters, search and sort to defaults |
| `n` `shift+→` | load next page |
| `?` | help |
| `q` | quit |

Config (`$XDG_CONFIG_HOME/cloudssh/config.yaml`, or `--config`):

```yaml
cloud: aws
profile: default
region: us-east-1
pem_files_path: ~/.cloudssh/pems   # <key-pair-name>.pem is passed with -i when present
default_ip_type: private           # or public
page_size: 50
view: ec2                          # dashboard, ec2, ecs, eks, asg or vpc
skip_splash: false
```

## Development

Everything runs in Docker; no Go toolchain is needed on your machine.

```sh
make check   # fmt + vet + lint + tests
make run     # run the TUI in a container (mounts ~/.aws and ~/.ssh read-only)
make help
```

See [CONTRIBUTING.md](CONTRIBUTING.md). The previous Python implementation is on the `main-python` branch.

## License

MIT

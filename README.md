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
| `S` | SSH dialog: user, private/public IP, command preview, and `ctrl+t` to connect with [EC2 Instance Connect](#ec2-instance-connect) instead of your own key |
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
ssh_method: ssh                    # or instance-connect (temporary key via EC2 Instance Connect)
page_size: 50
view: ec2                          # dashboard, ec2, ecs, eks, asg or vpc
skip_splash: false
```

## EC2 Instance Connect

No key pair needed: in the SSH dialog press `ctrl+t` (or set `ssh_method: instance-connect` / `--ssh-method instance-connect`). cloudssh generates a throwaway ed25519 key with `ssh-keygen`, pushes the public half to the instance with `SendSSHPublicKey` (valid for about 60 seconds), runs your system `ssh` with only that key, and deletes it when the session ends. No AWS CLI is needed.

Requirements: the IAM permission `ec2-instance-connect:SendSSHPublicKey`, an AMI with Instance Connect installed (Amazon Linux 2/2023, Ubuntu 20.04+), port 22 reachable from your machine, and the right user (the dialog pre-fills the AMI's default). Private-only instances need a network path (VPN, bastion); EC2 Instance Connect Endpoint tunnels are not supported yet.

## Private instances: use your own tunnel

cloudssh does not manage tunnels. If you already run one for an instance behind an [EC2 Instance Connect Endpoint](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-using-eice.html), for example

```sh
aws ec2-instance-connect open-tunnel --instance-id i-0abc123 --local-port 2222
```

the SSH dialog notices it (it looks for `open-tunnel --local-port` processes that lead to the selected instance's SSH port) and offers **Connect via: Running tunnel** — press `ctrl+o` to use it. It is never chosen automatically, and the option does not appear when no matching tunnel runs. It combines with `ctrl+t` (temporary key). The host key is stored under the instance id, so `known_hosts` stays valid whichever local port you pick.

Detection reads your local process list, so run the cloudssh binary directly (not `make run`, where the container cannot see your tunnel or `127.0.0.1`). Not available on Windows.

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

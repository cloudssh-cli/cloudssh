# cloudssh

A [k9s](https://k9scli.io)-style terminal UI for browsing cloud resources and SSHing into them. AWS (EC2) today; ECS, EKS, ASG, VPC and other clouds are on the roadmap.

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

| Key | Action |
|---|---|
| `↑/↓` | move |
| `/` | filter (`esc` clears) |
| `n` | load next page |
| `r` | refresh |
| `enter` | SSH into the selected instance |
| `q` | quit |

Config (`$XDG_CONFIG_HOME/cloudssh/config.yaml`, or `--config`):

```yaml
cloud: aws
profile: default
region: us-east-1
pem_files_path: ~/.cloudssh/pems   # <key-pair-name>.pem is passed with -i when present
default_ip_type: private           # or public
page_size: 50
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

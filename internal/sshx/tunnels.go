package sshx

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// LocalTunnel is a tunnel the user already runs, for example
//
//	aws ec2-instance-connect open-tunnel --instance-id i-0abc --local-port 2222
//
// cloudssh never starts or manages tunnels; it only notices them so ssh can
// go through one (private machines behind a cloud's managed tunnel service).
type LocalTunnel struct {
	Port       int // local listening port
	RemotePort int // port on the machine (default 22)
	InstanceID string
	PrivateIP  string
}

// Matches reports whether the tunnel leads to h's ssh port.
func (t LocalTunnel) Matches(h provider.Host) bool {
	if t.RemotePort != 22 {
		return false
	}
	return (t.InstanceID != "" && t.InstanceID == h.ID) || (t.PrivateIP != "" && t.PrivateIP == h.PrivateIP)
}

// tunnelParser recognises one cloud's tunnel command in a process command line
// (already split into arguments). To support another cloud's tunnel command,
// write a parser and add it to tunnelParsers; see .claude/skills/add-cloud.
type tunnelParser func(args []string) (LocalTunnel, bool)

var tunnelParsers = []tunnelParser{
	parseAWSInstanceConnectTunnel, // aws ec2-instance-connect open-tunnel --local-port N
}

// flagValue extracts "--name value" or "--name=value" from args.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
}

// parseAWSInstanceConnectTunnel handles `aws ec2-instance-connect open-tunnel`.
// Without --local-port the tunnel talks over stdio (it is an ssh ProxyCommand),
// so there is nothing local to connect to.
func parseAWSInstanceConnectTunnel(args []string) (LocalTunnel, bool) {
	if !contains(args, "ec2-instance-connect") || !contains(args, "open-tunnel") {
		return LocalTunnel{}, false
	}
	port, err := strconv.Atoi(flagValue(args, "--local-port"))
	if err != nil || port < 1 || port > 65535 {
		return LocalTunnel{}, false
	}
	remote := 22
	if v, err := strconv.Atoi(flagValue(args, "--remote-port")); err == nil {
		remote = v
	}
	return LocalTunnel{
		Port: port, RemotePort: remote,
		InstanceID: flagValue(args, "--instance-id"),
		PrivateIP:  flagValue(args, "--private-ip-address"),
	}, true
}

// ParseTunnels finds known tunnel commands in `ps` output, one per local port.
func ParseTunnels(psOutput string) []LocalTunnel {
	var out []LocalTunnel
	seen := map[int]bool{}
	for _, line := range strings.Split(psOutput, "\n") {
		args := strings.Fields(line)
		for _, parse := range tunnelParsers {
			if t, ok := parse(args); ok && !seen[t.Port] {
				seen[t.Port] = true
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// DetectTunnels lists the tunnels currently running on this machine. It is
// best effort: unsupported platforms and errors simply yield none.
func DetectTunnels(ctx context.Context) []LocalTunnel {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "args=").Output() //nolint:gosec // fixed arguments
	if err != nil {
		return nil
	}
	return ParseTunnels(string(out))
}

// TunnelsFor keeps the tunnels that lead to h.
func TunnelsFor(all []LocalTunnel, h provider.Host) []LocalTunnel {
	var out []LocalTunnel
	for _, t := range all {
		if t.Matches(h) {
			out = append(out, t)
		}
	}
	return out
}

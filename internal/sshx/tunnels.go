package sshx

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// LocalTunnel is a tunnel the user already runs, e.g.
//
//	aws ec2-instance-connect open-tunnel --instance-id i-0abc --local-port 2222
//
// cloudssh never starts or manages tunnels; it only notices them so ssh can
// go through one (private instances behind an EC2 Instance Connect Endpoint).
type LocalTunnel struct {
	Port       int // local listening port
	RemotePort int // port on the instance (default 22)
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

// ParseTunnels finds `ec2-instance-connect open-tunnel --local-port N`
// commands in `ps` output. Tunnels without --local-port talk over stdio (they
// are ssh ProxyCommands), so there is nothing local to connect to; skip them.
func ParseTunnels(psOutput string) []LocalTunnel {
	var out []LocalTunnel
	seen := map[int]bool{}
	for _, line := range strings.Split(psOutput, "\n") {
		args := strings.Fields(line)
		if !contains(args, "ec2-instance-connect") || !contains(args, "open-tunnel") {
			continue
		}
		port, err := strconv.Atoi(flagValue(args, "--local-port"))
		if err != nil || port < 1 || port > 65535 || seen[port] {
			continue
		}
		remote := 22
		if v, err := strconv.Atoi(flagValue(args, "--remote-port")); err == nil {
			remote = v
		}
		seen[port] = true
		out = append(out, LocalTunnel{
			Port: port, RemotePort: remote,
			InstanceID: flagValue(args, "--instance-id"),
			PrivateIP:  flagValue(args, "--private-ip-address"),
		})
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

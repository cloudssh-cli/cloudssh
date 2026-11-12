// Package sshx builds ssh invocations. cloudssh never re-implements SSH: it
// execs the system ssh so ~/.ssh/config, agents and ProxyJump keep working.
package sshx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// Target is a resolved connection request.
type Target struct {
	User    string
	Address string
	KeyFile string // optional
	// OnlyKey makes ssh offer just KeyFile (IdentitiesOnly), needed for
	// short-lived keys so agent keys don't exhaust the server's auth attempts.
	OnlyKey bool
}

// Args returns the argument vector for ssh (without the binary name).
func (t Target) Args() []string {
	var args []string
	if t.KeyFile != "" {
		args = append(args, "-i", t.KeyFile)
		if t.OnlyKey {
			args = append(args, "-o", "IdentitiesOnly=yes")
		}
	}
	return append(args, t.User+"@"+t.Address)
}

// Command returns an exec.Cmd wired to the current terminal.
func (t Target) Command() *exec.Cmd {
	cmd := exec.Command("ssh", t.Args()...) //nolint:gosec // args are passed as argv, never via a shell
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// Resolve picks the address (preferring preferIP, "private" or "public") and
// the key file (<pemDir>/<KeyName>.pem, only if it exists).
func Resolve(h provider.Host, user, preferIP, pemDir string) (Target, error) {
	addr := h.PrivateIP
	if preferIP == "public" && h.PublicIP != "" || addr == "" {
		addr = h.PublicIP
	}
	if addr == "" {
		return Target{}, errors.New("host has no reachable IP address")
	}
	if user == "" {
		user = h.DefaultUser
	}
	if user == "" {
		user = "ec2-user"
	}
	t := Target{User: user, Address: addr}
	if h.KeyName != "" {
		pem := filepath.Join(pemDir, h.KeyName+".pem")
		if st, err := os.Stat(pem); err == nil && !st.IsDir() {
			t.KeyFile = pem
		}
	}
	return t, nil
}

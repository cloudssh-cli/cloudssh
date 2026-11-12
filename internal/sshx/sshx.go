// Package sshx builds ssh invocations. cloudssh never re-implements SSH: it
// execs the system ssh so ~/.ssh/config, agents and ProxyJump keep working.
package sshx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	// Port and Options are for tunnelled connections (local forwarded port,
	// HostKeyAlias, ServerAliveInterval, ...).
	Port    int
	Options []string
}

// Args returns the argument vector for ssh (without the binary name).
func (t Target) Args() []string {
	var args []string
	if t.Port > 0 {
		args = append(args, "-p", strconv.Itoa(t.Port))
	}
	for _, o := range t.Options {
		args = append(args, "-o", o)
	}
	if t.KeyFile != "" {
		args = append(args, "-i", t.KeyFile)
		if t.OnlyKey {
			args = append(args, "-o", "IdentitiesOnly=yes")
		}
	}
	if t.User == "" {
		return append(args, t.Address) // ssh then uses its own default user
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
		user = h.DefaultUser // set by the provider (e.g. from the machine image); may stay empty
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

var safeArg = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

// shellQuote quotes s for display in a shell command line.
func shellQuote(s string) string {
	if safeArg.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// DemoCommand is what an ssh session becomes in demo mode (cloudssh --demo):
// a banner showing the command that would have run, then a local shell with
// a prompt for the pretend host. Nothing is sent anywhere.
func (t Target) DemoCommand() *exec.Cmd {
	parts := make([]string, len(t.Args()))
	for i, a := range t.Args() {
		parts[i] = shellQuote(a)
	}
	line := "ssh " + strings.Join(parts, " ")

	shell := "exec sh -i"
	if _, err := exec.LookPath("bash"); err == nil {
		shell = "exec bash --noprofile --norc -i"
	}
	script := `printf '[demo] would run: %s  (local shell, type exit to return)\n\n' "$1"; ` + shell

	who := t.User
	if who == "" {
		who = "demo"
	}
	cmd := exec.Command("sh", "-c", script, "demo", line) //nolint:gosec // fixed script; the target only appears as a printf argument
	cmd.Env = append(os.Environ(), `PS1=\[\033[1;32m\]`+who+"@"+t.Address+`\[\033[0m\]:\[\033[1;34m\]\w\[\033[0m\]\$ `)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

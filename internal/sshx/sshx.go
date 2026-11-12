// Package sshx builds ssh invocations. cloudssh never re-implements SSH: it
// execs the system ssh so ~/.ssh/config, agents and ProxyJump keep working.
package sshx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/SVMadhavaReddy/cloudssh/internal/logx"
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
	// KeyNote says why no pem file was added (or why the one added may not
	// work), for display next to the command. Empty when all is well.
	KeyNote string
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
// the key file (<pemDir>/<KeyName>.pem, only if it exists). A pem that is
// found is offered alone (IdentitiesOnly): it is the key the instance was
// launched with, and agent keys would only eat the server's auth attempts.
// When the key is missing or unusable, KeyNote says so.
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
	if h.KeyName != "" && pemDir != "" {
		t.KeyFile, t.KeyNote = findPem(pemDir, h.KeyName)
		t.OnlyKey = t.KeyFile != ""
		logx.L().Debug("pem lookup", "key_name", h.KeyName, "dir", pemDir, "file", t.KeyFile, "note", t.KeyNote)
	}
	return t, nil
}

// findPem looks for <dir>/<name>.pem. It returns the path when usable, and a
// note when there is something to tell the user (missing, unsafe, too open).
func findPem(dir, name string) (path, note string) {
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", "key pair name " + strconv.Quote(name) + " is not a plain file name; ignored"
	}
	pem := filepath.Join(dir, name+".pem")
	st, err := os.Stat(pem)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", "no " + pem + "; ssh uses your agent / default keys"
	case err != nil:
		return "", "cannot read " + pem + ": " + err.Error()
	case st.IsDir():
		return "", pem + " is a directory, not a key"
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		// ssh itself refuses such a key; tell the user how to fix it rather than touching their file.
		return pem, "permissions " + st.Mode().Perm().String() + " are too open, ssh will refuse it: chmod 600 " + pem
	}
	return pem, ""
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
// a table showing what would have run, then a local shell with a prompt for
// the pretend host. Nothing is sent anywhere.
func (t Target) DemoCommand() *exec.Cmd {
	shell := "exec sh -i"
	if _, err := exec.LookPath("bash"); err == nil {
		shell = "exec bash --noprofile --norc -i"
	}
	script := `printf '%s\n' "$1"; ` + shell

	who := t.User
	if who == "" {
		who = "demo"
	}
	cmd := exec.Command("sh", "-c", script, "demo", t.DemoTable()) //nolint:gosec // fixed script; the table only appears as a printf argument
	cmd.Env = append(os.Environ(), `PS1=\[\033[1;32m\]`+who+"@"+t.Address+`\[\033[0m\]:\[\033[1;34m\]\w\[\033[0m\]\$ `)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// CommandLine is the ssh command as a shell would take it, quoted.
func (t Target) CommandLine() string {
	args := t.Args()
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return "ssh " + strings.Join(parts, " ")
}

// demoWrap is how wide the value column of the demo table may get before a
// long value (a key path, the command) continues on the next line.
const demoWrap = 70

// DemoTable lays the connection out as a small table, one fact per row.
func (t Target) DemoTable() string {
	user := t.User
	if user == "" {
		user = "(ssh default)"
	}
	rows := [][2]string{{"User", user}, {"Address", t.Address}}
	if t.Port > 0 {
		rows = append(rows, [2]string{"Port", strconv.Itoa(t.Port)})
	}
	if t.KeyFile != "" {
		rows = append(rows, [2]string{"Key", t.KeyFile})
		if t.OnlyKey {
			rows = append(rows, [2]string{"Offers", "only this key (IdentitiesOnly)"})
		}
	}
	if len(t.Options) > 0 {
		rows = append(rows, [2]string{"Options", strings.Join(t.Options, ", ")})
	}
	if t.KeyNote != "" {
		rows = append(rows, [2]string{"Key note", t.KeyNote})
	}
	rows = append(rows, [2]string{"Command", t.CommandLine()})

	var lines [][2]string // wrapped rows; a continuation has an empty label
	kw, vw := 0, 0
	for _, r := range rows {
		kw = max(kw, len(r[0]))
		for i, part := range wrapRunes(r[1], demoWrap) {
			label := ""
			if i == 0 {
				label = r[0]
			}
			lines = append(lines, [2]string{label, part})
			vw = max(vw, len([]rune(part)))
		}
	}
	bar := func(l, m, r string) string {
		return l + strings.Repeat("─", kw+2) + m + strings.Repeat("─", vw+2) + r
	}
	var b strings.Builder
	b.WriteString("[demo] nothing is sent anywhere; this is what would have run (local shell below, type exit to return)\n")
	b.WriteString(bar("┌", "┬", "┐") + "\n")
	for _, l := range lines {
		b.WriteString("│ " + l[0] + strings.Repeat(" ", kw-len(l[0])) + " │ " + l[1] + strings.Repeat(" ", vw-len([]rune(l[1]))) + " │\n")
	}
	b.WriteString(bar("└", "┴", "┘") + "\n")
	return b.String()
}

// wrapRunes cuts s into pieces of at most n runes.
func wrapRunes(s string, n int) []string {
	r := []rune(s)
	if len(r) <= n {
		return []string{s}
	}
	var out []string
	for len(r) > n {
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return append(out, string(r))
}

package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/logx"
)

// clearScreen clears the visible screen and homes the cursor. It deliberately
// does not clear scrollback (3J): that history belongs to the user.
const clearScreen = "\x1b[2J\x1b[H"

// cleanSession runs a command like tea.ExecProcess, but on a cleared screen,
// so a session never starts underneath what the previous session (or the shell
// cloudssh was launched from) left behind.
type cleanSession struct {
	cmd *exec.Cmd
	out io.Writer
	in  io.Reader
}

func (c *cleanSession) SetStdin(r io.Reader) {
	c.in = r
	if c.cmd.Stdin == nil {
		c.cmd.Stdin = r
	}
}

func (c *cleanSession) SetStdout(w io.Writer) {
	c.out = w
	if c.cmd.Stdout == nil {
		c.cmd.Stdout = w
	}
}

func (c *cleanSession) SetStderr(w io.Writer) {
	if c.cmd.Stderr == nil {
		c.cmd.Stderr = w
	}
}

func (c *cleanSession) Run() error {
	w := c.out
	if w == nil {
		w = c.cmd.Stdout
	}
	if w != nil {
		_, _ = fmt.Fprint(w, clearScreen)
	}
	tail := &tailBuffer{max: 4096}
	if c.cmd.Stderr != nil {
		same := c.cmd.Stdout == c.cmd.Stderr // one writer for both: keep it one, so exec serialises the writes
		c.cmd.Stderr = io.MultiWriter(c.cmd.Stderr, tail)
		if same {
			c.cmd.Stdout = c.cmd.Stderr
		}
	} else {
		c.cmd.Stderr = tail
	}
	err := c.cmd.Run()
	if connectFailed(err) {
		logx.L().Error("ssh did not connect", "err", err, "stderr", tail.String())
		c.holdFailure(w, err, tail.String())
	}
	return err
}

// holdFailure keeps the session screen up after ssh itself failed: Bubble Tea
// would otherwise swap straight back to the list and the reason (ssh's own
// message above) would vanish before it could be read.
func (c *cleanSession) holdFailure(w io.Writer, err error, stderr string) {
	in := c.in
	if in == nil {
		in = c.cmd.Stdin
	}
	if w == nil || in == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "\r\n\x1b[1mssh did not connect\x1b[0m (%v)\r\n", err)
	if h := sshHint(stderr); h != "" {
		_, _ = fmt.Fprintf(w, "%s\r\n", h)
	}
	_, _ = fmt.Fprint(w, "\r\nPress enter to return to cloudssh... ")
	_, _ = bufio.NewReader(in).ReadString('\n')
}

// connectFailed: ssh exits 255 when it could not connect or authenticate (the
// remote command's own exit codes are never 255 by convention), and an error
// that is not an exit status means ssh could not be started at all.
func connectFailed(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 255
	}
	return true
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.buf) }

// sshHint turns ssh's error text into what to try next ("" when unknown).
func sshHint(stderr string) string {
	low := strings.ToLower(stderr)
	for _, h := range []struct{ match, hint string }{
		{"unprotected private key file", "The key file's permissions are too open: chmod 600 it."},
		{"bad permissions", "The key file's permissions are too open: chmod 600 it."},
		{"host key verification failed", "The host key does not match known_hosts (an address reused by another machine?): ssh-keygen -R <address>."},
		{"remote host identification has changed", "The host key changed (an address reused by another machine?): ssh-keygen -R <address>."},
		{"permission denied", "The server refused the key. Check the user name, that the key is the one the machine was created with, or try the other Method in the ssh dialog."},
		{"connection timed out", "No answer. Check private vs public address, the firewall / security group, and your VPN or tunnel."},
		{"operation timed out", "No answer. Check private vs public address, the firewall / security group, and your VPN or tunnel."},
		{"connection refused", "Nothing is listening on the ssh port. Is sshd running, and is it the right port or tunnel?"},
		{"no route to host", "No route to the host. Check your VPN or tunnel and the address."},
		{"could not resolve hostname", "The address does not resolve."},
		{"too many authentication failures", "The server gave up on the offered keys. Pick one key (the pem, or the temporary-key method)."},
	} {
		if strings.Contains(low, h.match) {
			return h.hint
		}
	}
	return ""
}

// execSession hands the terminal to cmd on a clean screen and reports back
// through cb when it ends.
func execSession(cmd *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
	return tea.Exec(&cleanSession{cmd: cmd}, cb)
}

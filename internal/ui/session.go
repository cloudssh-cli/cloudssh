package ui

import (
	"fmt"
	"io"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
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
}

func (c *cleanSession) SetStdin(r io.Reader) {
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
	return c.cmd.Run()
}

// execSession hands the terminal to cmd on a clean screen and reports back
// through cb when it ends.
func execSession(cmd *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
	return tea.Exec(&cleanSession{cmd: cmd}, cb)
}

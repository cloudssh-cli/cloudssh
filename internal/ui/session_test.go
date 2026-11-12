package ui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionsStartOnAFreshAlternateScreen(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo session-output")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := (&cleanSession{cmd: cmd}).Run(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, enterSessionScreen) || !strings.Contains(got, "session-output") {
		t.Fatalf("output %q must start by entering the session screen", got)
	}
	if strings.Contains(got, "\x1b[3J") || strings.Contains(strings.TrimPrefix(got, enterSessionScreen), "\x1b[2J") {
		t.Fatal("must never clear the user's own screen or scrollback")
	}
}

func TestSessionUsesTheWriterBubbleTeaProvidesWhenTheCommandHasNone(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo hello")
	s := &cleanSession{cmd: cmd}
	var out bytes.Buffer
	s.SetStdout(&out)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), enterSessionScreen) || !strings.Contains(out.String(), "hello") {
		t.Fatalf("got %q", out.String())
	}
}

func TestSshFailureHoldsTheScreenUntilEnter(t *testing.T) {
	cmd := fakeSSH(t, "echo 'user@host: Permission denied (publickey).' >&2; exit 255")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.Stdin = strings.NewReader("\n") // the user presses enter
	err := (&cleanSession{cmd: cmd}).Run()
	if err == nil {
		t.Fatal("the failure must still be reported")
	}
	got := out.String()
	for _, want := range []string{"Permission denied", "ssh did not connect", "server refused the key", "Press enter"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

func TestOrdinaryRemoteFailuresDoNotPause(t *testing.T) {
	cmd := fakeSSH(t, "exit 1") // the remote command's own status
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stdin = strings.NewReader("")
	if err := (&cleanSession{cmd: cmd}).Run(); err == nil {
		t.Fatal("exit status still reported")
	}
	if strings.Contains(out.String(), "Press enter") {
		t.Fatal("only ssh's own failures (255) hold the screen")
	}
}

func TestSshHints(t *testing.T) {
	for in, want := range map[string]string{
		"WARNING: UNPROTECTED PRIVATE KEY FILE!":                       "chmod 600",
		"ec2-user@1.2.3.4: Permission denied (publickey,gssapi-keyex)": "refused the key",
		"ssh: connect to host 10.0.0.1 port 22: Operation timed out":   "firewall",
		"ssh: connect to host x port 22: Connection refused":           "sshd",
		"something unusual": "",
	} {
		if got := sshHint(in); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("sshHint(%q) = %q, want it to mention %q", in, got, want)
		}
	}
}

// fakeSSH is a command named ssh (only those get the failure hold) running script.
func fakeSSH(t *testing.T, script string) *exec.Cmd {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	return exec.Command(path)
}

func TestSessionRunsOnItsOwnScreenAndLeavesTheUsersTerminalAlone(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo session-output")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := (&cleanSession{cmd: cmd}).Run(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasSuffix(got, leaveSessionScreen) || !strings.Contains(got, "session-output") {
		t.Fatalf("the session must run between enter and leave of the alternate screen: %q", got)
	}
	if strings.Contains(got, "\x1b[3J") || strings.Contains(strings.TrimPrefix(got, enterSessionScreen), "\x1b[2J") {
		t.Fatal("must never clear the user's own screen or scrollback")
	}
}

package ui

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestSessionsStartOnAClearedScreenButKeepScrollback(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo session-output")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := (&cleanSession{cmd: cmd}).Run(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "\x1b[2J\x1b[H") || !strings.Contains(got, "session-output") {
		t.Fatalf("output %q must start with the clear sequence", got)
	}
	if strings.Contains(got, "\x1b[3J") {
		t.Fatal("must not wipe the user's scrollback")
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
	if !strings.HasPrefix(out.String(), clearScreen) || !strings.Contains(out.String(), "hello") {
		t.Fatalf("got %q", out.String())
	}
}

func TestSshFailureHoldsTheScreenUntilEnter(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'user@host: Permission denied (publickey).' >&2; exit 255")
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
	cmd := exec.Command("sh", "-c", "exit 1") // the remote command's own status
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

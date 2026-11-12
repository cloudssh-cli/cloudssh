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

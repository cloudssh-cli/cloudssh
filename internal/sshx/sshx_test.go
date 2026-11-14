package sshx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

func TestResolvePrefersConfiguredIP(t *testing.T) {
	h := provider.Host{PrivateIP: "10.0.0.5", PublicIP: "1.2.3.4", DefaultUser: "ubuntu"}
	for _, tc := range []struct{ prefer, want string }{
		{"private", "10.0.0.5"},
		{"public", "1.2.3.4"},
	} {
		got, err := Resolve(h, "", tc.prefer, t.TempDir())
		if err != nil || got.Address != tc.want || got.User != "ubuntu" {
			t.Errorf("prefer %s: got %+v err %v", tc.prefer, got, err)
		}
	}
}

func TestResolveFallsBackToOnlyAvailableIP(t *testing.T) {
	got, err := Resolve(provider.Host{PublicIP: "1.2.3.4"}, "bob", "private", t.TempDir())
	if err != nil || got.Address != "1.2.3.4" || got.User != "bob" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestResolveNoIP(t *testing.T) {
	if _, err := Resolve(provider.Host{}, "", "private", t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestArgsIncludeKeyOnlyWhenPemExists(t *testing.T) {
	dir := t.TempDir()
	h := provider.Host{PrivateIP: "10.0.0.5", KeyName: "prod"}

	got, _ := Resolve(h, "u", "private", dir)
	if want := []string{"u@10.0.0.5"}; !reflect.DeepEqual(got.Args(), want) {
		t.Fatalf("no pem: got %v", got.Args())
	}

	pem := filepath.Join(dir, "prod.pem")
	if err := os.WriteFile(pem, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ = Resolve(h, "u", "private", dir)
	if want := []string{"-i", pem, "-o", "IdentitiesOnly=yes", "u@10.0.0.5"}; !reflect.DeepEqual(got.Args(), want) {
		t.Fatalf("with pem: got %v", got.Args())
	}
}

func TestArgsOnlyKeyAddsIdentitiesOnly(t *testing.T) {
	got := strings.Join(Target{User: "ec2-user", Address: "10.0.0.1", KeyFile: "/k", OnlyKey: true}.Args(), " ")
	if got != "-i /k -o IdentitiesOnly=yes ec2-user@10.0.0.1" {
		t.Fatalf("args: %s", got)
	}
	if got := strings.Join(Target{User: "u", Address: "a", KeyFile: "/k"}.Args(), " "); got != "-i /k u@a" {
		t.Fatalf("plain args: %s", got)
	}
}

func TestNewTempKeyGeneratesAndCleansUp(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	k, err := NewTempKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.Public, "ssh-ed25519 ") {
		t.Fatalf("public key: %q", k.Public)
	}
	if _, err := os.Stat(k.Path); err != nil {
		t.Fatal("private key missing")
	}
	k.Cleanup()
	if _, err := os.Stat(k.Dir); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove the key directory")
	}
}

func TestArgsTunnelPortAndOptions(t *testing.T) {
	got := strings.Join(Target{
		User: "ubuntu", Address: "127.0.0.1", Port: 4242, KeyFile: "/k", OnlyKey: true,
		Options: []string{"HostKeyAlias=i-1", "ServerAliveInterval=5"},
	}.Args(), " ")
	want := "-p 4242 -o HostKeyAlias=i-1 -o ServerAliveInterval=5 -i /k -o IdentitiesOnly=yes ubuntu@127.0.0.1"
	if got != want {
		t.Fatalf("args:\n got %s\nwant %s", got, want)
	}
}

func TestDemoCommandShowsTheCommandAndRunsOnlyALocalShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this machine")
	}
	cmd := Target{User: "ubuntu", Address: "10.0.1.5", KeyFile: "/tmp/my key.pem"}.DemoCommand()
	if filepath.Base(cmd.Path) != "sh" {
		t.Fatalf("demo mode must never start ssh, it starts %s", cmd.Path)
	}
	cmd.Stdin = strings.NewReader("echo reached-the-local-shell\nexit\n")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("demo shell: %v\n%s", err, out.String())
	}
	for _, want := range []string{"[demo]", "│ Command │ ssh -i '/tmp/my key.pem' ubuntu@10.0.1.5", "│ User    │ ubuntu", "local shell", "reached-the-local-shell"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{"plain": "plain", "ubuntu@10.0.0.1": "ubuntu@10.0.0.1", "a b": "'a b'", "it's": `'it'\''s'`, "$(x)": "'$(x)'"} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPemNotes(t *testing.T) {
	dir := t.TempDir()
	h := provider.Host{PrivateIP: "10.0.0.5", KeyName: "prod"}

	got, _ := Resolve(h, "u", "private", dir)
	if got.KeyFile != "" || !strings.Contains(got.KeyNote, "no "+filepath.Join(dir, "prod.pem")) {
		t.Fatalf("missing pem must be explained: %+v", got)
	}

	pem := filepath.Join(dir, "prod.pem")
	if err := os.WriteFile(pem, nil, 0o644); err != nil { //nolint:gosec // testing the too-open warning
		t.Fatal(err)
	}
	got, _ = Resolve(h, "u", "private", dir)
	if got.KeyFile != pem || !got.OnlyKey || !strings.Contains(got.KeyNote, "chmod 600") {
		t.Fatalf("open permissions must be flagged but the key still used: %+v", got)
	}
	_ = os.Chmod(pem, 0o600)
	if got, _ = Resolve(h, "u", "private", dir); got.KeyNote != "" {
		t.Fatalf("a good pem needs no note: %q", got.KeyNote)
	}
}

func TestPemNameCannotEscapeTheDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "evil.pem"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../evil", "a/b", ".."} {
		got, _ := Resolve(provider.Host{PrivateIP: "10.0.0.5", KeyName: name}, "u", "private", dir)
		if got.KeyFile != "" || got.KeyNote == "" {
			t.Errorf("key name %q must be ignored with a note: %+v", name, got)
		}
	}
}

func TestDemoTableRowsAndWrapping(t *testing.T) {
	long := "/var/folders/" + strings.Repeat("x", 90) + "/id_ed25519"
	tbl := Target{User: "ec2-user", Address: "10.0.0.4", KeyFile: long, OnlyKey: true, Port: 22}.DemoTable()
	for _, want := range []string{"┌", "└", "│ User", "│ Address", "│ Port", "│ Key", "only this key", "│ Command"} {
		if !strings.Contains(tbl, want) {
			t.Errorf("table lacks %q:\n%s", want, tbl)
		}
	}
	for _, l := range strings.Split(strings.TrimSpace(tbl), "\n")[1:] {
		if n := len([]rune(l)); n > demoWrap+20 {
			t.Errorf("row too wide (%d): %s", n, l)
		}
	}
}

func TestLabelledKeyShowsAsTheLabelNotATempPath(t *testing.T) {
	tg := Target{User: "ec2-user", Address: "10.0.0.4", KeyFile: "/var/folders/x/cloudssh-key-1/id_ed25519", OnlyKey: true, KeyLabel: "EC2 Instance Connect"}
	if got, want := tg.DisplayLine(), "ssh -i <EC2 Instance Connect> -o IdentitiesOnly=yes ec2-user@10.0.0.4"; got != want {
		t.Fatalf("display:\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(tg.CommandLine(), "cloudssh-key-1") {
		t.Fatal("the real command still carries the real path")
	}
	tbl := tg.DemoTable()
	if strings.Contains(tbl, "cloudssh-key-1") || !strings.Contains(tbl, "EC2 Instance Connect (temporary key)") {
		t.Fatalf("table must not show the temp path:\n%s", tbl)
	}
}

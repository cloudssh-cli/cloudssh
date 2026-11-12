package sshx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
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
	if want := []string{"-i", pem, "u@10.0.0.5"}; !reflect.DeepEqual(got.Args(), want) {
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

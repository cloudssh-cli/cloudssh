package sshx

import (
	"os"
	"path/filepath"
	"reflect"
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

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestVersionFlag(t *testing.T) {
	out, err := run(t, "--version")
	if err != nil || !strings.HasPrefix(out, "cloudssh dev") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestUnsupportedCloud(t *testing.T) {
	_, err := run(t, "--cloud", "gcp", "--config", writeConfig(t, ""))
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err=%v", err)
	}
}

func TestMissingExplicitConfig(t *testing.T) {
	_, err := run(t, "--config", filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInvalidConfigYAML(t *testing.T) {
	_, err := run(t, "--config", writeConfig(t, "cloud: [unterminated"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

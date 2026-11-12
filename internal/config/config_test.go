package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cloud != "aws" || cfg.PageSize != 50 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadMissingExplicitFileErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), true); err == nil {
		t.Fatal("expected error for missing explicit config")
	}
}

func TestLoadOverridesAndClampsPageSize(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("region: eu-west-1\npage_size: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "eu-west-1" || cfg.PageSize != 5 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestViewAndSplashDefaultsAndOverride(t *testing.T) {
	d := Default()
	if d.View != "" || d.SkipSplash {
		t.Fatalf("defaults: %+v", d)
	}
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("view: dashboard\nskip_splash: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.View != "dashboard" || !cfg.SkipSplash {
		t.Fatalf("overrides: %+v", cfg)
	}
}

func TestSSHMethodDefaultAndValidation(t *testing.T) {
	if Default().SSHMethod != "ssh" {
		t.Fatalf("default: %q", Default().SSHMethod)
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte("ssh_method: temporary-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(good, true); err != nil || cfg.SSHMethod != SSHMethodTemporaryKey {
		t.Fatalf("good: %+v %v", cfg, err)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("ssh_method: telnet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad, true); err == nil {
		t.Fatal("unknown ssh_method must be rejected")
	}
}

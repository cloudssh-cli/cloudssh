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
	if cfg.Cloud != "aws" || cfg.PageSize != 100 {
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
	if err := os.WriteFile(good, []byte("ssh_method: \" Instance-Connect \"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(good, true); err != nil || cfg.SSHMethod != "instance-connect" {
		t.Fatalf("value is trimmed and lower-cased: %+v %v", cfg, err)
	}
	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("ssh_method: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(empty, true); err != nil || cfg.SSHMethod != SSHMethodSSH {
		t.Fatalf("empty means ssh: %+v %v", cfg, err)
	}
}

func TestConfigDirIsXDGOrDotConfigOnEveryOS(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/cfg")
	if d, _ := Dir(); d != "/x/cfg/cloudssh" {
		t.Fatalf("XDG: %s", d)
	}
	if p, _ := ThemesDir(); p != "/x/cfg/cloudssh/themes" {
		t.Fatalf("themes: %s", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/u")
	if d, _ := Dir(); d != "/home/u/.config/cloudssh" {
		t.Fatalf("default: %s", d)
	}
	if p, _ := DefaultPath(); p != "/home/u/.config/cloudssh/config.yaml" {
		t.Fatalf("config path: %s", p)
	}
}

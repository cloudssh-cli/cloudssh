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

// Package config loads cloudssh settings from a YAML file with defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Cloud        string `yaml:"cloud"`
	Profile      string `yaml:"profile"`
	Region       string `yaml:"region"`
	PemFilesPath string `yaml:"pem_files_path"`
	DefaultIP    string `yaml:"default_ip_type"` // "private" or "public"
	PageSize     int32  `yaml:"page_size"`
	// View is the screen opened at start: "dashboard" or a resource key (ec2, ecs, eks, asg, vpc).
	View       string `yaml:"view"`
	SkipSplash bool   `yaml:"skip_splash"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Cloud:        "aws",
		PemFilesPath: "~/.cloudssh/pems",
		DefaultIP:    "private",
		PageSize:     50,
		View:         "ec2",
	}
}

// DefaultPath is where the config file is looked up when --config is not set.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cloudssh", "config.yaml"), nil
}

// Load reads path over the defaults. A missing file is not an error unless
// explicit is true (the user passed --config).
func Load(path string, explicit bool) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path) //nolint:gosec // path is the user-chosen config file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.PageSize < 5 {
		cfg.PageSize = 5
	}
	return cfg, nil
}

// ExpandHome expands a leading ~ in p.
func ExpandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

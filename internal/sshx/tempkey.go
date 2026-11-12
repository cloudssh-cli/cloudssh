package sshx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TempKey is a throwaway ed25519 key pair for short-lived SSH access
// (EC2 Instance Connect pushes the public half for ~60 seconds).
type TempKey struct {
	Dir    string // private temp dir holding the key
	Path   string // private key file
	Public string // public key line, ready to send
}

// NewTempKey generates a key pair with ssh-keygen (always present where ssh is).
func NewTempKey(ctx context.Context) (TempKey, error) {
	dir, err := os.MkdirTemp("", "cloudssh-key-")
	if err != nil {
		return TempKey{}, err
	}
	path := filepath.Join(dir, "id_ed25519")
	cmd := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "cloudssh-temp", "-f", path) //nolint:gosec // fixed args, temp path we created
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return TempKey{}, fmt.Errorf("ssh-keygen: %w: %s", err, strings.TrimSpace(string(out)))
	}
	pub, err := os.ReadFile(path + ".pub") //nolint:gosec // path we just created
	if err != nil {
		_ = os.RemoveAll(dir)
		return TempKey{}, err
	}
	return TempKey{Dir: dir, Path: path, Public: strings.TrimSpace(string(pub))}, nil
}

// Cleanup deletes the key material.
func (k TempKey) Cleanup() {
	if k.Dir != "" {
		_ = os.RemoveAll(k.Dir)
	}
}

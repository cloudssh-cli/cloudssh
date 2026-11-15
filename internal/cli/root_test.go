package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
	awsprov "github.com/cloudssh-cli/cloudssh/internal/provider/aws"
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

type viewProv struct{}

func (viewProv) Name() string { return "aws" }
func (viewProv) Resources() []provider.Resource {
	return (&awsprov.Provider{}).Resources()
}

func TestValidateView(t *testing.T) {
	for _, ok := range []string{"dashboard", "ec2", "ecs", "eks", "asg", "vpc"} {
		if err := validateView(ok, viewProv{}); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	err := validateView("nope", viewProv{})
	if err == nil || !strings.Contains(err.Error(), "dashboard, ec2, ecs, eks, asg, vpc") {
		t.Fatalf("want helpful error, got %v", err)
	}
}

func TestEffectivePageSize(t *testing.T) {
	for _, c := range []struct {
		flag       int
		demo       bool
		configured int32
		want       int32
	}{
		{0, false, 50, 50}, {0, true, 50, 25}, {10, true, 50, 10}, {2, false, 50, 5}, {100, false, 50, 100},
	} {
		if got := effectivePageSize(c.flag, c.demo, c.configured); got != c.want {
			t.Errorf("flag=%d demo=%v configured=%d: got %d, want %d", c.flag, c.demo, c.configured, got, c.want)
		}
	}
}

func TestSSHMethodIsCheckedAgainstTheCloud(t *testing.T) {
	aws := awsprov.NewDemo(10)
	for _, ok := range []string{"ssh", "instance-connect", "temporary-key"} {
		if err := validateSSHMethod(ok, aws); err != nil {
			t.Errorf("%q should be accepted: %v", ok, err)
		}
	}
	err := validateSSHMethod("telnet", aws)
	if err == nil || !strings.Contains(err.Error(), "instance-connect") {
		t.Fatalf("a typo must be rejected and name the valid choice, got %v", err)
	}
}

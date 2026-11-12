package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExplainAuthErrorCases(t *testing.T) {
	cases := map[string]struct {
		err     string
		profile string
		want    []string
	}{
		"missing profile": {"failed to get shared config profile, prod", "prod",
			[]string{`profile "prod" was not found`, "aws configure list-profiles", "--profile"}},
		"expired sso": {"failed to refresh cached credentials, the SSO session has expired or is invalid", "dev",
			[]string{"SSO session", "aws sso login --profile dev"}},
		"sso without profile": {"refresh cached SSO token failed", "",
			[]string{"aws sso login"}},
		"expired keys": {"ExpiredToken: the security token included in the request is expired", "",
			[]string{"have expired"}},
		"nothing at all": {"failed to refresh cached credentials, no EC2 IMDS role found", "",
			[]string{"no AWS credentials found", "AWS_ACCESS_KEY_ID", "aws sso login", "--profile"}},
	}
	for name, c := range cases {
		got := explainAuthError(errors.New(c.err), c.profile).Error()
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: message lacks %q:\n%s", name, w, got)
			}
		}
		if !strings.Contains(got, "details: "+c.err) {
			t.Errorf("%s: original error must be kept:\n%s", name, got)
		}
	}
}

// isolateAWS points the SDK at empty config files and clears every variable
// that could supply credentials, so these tests never touch the real account
// or the network.
func isolateAWS(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE",
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(k, "")
	}
}

func TestNewReportsMissingProfile(t *testing.T) {
	isolateAWS(t)
	_, err := New(context.Background(), "definitely-missing", "us-east-1", 50)
	if err == nil || !strings.Contains(err.Error(), `profile "definitely-missing" was not found`) {
		t.Fatalf("got %v", err)
	}
}

func TestNewReportsMissingCredentialsBeforeStartingTheUI(t *testing.T) {
	isolateAWS(t)
	start := time.Now()
	_, err := New(context.Background(), "", "us-east-1", 50)
	if err == nil || !strings.Contains(err.Error(), "no AWS credentials found") {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > credentialsTimeout+2*time.Second {
		t.Fatal("credential check must be bounded")
	}
}

func TestNewSucceedsWithEnvironmentCredentials(t *testing.T) {
	isolateAWS(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	p, err := New(context.Background(), "", "us-east-1", 50)
	if err != nil || p == nil {
		t.Fatalf("env credentials should be accepted: %v", err)
	}
}

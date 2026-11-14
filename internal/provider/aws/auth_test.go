package aws

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

func TestExplainAuthErrorCases(t *testing.T) {
	cases := map[string]struct {
		err      string
		profile  string
		headline string
		want     []string
	}{
		"missing profile": {"failed to get shared config profile, prod", "prod",
			`no AWS profile called "prod"`, []string{"~/.aws/config", "aws configure sso"}},
		"expired sso": {"failed to refresh cached credentials, the SSO session has expired or is invalid", "dev",
			"single sign-on session needs a refresh", []string{"aws sso login --profile dev", "press r"}},
		"sso without profile": {"refresh cached SSO token failed", "",
			"single sign-on session", []string{"aws sso login"}},
		"expired keys": {"ExpiredToken: the security token included in the request is expired", "",
			"credentials have expired", []string{"press r"}},
		"nothing at all": {"failed to refresh cached credentials, no EC2 IMDS role found", "",
			"No AWS credentials found yet", []string{"AWS_ACCESS_KEY_ID", "aws configure sso", "IAM role"}},
	}
	for name, c := range cases {
		got := explainAuthError(errorString(c.err), c.profile).Error()
		first, _, _ := strings.Cut(got, "\n")
		if !strings.Contains(first, c.headline) {
			t.Errorf("%s: headline %q lacks %q", name, first, c.headline)
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: message lacks %q:\n%s", name, w, got)
			}
		}
		if !strings.HasSuffix(got, "details: "+c.err) {
			t.Errorf("%s: original error must come last:\n%s", name, got)
		}
		for _, blame := range []string{"you forgot", "you must", "invalid input", "failed to"} {
			if strings.Contains(strings.ToLower(strings.SplitN(got, "details:", 2)[0]), blame) {
				t.Errorf("%s: friendly text should not blame the user (%q):\n%s", name, blame, got)
			}
		}
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestAccessDescriptions(t *testing.T) {
	for _, c := range []struct{ profile, source, want string }{
		{"", "EnvConfigCredentials", "environment keys"},
		{"prod", "SSOProvider", "SSO (prod)"},
		{"ci", "AssumeRoleProvider", "assumed role (ci)"},
		{"dev", "SharedConfigCredentials: /home/me/.aws/credentials", "access keys (dev)"},
		{"", "EC2RoleProvider", "instance role"},
		{"", "CredentialsEndpointProvider", "container role"},
		{"", "WebIdentityCredentials", "web identity"},
		{"x", "ProcessProvider", "credential process (x)"},
		{"", "SomethingNew", "AWS credentials"},
	} {
		if got := describeAccess(c.profile, c.source); got != c.want {
			t.Errorf("%q/%q = %q, want %q", c.profile, c.source, got, c.want)
		}
	}
}

// isolateAWS points the SDK at empty config files and clears every variable
// that could supply credentials, so these tests never touch the real account
// or the network. It returns the config and credentials file paths.
func isolateAWS(t *testing.T) (configPath, credsPath string) {
	t.Helper()
	dir := t.TempDir()
	configPath, credsPath = filepath.Join(dir, "config"), filepath.Join(dir, "credentials")
	for _, f := range []string{configPath, credsPath} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := identityLookup
	identityLookup = func(context.Context, *clients) provider.Identity { return provider.Identity{} } // never reach the network
	t.Cleanup(func() { identityLookup = old })
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_REGION",
		"AWS_DEFAULT_REGION", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(k, "")
	}
	return configPath, credsPath
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewStartsEvenWithAMissingProfileAndExplainsIt(t *testing.T) {
	isolateAWS(t)
	p, err := New(context.Background(), "definitely-missing", "us-east-1", 50)
	if err != nil || p == nil {
		t.Fatalf("the provider must start so the UI can help: %v", err)
	}
	if ae := p.AuthError(); ae == nil || !strings.Contains(ae.Error(), `no AWS profile called "definitely-missing"`) {
		t.Fatalf("AuthError: %v", ae)
	}
	if p.Access() != "not signed in" {
		t.Fatalf("access: %q", p.Access())
	}
	// resources must refuse politely rather than crash while signed out
	if _, err := (&instances{p: p}).List(context.Background(), provider.Query{}); err == nil || !strings.Contains(err.Error(), "no AWS profile") {
		t.Fatalf("List while signed out: %v", err)
	}
}

func TestNewReportsMissingCredentialsWithoutBlocking(t *testing.T) {
	isolateAWS(t)
	start := time.Now()
	p, _ := New(context.Background(), "", "us-east-1", 50)
	if ae := p.AuthError(); ae == nil || !strings.HasPrefix(ae.Error(), "No AWS credentials found yet.") {
		t.Fatalf("AuthError: %v", ae)
	}
	if time.Since(start) > credentialsTimeout+2*time.Second {
		t.Fatal("credential check must be bounded")
	}
}

func TestEnvironmentKeysNeedNoSetup(t *testing.T) {
	isolateAWS(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	p, _ := New(context.Background(), "", "us-east-1", 50)
	if p.AuthError() != nil {
		t.Fatalf("env keys should just work: %v", p.AuthError())
	}
	if p.Access() != "environment keys" || p.Profile() != "" || p.Region() != "us-east-1" {
		t.Fatalf("access=%q profile=%q region=%q", p.Access(), p.Profile(), p.Region())
	}
}

func TestNoRegionIsExplained(t *testing.T) {
	cfg, creds := isolateAWS(t)
	write(t, cfg, "[profile dev]\n")
	write(t, creds, "[dev]\naws_access_key_id = AKIDDEV\naws_secret_access_key = s\n")
	p, _ := New(context.Background(), "dev", "", 50)
	if ae := p.AuthError(); ae == nil || !strings.HasPrefix(ae.Error(), "No AWS region is set.") || !strings.Contains(ae.Error(), "--region") {
		t.Fatalf("AuthError: %v", ae)
	}
}

func TestSwitchingProfilesWorksAtRuntimeAndBeatsEnvironmentKeys(t *testing.T) {
	cfg, creds := isolateAWS(t)
	write(t, cfg, "[profile dev]\nregion = eu-west-1\n[profile ops]\nregion = ap-south-1\n")
	write(t, creds, "[dev]\naws_access_key_id = AKIDDEV\naws_secret_access_key = s\n[ops]\naws_access_key_id = AKIDOPS\naws_secret_access_key = s\n")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDENV") // environment keys are present...
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	p, _ := New(context.Background(), "", "", 50)
	_ = p.UseProfile(context.Background(), "") // starts on env keys; region unset here, so it may not be ready
	if err := p.UseProfile(context.Background(), "dev"); err != nil {
		t.Fatalf("switch to dev: %v", err)
	}
	// ...but choosing a profile must really use that profile's keys and region
	if p.Access() != "access keys (dev)" || p.Profile() != "dev" || p.Region() != "eu-west-1" {
		t.Fatalf("after dev: access=%q profile=%q region=%q", p.Access(), p.Profile(), p.Region())
	}
	if err := p.UseProfile(context.Background(), "ops"); err != nil || p.Region() != "ap-south-1" || p.Profile() != "ops" {
		t.Fatalf("after ops: %v %q %q", err, p.Region(), p.Profile())
	}
	if err := p.UseProfile(context.Background(), "nope"); err == nil || p.AuthError() == nil {
		t.Fatal("a missing profile must put the provider back into the explained, signed-out state")
	}
	if err := p.UseProfile(context.Background(), "dev"); err != nil || p.AuthError() != nil {
		t.Fatalf("and recover when a good profile is chosen again: %v", err)
	}
}

func TestProfilesListReadsTheAWSFiles(t *testing.T) {
	cfg, creds := isolateAWS(t)
	write(t, cfg, "[profile dev]\nregion = eu-west-1\n")
	write(t, creds, "[dev]\naws_access_key_id = AKID\n[ci]\naws_access_key_id = AKID2\n")
	got := (&Provider{}).Profiles()
	if len(got) != 2 || got[0].Name != "ci" || got[1].Name != "dev" || got[1].Region != "eu-west-1" {
		t.Fatalf("profiles: %+v", got)
	}
}

func TestParseProfiles(t *testing.T) {
	config := `
# a comment
[default]
region = us-east-1

[profile sso-prod]
sso_session = corp
sso_account_id = 1111
sso_role_name = Admin
region = eu-west-1

[profile deploy]
role_arn = arn:aws:iam::2222:role/Deployer
source_profile = default

[profile viaproc]
credential_process = /usr/local/bin/creds

[sso-session corp]
sso_start_url = https://corp.awsapps.com/start

[services s3-special]
s3 =
  endpoint_url = http://localhost:4566
`
	creds := "[default]\naws_access_key_id = AKIA1\n[keysonly]\naws_access_key_id = AKIA2\n"
	got := parseProfiles(config, creds)
	byName := map[string]provider.ProfileInfo{}
	for _, p := range got {
		byName[p.Name] = p
	}
	if len(got) != 5 {
		t.Fatalf("want default, sso-prod, deploy, viaproc, keysonly; got %+v", got)
	}
	want := map[string][3]string{ // kind, region, detail
		"default":  {"access keys", "us-east-1", ""},
		"sso-prod": {"SSO", "eu-west-1", "1111 Admin"},
		"deploy":   {"assumed role", "", "Deployer"},
		"viaproc":  {"credential process", "", ""},
		"keysonly": {"access keys", "", ""},
	}
	for name, w := range want {
		p := byName[name]
		if p.Kind != w[0] || p.Region != w[1] || p.Detail != w[2] {
			t.Errorf("%s = %+v, want %v", name, p, w)
		}
	}
	if got[0].Name != "default" || got[len(got)-1].Name != "viaproc" {
		t.Errorf("must be sorted by name: %+v", got)
	}
	if len(parseProfiles("", "")) != 0 {
		t.Error("no files means no profiles")
	}
}

func TestAccessKindIgnoresPathsInTheSource(t *testing.T) {
	if got := accessKind("SharedConfigCredentials: /tmp/environment/sso/credentials"); got != "access keys" {
		t.Fatalf("paths must not influence the kind: %q", got)
	}
}

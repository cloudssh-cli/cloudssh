package aws

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// section is one [header] of an INI file with its key/value pairs.
type section struct {
	name string
	kv   map[string]string
}

func parseINI(text string) []section {
	var out []section
	var cur *section
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			out = append(out, section{name: strings.TrimSpace(line[1 : len(line)-1]), kv: map[string]string{}})
			cur = &out[len(out)-1]
		case cur != nil:
			if k, v, ok := strings.Cut(line, "="); ok {
				cur.kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	return out
}

func lastSegment2(s string) string {
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// parseProfiles merges ~/.aws/config ([profile x], [default]) and
// ~/.aws/credentials ([x]) into one sorted list. sso-session and services
// sections are not profiles.
func parseProfiles(config, credentials string) []provider.ProfileInfo {
	byName := map[string]*provider.ProfileInfo{}
	get := func(name string) *provider.ProfileInfo {
		if p, ok := byName[name]; ok {
			return p
		}
		p := &provider.ProfileInfo{Name: name}
		byName[name] = p
		return p
	}
	for _, sec := range parseINI(config) {
		name := sec.name
		switch {
		case name == "default":
		case strings.HasPrefix(name, "profile "):
			name = strings.TrimSpace(strings.TrimPrefix(name, "profile "))
		default:
			continue // sso-session ..., services ...
		}
		p := get(name)
		p.Region = sec.kv["region"]
		switch {
		case sec.kv["sso_session"] != "" || sec.kv["sso_start_url"] != "":
			p.Kind = "SSO"
			if acct, role := sec.kv["sso_account_id"], sec.kv["sso_role_name"]; acct != "" || role != "" {
				p.Detail = strings.TrimSpace(acct + " " + role)
			}
		case sec.kv["role_arn"] != "":
			p.Kind, p.Detail = "assumed role", lastSegment2(sec.kv["role_arn"])
		case sec.kv["credential_process"] != "":
			p.Kind = "credential process"
		case sec.kv["web_identity_token_file"] != "":
			p.Kind = "web identity"
		}
	}
	for _, sec := range parseINI(credentials) {
		p := get(sec.name)
		if p.Kind == "" && sec.kv["aws_access_key_id"] != "" {
			p.Kind = "access keys"
		}
	}
	out := make([]provider.ProfileInfo, 0, len(byName))
	for _, p := range byName {
		if p.Kind == "" {
			p.Kind = "profile"
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func awsFile(env, rel string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aws", rel)
}

// Profiles lists the profiles configured on this machine.
func (p *Provider) Profiles() []provider.ProfileInfo {
	if p.demo != nil {
		out := make([]provider.ProfileInfo, 0, len(p.demo.accounts))
		for _, a := range p.demo.accounts {
			out = append(out, a.profile)
		}
		return out
	}
	read := func(path string) string {
		if path == "" {
			return ""
		}
		b, err := os.ReadFile(path) //nolint:gosec // the user's own AWS config files
		if err != nil {
			return ""
		}
		return string(b)
	}
	return parseProfiles(read(awsFile("AWS_CONFIG_FILE", "config")), read(awsFile("AWS_SHARED_CREDENTIALS_FILE", "credentials")))
}

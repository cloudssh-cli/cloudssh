package aws

import (
	"errors"
	"fmt"
	"strings"
)

// authProblem is a friendly, non-blaming explanation. Error() renders it as a
// headline followed by guidance lines (and the original SDK error last), which
// is the shape provider.Authenticator promises.
type authProblem struct {
	headline string
	lines    []string
	detail   string
}

func (a *authProblem) Error() string {
	out := a.headline
	for _, l := range a.lines {
		out += "\n" + l
	}
	if a.detail != "" {
		out += "\ndetails: " + a.detail
	}
	return out
}

// explainAuthError turns the SDK's terse config and credential errors into
// messages that say what happened and what to do next.
func explainAuthError(err error, profile string) error {
	raw := err.Error()
	lower := strings.ToLower(raw)
	detail := firstLine(raw)
	switch {
	case strings.Contains(lower, "failed to get shared config profile"):
		return &authProblem{
			headline: fmt.Sprintf("There is no AWS profile called %q on this machine.", profile),
			lines: []string{
				"Profiles live in ~/.aws/config and ~/.aws/credentials.",
				"Pick one of the profiles listed here, or set up a new one with: aws configure sso",
			},
			detail: detail,
		}
	case strings.Contains(lower, "sso") && (strings.Contains(lower, "expired") || strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "refresh") || strings.Contains(lower, "token") || strings.Contains(lower, "login")):
		return &authProblem{
			headline: "Your AWS single sign-on session needs a refresh.",
			lines:    []string{"Run:  aws sso login" + profileFlag(profile), "then press r here to try again."},
			detail:   detail,
		}
	case strings.Contains(lower, "expired"):
		return &authProblem{
			headline: "Your AWS credentials have expired.",
			lines:    []string{"Refresh them (aws sso login" + profileFlag(profile) + ", or export new keys), then press r here to try again."},
			detail:   detail,
		}
	default:
		return &authProblem{
			headline: "No AWS credentials found yet.",
			lines: []string{
				"cloudssh uses the standard AWS credential chain. Any of these works:",
				"  - a profile:      aws configure sso   (then pick it here)",
				"  - access keys:    export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY",
				"  - a role:         run on an instance / container with an IAM role",
			},
			detail: detail,
		}
	}
}

func noRegionError(profile string) error {
	return &authProblem{
		headline: "No AWS region is set.",
		lines: []string{
			"Pass one with --region (for example --region us-east-1),",
			"or save it in the profile:  aws configure set region us-east-1" + profileFlag(profile),
		},
	}
}

func profileFlag(profile string) string {
	if profile == "" {
		return ""
	}
	return " --profile " + profile
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var errNotConnected = errors.New("not connected to AWS yet")

// accessKind names how credentials were obtained from the SDK's credential
// source string, e.g. "SSOProvider" -> "SSO".
func accessKind(source string) string {
	// "SharedConfigCredentials: /path/to/file": only the provider name counts;
	// the path may contain any word.
	name, _, _ := strings.Cut(source, ":")
	s := strings.ToLower(name)
	switch {
	case strings.Contains(s, "sso"):
		return "SSO"
	case strings.Contains(s, "assumerole"):
		return "assumed role"
	case strings.Contains(s, "webidentity"):
		return "web identity"
	case strings.Contains(s, "ec2role"):
		return "instance role"
	case strings.Contains(s, "credentialsendpoint"):
		return "container role"
	case strings.Contains(s, "process"):
		return "credential process"
	case strings.Contains(s, "env"):
		return "environment keys"
	case strings.Contains(s, "shared"), strings.Contains(s, "static"):
		return "access keys"
	}
	return "AWS credentials"
}

// describeAccess is the header text: "profile prod · SSO" or "environment keys".
func describeAccess(profile, source string) string {
	kind := accessKind(source)
	if profile == "" {
		return kind
	}
	return "profile " + profile + " · " + kind
}

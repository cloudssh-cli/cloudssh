package aws

import (
	"errors"
	"fmt"
	"strings"
)

// explainAuthError turns the SDK's terse config and credential errors into
// messages that say what is wrong and how to fix it. The original error is
// kept at the end so nothing is hidden.
func explainAuthError(err error, profile string) error {
	raw := err.Error()
	lower := strings.ToLower(raw)
	var msg string
	switch {
	case strings.Contains(lower, "failed to get shared config profile"):
		msg = fmt.Sprintf(`AWS profile %q was not found (looked in ~/.aws/config and ~/.aws/credentials)
  list your profiles:   aws configure list-profiles
  then run:             cloudssh --profile <name>   (or export AWS_PROFILE=<name>)`, profile)
	case strings.Contains(lower, "sso") && (strings.Contains(lower, "expired") || strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "refresh") || strings.Contains(lower, "token") || strings.Contains(lower, "login")):
		msg = "your AWS SSO session is missing or has expired\n  log in again:   aws sso login" + profileFlag(profile)
	case strings.Contains(lower, "expired"):
		msg = "your AWS credentials have expired\n  refresh them (aws sso login" + profileFlag(profile) + ", or export new keys) and run cloudssh again"
	default:
		msg = `no AWS credentials found
  cloudssh uses the standard AWS credential chain. Do one of:
    - choose a profile:   cloudssh --profile <name>   (aws configure list-profiles)
    - log in with SSO:    aws sso login --profile <name>
    - export keys:        AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY (plus AWS_SESSION_TOKEN if temporary)`
	}
	return errors.New(msg + "\n  details: " + firstLine(raw))
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

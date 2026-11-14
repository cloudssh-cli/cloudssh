package aws

import (
	"context"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

const identityTimeout = 5 * time.Second

// identityLookup is replaceable so unit tests never reach the network.
var identityLookup = resolveIdentity

var ssoRole = regexp.MustCompile(`^AWSReservedSSO_(.+)_[0-9a-f]{16}$`)

// principalFromARN turns a caller ARN into the words people use:
//
//	arn:aws:sts::1:assumed-role/AWSReservedSSO_Admin_0123456789abcdef/me -> "role Admin"
//	arn:aws:sts::1:assumed-role/Deployer/session                         -> "role Deployer"
//	arn:aws:iam::1:user/ci/deploy-bot                                    -> "user deploy-bot"
//	arn:aws:iam::1:root                                                  -> "root"
func principalFromARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	res := parts[5]
	switch {
	case res == "root":
		return "root"
	case strings.HasPrefix(res, "assumed-role/"):
		segs := strings.Split(res, "/")
		if len(segs) < 2 {
			return ""
		}
		role := segs[1]
		if m := ssoRole.FindStringSubmatch(role); m != nil {
			role = m[1]
		}
		return "role " + role
	case strings.HasPrefix(res, "user/"):
		segs := strings.Split(res, "/")
		return "user " + segs[len(segs)-1]
	case strings.HasPrefix(res, "federated-user/"):
		return "federated " + strings.TrimPrefix(res, "federated-user/")
	}
	return ""
}

// resolveIdentity asks the cloud who we are. Everything is best effort: a
// missing permission just means a less detailed header.
func resolveIdentity(ctx context.Context, cl *clients) provider.Identity {
	ctx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()
	var id provider.Identity
	if cl.sts == nil {
		return id
	}
	if out, err := cl.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err == nil {
		id.Account = awssdk.ToString(out.Account)
		id.Principal = principalFromARN(awssdk.ToString(out.Arn))
	}
	if id.Account != "" && cl.iam != nil {
		if out, err := cl.iam.ListAccountAliases(ctx, &iam.ListAccountAliasesInput{}); err == nil && len(out.AccountAliases) > 0 {
			id.Alias = out.AccountAliases[0]
		}
	}
	return id
}

// Identity is who the app is acting as.
func (p *Provider) Identity() provider.Identity {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.identity
}

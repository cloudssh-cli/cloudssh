// Package aws implements provider.Provider on top of aws-sdk-go-v2.
package aws

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// credentialsTimeout bounds the credential check (the default chain may try
// the EC2 metadata service, which is slow to give up off AWS).
const credentialsTimeout = 10 * time.Second

// clients are the service clients for one profile. They are replaced as a
// whole when the user switches profile.
type clients struct {
	ec2 ec2API
	asg asgAPI
	ecs ecsAPI
	eks eksAPI
	eic instanceConnect
	sts stsAPI
	iam iamAPI
}

type Provider struct {
	pageSize     int32
	regionFlag   string // --region / config, wins over the profile's region
	profileInUse string

	mu       sync.RWMutex
	cl       *clients
	authErr  error
	region   string
	access   string // header text, e.g. "SSO (prod)"
	identity provider.Identity
	demo     *demoState
}

var (
	_ provider.Authenticator = (*Provider)(nil)
	_ provider.HostResolver  = (*Provider)(nil)
)

// New returns a provider even when it cannot sign in: the UI starts, says what
// is missing and lets the user pick a profile or retry (see AuthError).
func New(ctx context.Context, profile, region string, pageSize int32) (*Provider, error) {
	p := &Provider{pageSize: pageSize, regionFlag: region}
	_ = p.UseProfile(ctx, profile) // the outcome is kept in AuthError
	return p, nil
}

// buildClients loads the profile, verifies its credentials and region, and
// creates the service clients.
func buildClients(ctx context.Context, profile, regionFlag string) (*clients, string, string, provider.Identity, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	if regionFlag != "" {
		opts = append(opts, awsconfig.WithRegion(regionFlag))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, "", "", provider.Identity{}, explainAuthError(err, profile)
	}
	cctx, cancel := context.WithTimeout(ctx, credentialsTimeout)
	defer cancel()
	creds, err := cfg.Credentials.Retrieve(cctx)
	if err != nil {
		return nil, "", "", provider.Identity{}, explainAuthError(err, profile)
	}
	if cfg.Region == "" {
		return nil, "", "", provider.Identity{}, noRegionError(profile)
	}
	cl := &clients{
		ec2: ec2.NewFromConfig(cfg),
		asg: autoscaling.NewFromConfig(cfg),
		ecs: ecs.NewFromConfig(cfg),
		eks: eks.NewFromConfig(cfg),
		eic: ec2instanceconnect.NewFromConfig(cfg),
		sts: sts.NewFromConfig(cfg),
		iam: iam.NewFromConfig(cfg),
	}
	return cl, cfg.Region, describeAccess(profile, creds.Source), identityLookup(ctx, cl), nil
}

// UseProfile (re)connects with profile and records the outcome.
func (p *Provider) UseProfile(ctx context.Context, profile string) error {
	if p.demo != nil {
		return p.useDemo(profile)
	}
	cl, region, access, id, err := buildClients(ctx, profile, p.regionFlag)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.profileInUse = profile
	if err != nil {
		p.cl, p.authErr, p.region, p.access, p.identity = nil, err, "", "not signed in", provider.Identity{}
		return err
	}
	p.cl, p.authErr, p.region, p.access, p.identity = cl, nil, region, access, id
	return nil
}

func (p *Provider) AuthError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.authErr
}

// Profile is the explicit profile, else AWS_PROFILE, else "" (env keys, instance role).
func (p *Provider) Profile() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.profileInUse != "" {
		return p.profileInUse
	}
	return os.Getenv("AWS_PROFILE")
}

// Access says how we are signed in ("SSO (prod)", "environment keys", ...).
func (p *Provider) Access() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.access
}

func (p *Provider) Region() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.region
}

// clients returns the live clients, or the friendly reason there are none.
func (p *Provider) clients() (*clients, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.authErr != nil {
		return nil, p.authErr
	}
	if p.cl == nil {
		return nil, errNotConnected
	}
	return p.cl, nil
}

func (p *Provider) Name() string { return "aws" }

// Resources returns the top-level modes in shortcut order: 1=ec2 2=ecs 3=eks 4=asg 5=vpc.
func (p *Provider) Resources() []provider.Resource {
	return []provider.Resource{
		&instances{p: p},
		&ecsClusters{p: p},
		&eksClusters{p: p},
		&asgs{p: p},
		&vpcs{p: p},
	}
}

// size caps the configured page size to an API's maximum.
func (p *Provider) size(limit int32) int32 { return min(p.pageSize, limit) }

// ResolveHost fills Host.DefaultUser from the instance's AMI description.
func (p *Provider) ResolveHost(ctx context.Context, h provider.Host) (provider.Host, error) {
	if h.ImageID == "" || h.DefaultUser != "" {
		return h, nil
	}
	cl, err := p.clients()
	if err != nil {
		return h, err
	}
	out, err := cl.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds:          []string{h.ImageID},
		IncludeDeprecated: awssdk.Bool(true),
		IncludeDisabled:   awssdk.Bool(true),
	})
	if err != nil {
		return h, fmt.Errorf("describe image %s: %w", h.ImageID, err)
	}
	if len(out.Images) > 0 {
		h.DefaultUser = UserForImage(awssdk.ToString(out.Images[0].Description))
	}
	return h, nil
}

// UserForImage guesses the default SSH user from an AMI description.
func UserForImage(description string) string {
	d := strings.ToLower(description)
	switch {
	case strings.Contains(d, "ubuntu"):
		return "ubuntu"
	case strings.Contains(d, "debian"):
		return "admin"
	case strings.Contains(d, "rocky linux"):
		return "rocky"
	case strings.Contains(d, "bitnami"):
		return "bitnami"
	default: // amazon linux, oracle, centos, fedora, rhel, suse, unknown (root is refused on RHEL/SUSE AMIs)
		return "ec2-user"
	}
}

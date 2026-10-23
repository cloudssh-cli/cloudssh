// Package aws implements provider.Provider on top of aws-sdk-go-v2.
package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type Provider struct {
	ec2      *ec2.Client
	pageSize int32
}

// New loads credentials via the standard AWS chain (env, ~/.aws, SSO, IMDS).
func New(ctx context.Context, profile, region string, pageSize int32) (*Provider, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &Provider{ec2: ec2.NewFromConfig(cfg), pageSize: pageSize}, nil
}

func (p *Provider) Name() string { return "aws" }

func (p *Provider) Resources() []provider.Resource {
	return []provider.Resource{&instances{p: p}}
}

// ResolveHost fills Host.DefaultUser from the instance's AMI description.
func (p *Provider) ResolveHost(ctx context.Context, h provider.Host) (provider.Host, error) {
	if h.ImageID == "" || h.DefaultUser != "" {
		return h, nil
	}
	out, err := p.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
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
	case strings.Contains(d, "centos"):
		return "centos"
	case strings.Contains(d, "fedora"):
		return "fedora"
	case strings.Contains(d, "rocky linux"):
		return "rocky"
	case strings.Contains(d, "bitnami"):
		return "bitnami"
	case strings.Contains(d, "rhel"), strings.Contains(d, "suse"):
		return "root"
	default: // amazon linux, oracle, unknown
		return "ec2-user"
	}
}

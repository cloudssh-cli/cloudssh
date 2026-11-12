package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// demoState is set on a Provider running on built-in sample data.
type demoState struct{ accounts []*demoAccount }

// Open is the cloud factory (see internal/cloud): a real provider, or the
// demo provider when o.Demo is set.
func Open(ctx context.Context, o provider.Options) (provider.Provider, error) {
	if o.Demo {
		p := NewDemo(o.PageSize)
		if o.Profile != "" {
			_ = p.UseProfile(ctx, o.Profile) // an unknown demo profile shows the friendly signed-out screen
		}
		return p, nil
	}
	return New(ctx, o.Profile, o.Region, o.PageSize)
}

// NewDemo returns a provider backed by in-memory sample accounts. The real
// resource code runs unchanged against fake SDK clients (demo_api.go), so the
// demo cannot drift from what the provider really maps.
func NewDemo(pageSize int32) *Provider {
	d := &demoState{accounts: buildDemoAccounts(time.Now())}
	p := &Provider{pageSize: pageSize, demo: d}
	_ = p.UseProfile(context.Background(), d.accounts[0].profile.Name)
	return p
}

// Demo reports whether this provider runs on sample data.
func (p *Provider) Demo() bool { return p.demo != nil }

func (d *demoState) clients(a *demoAccount) *clients {
	return &clients{ec2: &demoEC2{a}, asg: &demoASG{a}, ecs: &demoECS{a}, eks: &demoEKS{a}, eic: &demoEIC{a}, sts: &demoSTS{a}, iam: &demoIAM{a}}
}

// useDemo switches to the demo account named profile.
func (p *Provider) useDemo(profile string) error {
	for _, a := range p.demo.accounts {
		if a.profile.Name == profile {
			cl := p.demo.clients(a)
			id := resolveIdentity(context.Background(), cl) // the same lookup the real provider runs, against the fake STS/IAM
			p.mu.Lock()
			defer p.mu.Unlock()
			p.profileInUse, p.cl, p.authErr, p.region, p.access, p.identity = profile, cl, nil, a.region, a.access, id
			return nil
		}
	}
	err := &authProblem{
		headline: fmt.Sprintf("There is no demo profile called %q.", profile),
		lines:    []string{"The demo has: " + demoProfileNames(p.demo) + ". Pick one from the list."},
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.profileInUse, p.cl, p.authErr, p.region, p.access, p.identity = profile, nil, err, "", "not signed in", provider.Identity{}
	return err
}

func demoProfileNames(d *demoState) string {
	out := ""
	for i, a := range d.accounts {
		if i > 0 {
			out += ", "
		}
		out += a.profile.Name
	}
	return out
}

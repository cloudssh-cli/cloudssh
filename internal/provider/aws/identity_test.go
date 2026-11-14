package aws

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

func TestPrincipalFromARN(t *testing.T) {
	for arn, want := range map[string]string{
		"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdministratorAccess_8f1c2d3e4a5b6c7d/me": "role AdministratorAccess",
		"arn:aws:sts::123456789012:assumed-role/Deployer/ci-session":                                    "role Deployer",
		"arn:aws:iam::123456789012:user/ci/deploy-bot":                                                  "user deploy-bot",
		"arn:aws:iam::123456789012:user/alice":                                                          "user alice",
		"arn:aws:iam::123456789012:root":                                                                "root",
		"arn:aws:sts::123456789012:federated-user/bob":                                                  "federated bob",
		"garbage": "", "": "",
	} {
		if got := principalFromARN(arn); got != want {
			t.Errorf("%q -> %q, want %q", arn, got, want)
		}
	}
}

type fakeSTS struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f.out, f.err
}

type fakeIAM struct {
	aliases []string
	err     error
}

func (f fakeIAM) ListAccountAliases(context.Context, *iam.ListAccountAliasesInput, ...func(*iam.Options)) (*iam.ListAccountAliasesOutput, error) {
	return &iam.ListAccountAliasesOutput{AccountAliases: f.aliases}, f.err
}

func TestResolveIdentityIsBestEffort(t *testing.T) {
	ctx := context.Background()
	who := &sts.GetCallerIdentityOutput{Account: awssdk.String("111122223333"), Arn: awssdk.String("arn:aws:iam::111122223333:user/alice")}
	if got := resolveIdentity(ctx, &clients{sts: fakeSTS{out: who}, iam: fakeIAM{aliases: []string{"northwind"}}}); got != (provider.Identity{Account: "111122223333", Alias: "northwind", Principal: "user alice"}) {
		t.Fatalf("full: %+v", got)
	}
	if got := resolveIdentity(ctx, &clients{sts: fakeSTS{out: who}, iam: fakeIAM{err: errors.New("AccessDenied")}}); got.Alias != "" || got.Account != "111122223333" {
		t.Fatalf("a denied alias lookup keeps the rest: %+v", got)
	}
	if got := resolveIdentity(ctx, &clients{sts: fakeSTS{err: errors.New("offline")}, iam: fakeIAM{aliases: []string{"x"}}}); got != (provider.Identity{}) {
		t.Fatalf("no STS means no identity (and no alias lookup): %+v", got)
	}
	if got := resolveIdentity(ctx, &clients{}); got != (provider.Identity{}) {
		t.Fatalf("no clients: %+v", got)
	}
}

func TestDemoAccountsReportTheirIdentity(t *testing.T) {
	p := NewDemo(8)
	want := map[string]provider.Identity{
		"cloudssh-prod":    {Account: "123456789012", Alias: "cloudssh-production", Principal: "role AdministratorAccess"},
		"cloudssh-staging": {Account: "210987654321", Alias: "cloudssh-staging", Principal: "role PowerUserAccess"},
		"cloudssh-sandbox": {Account: "555566667777", Alias: "cloudssh-sandbox", Principal: "user deploy-bot"},
	}
	for name, id := range want {
		if err := p.UseProfile(context.Background(), name); err != nil {
			t.Fatal(err)
		}
		if got := p.Identity(); got != id {
			t.Errorf("%s: %+v, want %+v", name, got, id)
		}
	}
}

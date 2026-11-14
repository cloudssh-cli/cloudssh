package aws

import (
	"context"
	"errors"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

type fakeEIC struct {
	in  *ec2instanceconnect.SendSSHPublicKeyInput
	out *ec2instanceconnect.SendSSHPublicKeyOutput
	err error
}

func (f *fakeEIC) SendSSHPublicKey(_ context.Context, in *ec2instanceconnect.SendSSHPublicKeyInput, _ ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error) {
	f.in = in
	return f.out, f.err
}

func TestAuthorizeSSHSendsKeyForInstanceAndUser(t *testing.T) {
	f := &fakeEIC{out: &ec2instanceconnect.SendSSHPublicKeyOutput{Success: true}}
	p := withEIC(f)
	h := provider.Host{ID: "i-1", AZ: "us-east-1a"}
	if err := p.AuthorizeSSH(context.Background(), h, "ubuntu", "ssh-ed25519 AAAA"); err != nil {
		t.Fatal(err)
	}
	if awssdk.ToString(f.in.InstanceId) != "i-1" || awssdk.ToString(f.in.InstanceOSUser) != "ubuntu" ||
		awssdk.ToString(f.in.AvailabilityZone) != "us-east-1a" || awssdk.ToString(f.in.SSHPublicKey) != "ssh-ed25519 AAAA" {
		t.Fatalf("request: %+v", f.in)
	}
}

func TestAuthorizeSSHErrors(t *testing.T) {
	ctx := context.Background()
	if err := withEIC(&fakeEIC{}).AuthorizeSSH(ctx, provider.Host{ID: "i-1"}, "u", "k"); err == nil || !strings.Contains(err.Error(), "availability zone") {
		t.Fatalf("missing AZ should fail clearly, got %v", err)
	}
	p := withEIC(&fakeEIC{err: errors.New("AccessDenied")})
	if err := p.AuthorizeSSH(ctx, provider.Host{ID: "i-1", AZ: "a"}, "u", "k"); err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("API error should surface, got %v", err)
	}
	p = withEIC(&fakeEIC{out: &ec2instanceconnect.SendSSHPublicKeyOutput{Success: false}})
	if err := p.AuthorizeSSH(ctx, provider.Host{ID: "i-1", AZ: "a"}, "u", "k"); err == nil {
		t.Fatal("Success=false must be an error")
	}
}

func TestInstanceRowCarriesAZForInstanceConnect(t *testing.T) {
	// AZ must travel with the host or SendSSHPublicKey cannot be called
	row := instanceRow(testInstanceInAZ("us-east-1b"))
	if h := row.Ref.(provider.Host); h.AZ != "us-east-1b" || h.ID == "" {
		t.Fatalf("host: %+v", h)
	}
}

// withEIC is a connected provider whose only client is the given fake.
func withEIC(c instanceConnect) *Provider { return &Provider{cl: &clients{eic: c}} }

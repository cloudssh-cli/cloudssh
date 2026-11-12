package aws

import (
	"context"
	"errors"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// instanceConnect is the slice of the EC2 Instance Connect client we use.
type instanceConnect interface {
	SendSSHPublicKey(ctx context.Context, in *ec2instanceconnect.SendSSHPublicKeyInput,
		opts ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error)
}

var _ provider.Authorizer = (*Provider)(nil)

func (*Provider) AuthLabel() string { return "EC2 Instance Connect" }

// AuthorizeSSH pushes publicKey to the instance for user. The key is valid for
// about 60 seconds, so connect right after. Needs ec2-instance-connect:SendSSHPublicKey.
func (p *Provider) AuthorizeSSH(ctx context.Context, h provider.Host, user, publicKey string) error {
	if h.ID == "" || h.AZ == "" {
		return errors.New("instance id / availability zone unknown")
	}
	cl, err := p.clients()
	if err != nil {
		return err
	}
	out, err := cl.eic.SendSSHPublicKey(ctx, &ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:       awssdk.String(h.ID),
		InstanceOSUser:   awssdk.String(user),
		SSHPublicKey:     awssdk.String(publicKey),
		AvailabilityZone: awssdk.String(h.AZ),
	})
	if err != nil {
		return fmt.Errorf("send ssh public key: %w", err)
	}
	if out != nil && !out.Success {
		return errors.New("EC2 Instance Connect refused the key")
	}
	return nil
}

package aws

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

func TestInstanceRowMapsFields(t *testing.T) {
	row := instanceRow(types.Instance{
		InstanceId:       awssdk.String("i-123"),
		InstanceType:     types.InstanceTypeT3Micro,
		PrivateIpAddress: awssdk.String("10.0.0.5"),
		KeyName:          awssdk.String("prod"),
		ImageId:          awssdk.String("ami-1"),
		Placement:        &types.Placement{AvailabilityZone: awssdk.String("us-east-1a")},
		State:            &types.InstanceState{Name: types.InstanceStateNameRunning},
		Tags: []types.Tag{
			{Key: awssdk.String("Name"), Value: awssdk.String("web")},
			{Key: awssdk.String("aws:autoscaling:groupName"), Value: awssdk.String("web-asg")},
		},
	})
	want := []string{"i-123", "web", "t3.micro", "us-east-1a", "10.0.0.5", "", "web-asg", "running"}
	if len(row.Cells) != len(want) {
		t.Fatalf("cells = %v", row.Cells)
	}
	for i := range want {
		if row.Cells[i] != want[i] {
			t.Errorf("cell %d = %q, want %q", i, row.Cells[i], want[i])
		}
	}
	h, ok := row.Ref.(provider.Host)
	if !ok || h.ID != "i-123" || h.KeyName != "prod" || h.ImageID != "ami-1" || h.PublicIP != "" {
		t.Fatalf("bad host ref: %+v", row.Ref)
	}
}

func TestInstanceRowToleratesMissingOptionalFields(t *testing.T) {
	row := instanceRow(types.Instance{InstanceId: awssdk.String("i-1")})
	if len(row.Cells) != len((&instances{}).Columns()) {
		t.Fatalf("cells/columns mismatch: %d", len(row.Cells))
	}
}

func testInstanceInAZ(az string) types.Instance {
	return types.Instance{InstanceId: awssdk.String("i-9"), Placement: &types.Placement{AvailabilityZone: awssdk.String(az)}}
}

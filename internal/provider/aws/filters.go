package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// filterSpec maps a UI filter to an EC2 API filter name.
type filterSpec struct {
	key, name string // provider.FilterDef
	api       string // EC2 filter name
	exact     bool   // exact match; otherwise the value is wrapped in *...*
}

// shortNames are the compact header labels ("name=web").
var shortNames = map[string]string{
	"instance-name": "name", "auto-scaling-group": "asg", "private-ip": "ip", "instance-type": "type", "public-ip": "public-ip",
	"availability-zone": "az", "state": "state", "instance-id": "id", "subnet-id": "subnet",
	"vpc-name": "name", "vpc-id": "vpc", "vpc-cidr": "cidr", "vpc-default": "default", "vpc-state": "state", "subnet-name": "name",
}

func defsOf(specs []filterSpec) []provider.FilterDef {
	out := make([]provider.FilterDef, len(specs))
	for i, s := range specs {
		out[i] = provider.FilterDef{Key: s.key, Name: s.name, Short: shortNames[s.key]}
	}
	return out
}

// buildFilters converts UI filters to EC2 filters. Empty values and unknown
// keys are skipped.
func buildFilters(specs []filterSpec, fs []provider.Filter) []types.Filter {
	var out []types.Filter
	for _, f := range fs {
		if f.Value == "" {
			continue
		}
		for _, s := range specs {
			if s.key != f.Key {
				continue
			}
			v := "*" + f.Value + "*"
			if s.exact {
				v = f.Value
			}
			name := s.api
			out = append(out, types.Filter{Name: &name, Values: []string{v}})
			break
		}
	}
	return out
}

var ec2Specs = []filterSpec{
	{"instance-name", "Instance Name", "tag:Name", false},
	{"auto-scaling-group", "AutoScaling Group Name", "tag:aws:autoscaling:groupName", false},
	{"private-ip", "Private IP", "private-ip-address", false},
	{"instance-type", "Instance Type", "instance-type", false},
	{"public-ip", "Public IP", "network-interface.association.public-ip", false},
	{"availability-zone", "Availability Zone", "availability-zone", false},
	{"state", "Instance State", "instance-state-name", false},
	{"instance-id", "Instance ID", "instance-id", true},
	{"subnet-id", "Subnet ID", "subnet-id", true},
}

var vpcSpecs = []filterSpec{
	{"vpc-name", "Vpc Name", "tag:Name", false},
	{"vpc-id", "Vpc ID", "vpc-id", true},
	{"vpc-cidr", "Vpc Cidr", "cidr", false},
	{"vpc-default", "Is Default Vpc", "is-default", false},
	{"vpc-state", "Vpc State", "state", false},
}

var subnetSpecs = []filterSpec{
	{"subnet-name", "Subnet Name", "tag:Name", false},
	{"subnet-id", "Subnet ID", "subnet-id", true},
	{"vpc-id", "Vpc ID", "vpc-id", true},
}

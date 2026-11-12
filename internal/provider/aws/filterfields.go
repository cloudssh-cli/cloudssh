package aws

import (
	"regexp"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Field lookups shared by exclusion filtering (filters.go) and the demo fake
// (demo_api.go), so both read an item's fields the same way.

// wild matches s against an EC2-style filter value where * is a wildcard.
func wild(pattern, s string) bool {
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	ok, _ := regexp.MatchString(re, s)
	return ok
}

func tagValues(tags []ec2types.Tag, key string) []string {
	var out []string
	for _, t := range tags {
		if awssdk.ToString(t.Key) == key {
			out = append(out, awssdk.ToString(t.Value))
		}
	}
	return out
}

func instanceField(i ec2types.Instance, name string) ([]string, bool) {
	if k, ok := strings.CutPrefix(name, "tag:"); ok {
		return tagValues(i.Tags, k), true
	}
	switch name {
	case "instance-id":
		return []string{awssdk.ToString(i.InstanceId)}, true
	case "instance-type":
		return []string{string(i.InstanceType)}, true
	case "instance-state-name":
		return []string{string(i.State.Name)}, true
	case "availability-zone":
		return []string{awssdk.ToString(i.Placement.AvailabilityZone)}, true
	case "private-ip-address":
		return []string{awssdk.ToString(i.PrivateIpAddress)}, true
	case "network-interface.association.public-ip":
		return []string{awssdk.ToString(i.PublicIpAddress)}, true
	case "subnet-id":
		return []string{awssdk.ToString(i.SubnetId)}, true
	case "vpc-id":
		return []string{awssdk.ToString(i.VpcId)}, true
	}
	return nil, false
}

func vpcField(v ec2types.Vpc, name string) ([]string, bool) {
	if k, ok := strings.CutPrefix(name, "tag:"); ok {
		return tagValues(v.Tags, k), true
	}
	switch name {
	case "vpc-id":
		return []string{awssdk.ToString(v.VpcId)}, true
	case "cidr":
		return []string{awssdk.ToString(v.CidrBlock)}, true
	case "is-default":
		return []string{strconv.FormatBool(awssdk.ToBool(v.IsDefault))}, true
	case "state":
		return []string{string(v.State)}, true
	}
	return nil, false
}

func subnetField(s ec2types.Subnet, name string) ([]string, bool) {
	if k, ok := strings.CutPrefix(name, "tag:"); ok {
		return tagValues(s.Tags, k), true
	}
	switch name {
	case "subnet-id":
		return []string{awssdk.ToString(s.SubnetId)}, true
	case "vpc-id":
		return []string{awssdk.ToString(s.VpcId)}, true
	case "availability-zone":
		return []string{awssdk.ToString(s.AvailabilityZone)}, true
	}
	return nil, false
}

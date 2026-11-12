package aws

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/smithy-go"
)

// In-memory implementations of the SDK interfaces in api.go over a
// demoAccount. They deliberately behave like the real services where the
// provider could trip over it: page limits and tokens, ECS's "at most 10
// services per describe", EC2's MaxResults range, wildcard filter matching and
// -- importantly -- a hard error for any filter name they do not implement, so
// production code that starts using a new filter fails a test instead of
// silently showing empty demo data.

func apiErr(code, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// page slices [lo,hi) out of total items honouring an opaque token and a max.
func page(total int, token *string, limit int32) (lo, hi int, next *string, err error) {
	if token != nil && *token != "" {
		n, convErr := strconv.Atoi(strings.TrimPrefix(*token, "demo-"))
		if convErr != nil || n < 0 || n > total {
			return 0, 0, nil, apiErr("InvalidNextToken", "The provided token is not valid.")
		}
		lo = n
	}
	size := total
	if limit > 0 {
		size = int(limit)
	}
	hi = min(lo+size, total)
	if hi < total {
		t := "demo-" + strconv.Itoa(hi)
		next = &t
	}
	return lo, hi, next, nil
}

// wild matches s against an EC2-style filter value where * is a wildcard.
func wild(pattern, s string) bool {
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	ok, _ := regexp.MatchString(re, s)
	return ok
}

// applyFilters keeps items matching every filter (values OR-ed within one).
// field returns the values an item has for a filter name; ok=false rejects
// unknown names like the real API does.
func applyFilters[T any](items []T, filters []ec2types.Filter, field func(T, string) ([]string, bool)) ([]T, error) {
	if len(items) == 0 {
		return nil, nil
	}
	for _, f := range filters {
		if _, ok := field(items[0], awssdk.ToString(f.Name)); !ok {
			return nil, apiErr("InvalidParameterValue", "The filter '%s' is invalid", awssdk.ToString(f.Name))
		}
	}
	var out []T
next:
	for _, it := range items {
		for _, f := range filters {
			have, _ := field(it, awssdk.ToString(f.Name))
			hit := false
			for _, want := range f.Values {
				for _, h := range have {
					hit = hit || wild(want, h)
				}
			}
			if !hit {
				continue next
			}
		}
		out = append(out, it)
	}
	return out, nil
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

func checkEC2MaxResults(n *int32) error {
	if n != nil && (*n < 5 || *n > 1000) {
		return apiErr("InvalidParameterValue", "Value (%d) for parameter maxResults is invalid. Expecting a value between 5 and 1000.", *n)
	}
	return nil
}

// ---- EC2 ----

type demoEC2 struct{ a *demoAccount }

func (d *demoEC2) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if err := checkEC2MaxResults(in.MaxResults); err != nil {
		return nil, err
	}
	matched, err := applyFilters(d.a.instances, in.Filters, instanceField)
	if err != nil {
		return nil, err
	}
	if len(in.InstanceIds) > 0 {
		var byID []ec2types.Instance
		for _, i := range matched {
			for _, id := range in.InstanceIds {
				if awssdk.ToString(i.InstanceId) == id {
					byID = append(byID, i)
				}
			}
		}
		matched = byID
	}
	lo, hi, next, err := page(len(matched), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	out := &ec2.DescribeInstancesOutput{NextToken: next}
	for n, i := range matched[lo:hi] {
		rid := mix(uint64(lo+n)) + 99 //nolint:gosec // small non-negative index
		out.Reservations = append(out.Reservations, ec2types.Reservation{
			ReservationId: awssdk.String(fmt.Sprintf("r-0%016x", rid)), OwnerId: awssdk.String(d.a.accounts), Instances: []ec2types.Instance{i},
		})
	}
	return out, nil
}

func (d *demoEC2) DescribeVpcs(_ context.Context, in *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if err := checkEC2MaxResults(in.MaxResults); err != nil {
		return nil, err
	}
	matched, err := applyFilters(d.a.vpcs, in.Filters, vpcField)
	if err != nil {
		return nil, err
	}
	lo, hi, next, err := page(len(matched), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &ec2.DescribeVpcsOutput{Vpcs: matched[lo:hi], NextToken: next}, nil
}

func (d *demoEC2) DescribeSubnets(_ context.Context, in *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	if err := checkEC2MaxResults(in.MaxResults); err != nil {
		return nil, err
	}
	matched, err := applyFilters(d.a.subnets, in.Filters, subnetField)
	if err != nil {
		return nil, err
	}
	lo, hi, next, err := page(len(matched), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &ec2.DescribeSubnetsOutput{Subnets: matched[lo:hi], NextToken: next}, nil
}

func (d *demoEC2) DescribeImages(_ context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	out := &ec2.DescribeImagesOutput{}
	for _, id := range in.ImageIds {
		img, ok := d.a.images[id]
		if !ok {
			return nil, apiErr("InvalidAMIID.NotFound", "The image id '[%s]' does not exist", id)
		}
		out.Images = append(out.Images, img)
	}
	return out, nil
}

// ---- Auto Scaling ----

type demoASG struct{ a *demoAccount }

func (d *demoASG) DescribeAutoScalingGroups(_ context.Context, in *autoscaling.DescribeAutoScalingGroupsInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error) {
	if in.MaxRecords != nil && (*in.MaxRecords < 1 || *in.MaxRecords > 100) {
		return nil, apiErr("ValidationError", "1 validation error detected: Value '%d' at 'maxRecords' failed to satisfy constraint: Member must have value less than or equal to 100", *in.MaxRecords)
	}
	var matched []asgtypes.AutoScalingGroup
	for _, g := range d.a.asgs {
		if len(in.AutoScalingGroupNames) == 0 || contains(in.AutoScalingGroupNames, awssdk.ToString(g.AutoScalingGroupName)) {
			matched = append(matched, g)
		}
	}
	lo, hi, next, err := page(len(matched), in.NextToken, awssdk.ToInt32(in.MaxRecords))
	if err != nil {
		return nil, err
	}
	return &autoscaling.DescribeAutoScalingGroupsOutput{AutoScalingGroups: matched[lo:hi], NextToken: next}, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---- ECS ----

type demoECS struct{ a *demoAccount }

func ecsLimit(n *int32) error {
	if n != nil && (*n < 1 || *n > 100) {
		return apiErr("InvalidParameterException", "maxResults must be between 1 and 100")
	}
	return nil
}

func (d *demoECS) clusterByName(name string) (ecstypes.Cluster, bool) {
	for _, c := range d.a.clusters {
		if awssdk.ToString(c.ClusterName) == name || awssdk.ToString(c.ClusterArn) == name {
			return c, true
		}
	}
	return ecstypes.Cluster{}, false
}

func (d *demoECS) ListClusters(_ context.Context, in *ecs.ListClustersInput, _ ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	if err := ecsLimit(in.MaxResults); err != nil {
		return nil, err
	}
	var arns []string
	for _, c := range d.a.clusters {
		arns = append(arns, awssdk.ToString(c.ClusterArn))
	}
	lo, hi, next, err := page(len(arns), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &ecs.ListClustersOutput{ClusterArns: arns[lo:hi], NextToken: next}, nil
}

func (d *demoECS) DescribeClusters(_ context.Context, in *ecs.DescribeClustersInput, _ ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	if len(in.Clusters) > 100 {
		return nil, apiErr("InvalidParameterException", "Clusters cannot have more than 100 items")
	}
	out := &ecs.DescribeClustersOutput{}
	for _, name := range in.Clusters {
		if c, ok := d.clusterByName(name); ok {
			out.Clusters = append(out.Clusters, c)
		} else {
			out.Failures = append(out.Failures, ecstypes.Failure{Arn: awssdk.String(name), Reason: awssdk.String("MISSING")})
		}
	}
	return out, nil
}

func (d *demoECS) ListServices(_ context.Context, in *ecs.ListServicesInput, _ ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	if err := ecsLimit(in.MaxResults); err != nil {
		return nil, err
	}
	c, ok := d.clusterByName(awssdk.ToString(in.Cluster))
	if !ok {
		return nil, apiErr("ClusterNotFoundException", "Cluster not found.")
	}
	var arns []string
	for _, s := range d.a.services[awssdk.ToString(c.ClusterName)] {
		arns = append(arns, awssdk.ToString(s.ServiceArn))
	}
	lo, hi, next, err := page(len(arns), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &ecs.ListServicesOutput{ServiceArns: arns[lo:hi], NextToken: next}, nil
}

func (d *demoECS) DescribeServices(_ context.Context, in *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	if len(in.Services) == 0 {
		return nil, apiErr("InvalidParameterException", "Services cannot be empty.")
	}
	if len(in.Services) > 10 { // the limit real callers must chunk for
		return nil, apiErr("InvalidParameterException", "Services cannot have more than 10 items.")
	}
	c, ok := d.clusterByName(awssdk.ToString(in.Cluster))
	if !ok {
		return nil, apiErr("ClusterNotFoundException", "Cluster not found.")
	}
	out := &ecs.DescribeServicesOutput{}
	for _, want := range in.Services {
		found := false
		for _, s := range d.a.services[awssdk.ToString(c.ClusterName)] {
			if awssdk.ToString(s.ServiceArn) == want || awssdk.ToString(s.ServiceName) == want {
				out.Services, found = append(out.Services, s), true
			}
		}
		if !found {
			out.Failures = append(out.Failures, ecstypes.Failure{Arn: awssdk.String(want), Reason: awssdk.String("MISSING")})
		}
	}
	return out, nil
}

func (d *demoECS) ListTasks(_ context.Context, in *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	if err := ecsLimit(in.MaxResults); err != nil {
		return nil, err
	}
	c, ok := d.clusterByName(awssdk.ToString(in.Cluster))
	if !ok {
		return nil, apiErr("ClusterNotFoundException", "Cluster not found.")
	}
	var arns []string
	name := awssdk.ToString(c.ClusterName)
	var keys []string
	for k := range d.a.tasks {
		if strings.HasPrefix(k, name+"/") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if in.ServiceName != nil && k != name+"/"+*in.ServiceName {
			continue
		}
		for _, t := range d.a.tasks[k] {
			arns = append(arns, awssdk.ToString(t.TaskArn))
		}
	}
	lo, hi, next, err := page(len(arns), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &ecs.ListTasksOutput{TaskArns: arns[lo:hi], NextToken: next}, nil
}

func (d *demoECS) DescribeTasks(_ context.Context, in *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	if len(in.Tasks) == 0 {
		return nil, apiErr("InvalidParameterException", "Tasks cannot be empty.")
	}
	if len(in.Tasks) > 100 {
		return nil, apiErr("InvalidParameterException", "Tasks cannot have more than 100 items.")
	}
	out := &ecs.DescribeTasksOutput{}
	for _, want := range in.Tasks {
		found := false
		for _, ts := range d.a.tasks {
			for _, t := range ts {
				if awssdk.ToString(t.TaskArn) == want {
					out.Tasks, found = append(out.Tasks, t), true
				}
			}
		}
		if !found {
			out.Failures = append(out.Failures, ecstypes.Failure{Arn: awssdk.String(want), Reason: awssdk.String("MISSING")})
		}
	}
	return out, nil
}

func (d *demoECS) DescribeContainerInstances(_ context.Context, in *ecs.DescribeContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error) {
	c, ok := d.clusterByName(awssdk.ToString(in.Cluster))
	if !ok {
		return nil, apiErr("ClusterNotFoundException", "Cluster not found.")
	}
	out := &ecs.DescribeContainerInstancesOutput{}
	for _, want := range in.ContainerInstances {
		for _, ci := range d.a.containers[awssdk.ToString(c.ClusterName)] {
			if awssdk.ToString(ci.ContainerInstanceArn) == want {
				out.ContainerInstances = append(out.ContainerInstances, ci)
			}
		}
	}
	return out, nil
}

// ---- EKS ----

type demoEKS struct{ a *demoAccount }

func (d *demoEKS) ListClusters(_ context.Context, in *eks.ListClustersInput, _ ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 100) {
		return nil, apiErr("InvalidParameterException", "maxResults must be between 1 and 100")
	}
	var names []string
	for _, c := range d.a.eksClusters {
		names = append(names, awssdk.ToString(c.Name))
	}
	lo, hi, next, err := page(len(names), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &eks.ListClustersOutput{Clusters: names[lo:hi], NextToken: next}, nil
}

func (d *demoEKS) DescribeCluster(_ context.Context, in *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	for _, c := range d.a.eksClusters {
		if awssdk.ToString(c.Name) == awssdk.ToString(in.Name) {
			return &eks.DescribeClusterOutput{Cluster: &c}, nil
		}
	}
	return nil, apiErr("ResourceNotFoundException", "No cluster found for name: %s.", awssdk.ToString(in.Name))
}

func (d *demoEKS) ListNodegroups(_ context.Context, in *eks.ListNodegroupsInput, _ ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error) {
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 100) {
		return nil, apiErr("InvalidParameterException", "maxResults must be between 1 and 100")
	}
	var names []string
	for _, n := range d.a.nodegroups[awssdk.ToString(in.ClusterName)] {
		names = append(names, awssdk.ToString(n.NodegroupName))
	}
	lo, hi, next, err := page(len(names), in.NextToken, awssdk.ToInt32(in.MaxResults))
	if err != nil {
		return nil, err
	}
	return &eks.ListNodegroupsOutput{Nodegroups: names[lo:hi], NextToken: next}, nil
}

func (d *demoEKS) DescribeNodegroup(_ context.Context, in *eks.DescribeNodegroupInput, _ ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	for _, n := range d.a.nodegroups[awssdk.ToString(in.ClusterName)] {
		if awssdk.ToString(n.NodegroupName) == awssdk.ToString(in.NodegroupName) {
			return &eks.DescribeNodegroupOutput{Nodegroup: &n}, nil
		}
	}
	return nil, apiErr("ResourceNotFoundException", "No node group found for name: %s.", awssdk.ToString(in.NodegroupName))
}

// ---- EC2 Instance Connect ----

type demoEIC struct{ a *demoAccount }

func (d *demoEIC) SendSSHPublicKey(_ context.Context, in *ec2instanceconnect.SendSSHPublicKeyInput, _ ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error) {
	for _, i := range d.a.instances {
		if awssdk.ToString(i.InstanceId) != awssdk.ToString(in.InstanceId) {
			continue
		}
		switch {
		case awssdk.ToString(i.Placement.AvailabilityZone) != awssdk.ToString(in.AvailabilityZone):
			return nil, apiErr("InvalidArgsException", "Invalid availability zone for instance %s.", awssdk.ToString(in.InstanceId))
		case awssdk.ToString(in.InstanceOSUser) == "":
			return nil, apiErr("InvalidArgsException", "InstanceOSUser is required.")
		case !strings.HasPrefix(awssdk.ToString(in.SSHPublicKey), "ssh-"):
			return nil, apiErr("InvalidArgsException", "The provided SSH public key is not valid.")
		case i.State.Name != ec2types.InstanceStateNameRunning:
			return nil, apiErr("EC2InstanceStateInvalidException", "Instance %s is not running.", awssdk.ToString(in.InstanceId))
		}
		return &ec2instanceconnect.SendSSHPublicKeyOutput{Success: true, RequestId: awssdk.String("demo-request")}, nil
	}
	return nil, apiErr("EC2InstanceNotFoundException", "Instance %s not found.", awssdk.ToString(in.InstanceId))
}

var (
	_ ecsAPI          = (*demoECS)(nil)
	_ eksAPI          = (*demoEKS)(nil)
	_ ec2API          = (*demoEC2)(nil)
	_ asgAPI          = (*demoASG)(nil)
	_ instanceConnect = (*demoEIC)(nil)
	_                 = ekstypes.ClusterStatusActive
)

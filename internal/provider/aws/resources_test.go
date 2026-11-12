package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func assertCells(t *testing.T, row provider.Row, res provider.Resource, want ...string) {
	t.Helper()
	if len(row.Cells) != len(res.Columns()) {
		t.Fatalf("%s: %d cells for %d columns", res.Key(), len(row.Cells), len(res.Columns()))
	}
	for i, w := range want {
		if row.Cells[i] != w {
			t.Errorf("%s cell %d = %q, want %q", res.Key(), i, row.Cells[i], w)
		}
	}
}

func TestBuildFilters(t *testing.T) {
	got := buildFilters(ec2Specs, []provider.Filter{
		{Key: "instance-name", Value: "web"}, // wildcard
		{Key: "instance-id", Value: "i-1"},   // exact
		{Key: "state", Value: ""},            // empty -> skipped
		{Key: "nope", Value: "x"},            // unknown -> skipped
	})
	if len(got) != 2 {
		t.Fatalf("got %d filters: %+v", len(got), got)
	}
	if awssdk.ToString(got[0].Name) != "tag:Name" || got[0].Values[0] != "*web*" {
		t.Errorf("wildcard filter: %+v", got[0])
	}
	if awssdk.ToString(got[1].Name) != "instance-id" || got[1].Values[0] != "i-1" {
		t.Errorf("exact filter: %+v", got[1])
	}
}

func TestEC2DefaultsToRunning(t *testing.T) {
	d := (instances{}).DefaultFilters()
	if len(d) != 1 || d[0].Key != "state" || d[0].Value != "running" {
		t.Fatalf("defaults: %+v", d)
	}
}

func TestEveryDrilledFilterKeyIsKnownToTarget(t *testing.T) {
	// the filters modal must be able to display what a drill applies
	known := map[string]bool{}
	for _, d := range (instances{}).Filters() {
		known[d.Key] = true
	}
	for _, k := range []string{"auto-scaling-group", "subnet-id", "instance-id"} {
		if !known[k] {
			t.Errorf("EC2 filters lack %q", k)
		}
	}
	known = map[string]bool{}
	for _, d := range (subnets{}).Filters() {
		known[d.Key] = true
	}
	if !known["vpc-id"] {
		t.Error("subnet filters lack vpc-id")
	}
}

func TestTopLevelOrderMatchesShortcuts(t *testing.T) {
	var keys []string
	for _, r := range (&Provider{}).Resources() {
		keys = append(keys, r.Key())
	}
	if strings.Join(keys, ",") != "ec2,ecs,eks,asg,vpc" {
		t.Fatalf("order: %v", keys)
	}
}

func TestASGRowAndDrill(t *testing.T) {
	r := &asgs{p: &Provider{}}
	row := asgRow(asgtypes.AutoScalingGroup{
		AutoScalingGroupName: awssdk.String("web"),
		DesiredCapacity:      awssdk.Int32(3), MinSize: awssdk.Int32(1), MaxSize: awssdk.Int32(5),
		Instances: []asgtypes.Instance{{}, {}},
	})
	assertCells(t, row, r, "web", "2", "3", "1/5")
	d, err := r.Drill(context.Background(), row)
	if err != nil || d.Resource.Name() != "EC2 · asg web" || d.Filters[0].Value != "web" {
		t.Fatalf("drill: %+v %v", d, err)
	}
}

func TestVPCAndSubnetDrill(t *testing.T) {
	v := &vpcs{p: &Provider{}}
	row := vpcRow(ec2types.Vpc{
		VpcId: awssdk.String("vpc-1"), CidrBlock: awssdk.String("10.0.0.0/16"), IsDefault: awssdk.Bool(true),
		Tags: []ec2types.Tag{{Key: awssdk.String("Name"), Value: awssdk.String("main")}},
	})
	assertCells(t, row, v, "main", "vpc-1", "10.0.0.0/16", "Yes")
	d, err := v.Drill(context.Background(), row)
	if err != nil || d.Resource.Crumb() != "subnets" || d.Filters[0] != (provider.Filter{Key: "vpc-id", Value: "vpc-1"}) {
		t.Fatalf("vpc drill: %+v %v", d, err)
	}

	s := &subnets{p: &Provider{}}
	srow := subnetRow(ec2types.Subnet{
		SubnetId: awssdk.String("subnet-1"), VpcId: awssdk.String("vpc-1"), AvailabilityZone: awssdk.String("us-east-1a"),
		CidrBlock: awssdk.String("10.0.1.0/24"), AvailableIpAddressCount: awssdk.Int32(250),
	})
	assertCells(t, srow, s, "", "subnet-1", "vpc-1", "us-east-1a", "10.0.1.0/24", "250")
	d, err = s.Drill(context.Background(), srow)
	if err != nil || d.Filters[0] != (provider.Filter{Key: "subnet-id", Value: "subnet-1"}) {
		t.Fatalf("subnet drill: %+v %v", d, err)
	}
}

func TestECSClusterRowAndGuards(t *testing.T) {
	r := &ecsClusters{p: &Provider{}}
	row := ecsClusterRow(ecstypes.Cluster{
		ClusterName: awssdk.String("prod"), ActiveServicesCount: 2, RunningTasksCount: 7, RegisteredContainerInstancesCount: 3,
	})
	assertCells(t, row, r, "prod", "2", "7", "3")
	if d, err := r.Drill(context.Background(), row); err != nil || d.Resource.Name() != "ECS · prod" {
		t.Fatalf("drill: %+v %v", d, err)
	}
	empty := ecsClusterRow(ecstypes.Cluster{ClusterName: awssdk.String("idle")})
	if _, err := r.Drill(context.Background(), empty); err == nil || !strings.Contains(err.Error(), "no services") {
		t.Fatalf("expected no-services error, got %v", err)
	}
}

func TestECSServiceRowAndGuards(t *testing.T) {
	r := &ecsServices{p: &Provider{}, cluster: "prod"}
	row := ecsServiceRow("prod", ecstypes.Service{
		ServiceName: awssdk.String("api"), Status: awssdk.String("ACTIVE"), SchedulingStrategy: ecstypes.SchedulingStrategyReplica,
		DesiredCount: 3, RunningCount: 2, TaskDefinition: awssdk.String("arn:aws:ecs:r:1:task-definition/api:42"),
		Deployments: []ecstypes.Deployment{{Status: awssdk.String("PRIMARY")}}, LaunchType: ecstypes.LaunchTypeEc2,
	})
	assertCells(t, row, r, "api", "ACTIVE", "REPLICA", "2 running / 3 desired", "PRIMARY", "api:42", "EC2")
	if d, err := r.Drill(context.Background(), row); err != nil || d.Resource.Name() != "ECS · prod › api" {
		t.Fatalf("drill: %+v %v", d, err)
	}
	idle := ecsServiceRow("prod", ecstypes.Service{ServiceName: awssdk.String("idle")}) // no deployments, no launch type
	assertCells(t, idle, r, "idle", "", "", "0 running / 0 desired", "unknown", "", "-")
	if _, err := r.Drill(context.Background(), idle); err == nil {
		t.Fatal("expected no-tasks error")
	}
}

func TestECSTaskRowFargateAndGuard(t *testing.T) {
	r := &ecsTasks{p: &Provider{}, cluster: "prod", service: "api"}
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	row := ecsTaskRow("prod", ecstypes.Task{
		TaskArn:    awssdk.String("arn:aws:ecs:r:1:task/prod/0123456789abcdef"),
		LastStatus: awssdk.String("RUNNING"), DesiredStatus: awssdk.String("RUNNING"),
		StartedAt: &ts, Cpu: awssdk.String("256"), Memory: awssdk.String("512"), LaunchType: ecstypes.LaunchTypeFargate,
	})
	if row.Cells[0] != "01234567" || row.Cells[1] != "RUNNING/RUNNING" || row.Cells[3] != "-" || row.Cells[5] != "0.25" || row.Cells[6] != "512 MB" {
		t.Fatalf("cells: %v", row.Cells)
	}
	if _, err := r.Drill(context.Background(), row); err == nil || !strings.Contains(err.Error(), "Fargate") {
		t.Fatalf("expected Fargate error, got %v", err)
	}
	pending := ecsTaskRow("prod", ecstypes.Task{TaskArn: awssdk.String("x")}) // nil StartedAt must not panic
	if pending.Cells[2] != "-" {
		t.Fatalf("started: %q", pending.Cells[2])
	}
}

func TestHumanUnits(t *testing.T) {
	for in, want := range map[string]string{"1024": "1", "2048": "2", "256": "0.25", "bad": "bad", "": "-"} {
		if got := humanCPU(in); got != want {
			t.Errorf("cpu %q = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"512": "512 MB", "1024": "1024 MB", "2048": "2 GB", "1536": "1.50 GB"} {
		if got := humanMemory(in); got != want {
			t.Errorf("mem %q = %q, want %q", in, got, want)
		}
	}
}

func TestChunk(t *testing.T) {
	got := chunk([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}, 10)
	if len(got) != 3 || len(got[0]) != 10 || len(got[2]) != 5 {
		t.Fatalf("chunks: %v", got)
	}
	if len(chunk([]int{}, 10)) != 0 {
		t.Fatal("empty input should yield no chunks")
	}
}

func TestEKSRowsAndDrills(t *testing.T) {
	c := &eksClusters{p: &Provider{}}
	crow := eksClusterRow(ekstypes.Cluster{
		Name: awssdk.String("kube"), Version: awssdk.String("1.30"), PlatformVersion: awssdk.String("eks.5"), Status: ekstypes.ClusterStatusActive,
	})
	assertCells(t, crow, c, "kube", "1.30", "eks.5", "ACTIVE")
	d, err := c.Drill(context.Background(), crow)
	if err != nil || d.Resource.Name() != "EKS · kube" {
		t.Fatalf("cluster drill: %+v %v", d, err)
	}

	n := &eksNodeGroups{p: &Provider{}, cluster: "kube"}
	nrow := nodeGroupRow(ekstypes.Nodegroup{
		NodegroupName: awssdk.String("ng1"), ReleaseVersion: awssdk.String("1.30.0-2024"), Status: ekstypes.NodegroupStatusActive,
		ScalingConfig: &ekstypes.NodegroupScalingConfig{DesiredSize: awssdk.Int32(3), MinSize: awssdk.Int32(1), MaxSize: awssdk.Int32(6)},
		Resources:     &ekstypes.NodegroupResources{AutoScalingGroups: []ekstypes.AutoScalingGroup{{Name: awssdk.String("eks-ng1-asg")}}},
	})
	assertCells(t, nrow, n, "ng1", "3", "1/6", "eks-ng1-asg", "1.30.0-2024", "ACTIVE")
	d, err = n.Drill(context.Background(), nrow)
	if err != nil || d.Filters[0].Value != "eks-ng1-asg" {
		t.Fatalf("ng drill: %+v %v", d, err)
	}
	bare := nodeGroupRow(ekstypes.Nodegroup{NodegroupName: awssdk.String("x")}) // no scaling config / resources
	assertCells(t, bare, n, "x", "0", "0/0", "-", "", "")
	if _, err := n.Drill(context.Background(), bare); err == nil {
		t.Fatal("expected no-asg error")
	}
}

// allResources lists every resource type of this package, top-level and
// drill-only. It is the single place to register a new resource: the metadata
// test and the demo-data conformance test (demo_test.go) both read it, so a
// resource without sample data fails the build.
func allResources() []provider.Resource {
	return []provider.Resource{
		&instances{}, &asgs{}, &vpcs{}, &subnets{}, &ecsClusters{}, &ecsServices{cluster: "c"}, &ecsTasks{cluster: "c", service: "s"},
		&eksClusters{}, &eksNodeGroups{cluster: "c"},
	}
}

func TestEveryResourceHasConsistentMetadata(t *testing.T) {
	for _, r := range allResources() {
		if r.Key() == "" || r.Name() == "" || r.Crumb() == "" || len(r.Columns()) == 0 {
			t.Errorf("%T has empty metadata", r)
		}
		if _, ok := r.(provider.Drillable); !ok && r.Key() != "ec2" {
			t.Errorf("%T should be drillable", r)
		}
	}
}

func TestTitlesPreferNamesOverIDs(t *testing.T) {
	if got := (subnets{vpc: "vpc-1", vpcName: "prod-main"}).Name(); got != "VPC · prod-main" {
		t.Errorf("a named VPC shows its name: %q", got)
	}
	if got := (subnets{vpc: "vpc-1"}).Name(); got != "VPC · vpc-1" {
		t.Errorf("an unnamed VPC shows its id: %q", got)
	}
	if got := subnetLabel(subnetRef{ID: "subnet-9", Name: "prod-public-a"}); got != "subnet prod-public-a" {
		t.Errorf("a named subnet: %q", got)
	}
	if got := subnetLabel(subnetRef{ID: "subnet-9"}); got != "subnet-9" {
		t.Errorf("an unnamed subnet must not say subnet twice: %q", got)
	}
	if got := (instances{sub: "ecs task 45ddf63e"}).Name(); got != "EC2 · ecs task 45ddf63e" {
		t.Errorf("task title: %q", got)
	}
}

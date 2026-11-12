package aws

import (
	"context"
	"net/netip"
	"regexp"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"

	"github.com/SVMadhavaReddy/cloudssh/internal/cloud"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider/providertest"
)

// The demo provider is checked by the shared suite every cloud must pass.
func TestDemoConformance(t *testing.T) {
	var want []string
	for _, r := range allResources() { // new resource => it must have sample data
		want = append(want, r.Key())
	}
	providertest.Conformance(t, NewDemo(8), providertest.Options{
		WantKeys: want,
		// ecs/eks clusters, services and tasks that have nothing to open are guarded drill-downs
		DrillErrors: []string{"has no services", "no running / pending tasks", "Fargate", "no auto scaling group"},
		FilterSamples: map[string]string{
			"instance-name": "web", "auto-scaling-group": "web-prod-asg", "private-ip": "10.0.", "instance-type": "t3",
			"public-ip": "203.0.113", "availability-zone": "us-east-1a", "state": "running", "instance-id": "@col:ID",
			"vpc-name": "@col:Name", "vpc:vpc-id": "@col:ID", "vpc-cidr": "@col:IPv4 CIDR", "vpc-default": "true", "vpc-state": "available",
			"subnet-name": "@col:Name", "subnets:subnet-id": "@col:ID",
			// ec2:subnet-id and subnets:vpc-id are proven by the subnet and VPC drill-downs
		},
	})
}

func TestOpenReturnsTheDemoWhenAsked(t *testing.T) {
	p, err := Open(context.Background(), provider.Options{Demo: true, PageSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := p.(provider.Demoer); !ok || !d.Demo() {
		t.Fatal("Open(Demo) must return a Demoer")
	}
	a := p.(provider.Authenticator)
	if a.AuthError() != nil || a.Profile() != "acme-prod" || a.Access() != "SSO (acme-prod)" {
		t.Fatalf("starts as the first demo account: %v %q %q", a.AuthError(), a.Profile(), a.Access())
	}
	// an unknown demo profile shows the friendly signed-out state, not a crash
	p, _ = Open(context.Background(), provider.Options{Demo: true, Profile: "nope", PageSize: 8})
	a = p.(provider.Authenticator)
	if ae := a.AuthError(); ae == nil || !strings.Contains(ae.Error(), `no demo profile called "nope"`) || !strings.Contains(ae.Error(), "acme-prod") {
		t.Fatalf("AuthError: %v", ae)
	}
	if len(a.Profiles()) != 3 {
		t.Fatalf("the picker needs the demo profiles: %+v", a.Profiles())
	}
}

func TestAWSRegistersItselfWithTheCloudRegistry(t *testing.T) {
	found := false
	for _, n := range cloud.Names() {
		found = found || n == "aws"
	}
	if !found {
		t.Fatalf("aws must self-register; registered: %v", cloud.Names())
	}
}

func TestSwitchingDemoProfilesChangesTheData(t *testing.T) {
	p := NewDemo(8)
	ctx := context.Background()
	asgCount := func() int {
		pg, err := (&asgs{p: p}).List(ctx, provider.Query{})
		if err != nil {
			t.Fatal(err)
		}
		return len(pg.Rows)
	}
	if asgCount() == 0 {
		t.Fatal("the prod demo account has auto scaling groups")
	}
	if err := p.UseProfile(ctx, "sandbox"); err != nil {
		t.Fatal(err)
	}
	if asgCount() != 0 || p.Region() != "us-west-2" || p.Access() != "access keys (sandbox)" {
		t.Fatalf("sandbox should be tiny and elsewhere: asgs=%d region=%s access=%q", asgCount(), p.Region(), p.Access())
	}
}

// The fake must refuse what the real API refuses, or the demo would hide bugs.
func TestDemoFakeEnforcesRealAPILimits(t *testing.T) {
	a := buildDemoAccounts(timeNow())[0]
	ctx := context.Background()
	if _, err := (&demoEC2{a}).DescribeInstances(ctx, &ec2.DescribeInstancesInput{MaxResults: awssdk.Int32(3)}); err == nil {
		t.Error("EC2 MaxResults below 5 must be rejected")
	}
	if _, err := (&demoEC2{a}).DescribeInstances(ctx, &ec2.DescribeInstancesInput{Filters: []ec2types.Filter{{Name: awssdk.String("made-up"), Values: []string{"x"}}}}); err == nil {
		t.Error("an unimplemented filter name must fail loudly, not return nothing")
	}
	if _, err := (&demoEC2{a}).DescribeInstances(ctx, &ec2.DescribeInstancesInput{NextToken: awssdk.String("garbage")}); err == nil {
		t.Error("a bad page token must be rejected")
	}
	arns := make([]string, 11)
	for i := range arns {
		arns[i] = "arn"
	}
	if _, err := (&demoECS{a}).DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: awssdk.String("prod-apps"), Services: arns}); err == nil {
		t.Error("ECS DescribeServices takes at most 10 services")
	}
	if _, err := (&demoECS{a}).DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: awssdk.String("prod-apps")}); err == nil {
		t.Error("ECS DescribeTasks with no tasks is an error")
	}
}

func TestDemoPagingVisitsEveryInstanceOnce(t *testing.T) {
	a := buildDemoAccounts(timeNow())[0]
	seen := map[string]bool{}
	var token *string
	pages := 0
	for {
		out, err := (&demoEC2{a}).DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{MaxResults: awssdk.Int32(5), NextToken: token})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range out.Reservations {
			for _, i := range r.Instances {
				if seen[*i.InstanceId] {
					t.Fatalf("instance %s returned twice", *i.InstanceId)
				}
				seen[*i.InstanceId] = true
			}
		}
		pages++
		if token = out.NextToken; token == nil {
			break
		}
	}
	if len(seen) != len(a.instances) || pages < 2 {
		t.Fatalf("saw %d of %d instances in %d pages", len(seen), len(a.instances), pages)
	}
}

// Cross-service consistency: the demo must read like one coherent account.
func TestDemoDataIsConsistentAndShapedLikeAWS(t *testing.T) {
	idRe := map[string]*regexp.Regexp{
		"i": regexp.MustCompile(`^i-0[0-9a-f]{16}$`), "vpc": regexp.MustCompile(`^vpc-0[0-9a-f]{16}$`),
		"subnet": regexp.MustCompile(`^subnet-0[0-9a-f]{16}$`), "ami": regexp.MustCompile(`^ami-0[0-9a-f]{16}$`),
	}
	testNet := netip.MustParsePrefix("203.0.113.0/24") // RFC 5737 documentation range
	for _, a := range buildDemoAccounts(timeNow()) {
		name := a.profile.Name
		if !regexp.MustCompile(`^\d{12}$`).MatchString(a.accounts) {
			t.Errorf("%s: account id %q", name, a.accounts)
		}
		subnets, vpcs, ids := map[string]ec2types.Subnet{}, map[string]bool{}, map[string]bool{}
		check := func(kind, id string) {
			if !idRe[kind].MatchString(id) {
				t.Errorf("%s: %s id %q is not shaped like AWS", name, kind, id)
			}
			if ids[id] {
				t.Errorf("%s: duplicate id %s", name, id)
			}
			ids[id] = true
		}
		for _, v := range a.vpcs {
			check("vpc", *v.VpcId)
			vpcs[*v.VpcId] = true
		}
		for _, s := range a.subnets {
			check("subnet", *s.SubnetId)
			subnets[*s.SubnetId] = s
			if !vpcs[*s.VpcId] {
				t.Errorf("%s: subnet %s is in an unknown vpc", name, *s.SubnetId)
			}
			if *s.AvailableIpAddressCount < 0 {
				t.Errorf("%s: subnet %s over-allocated", name, *s.SubnetId)
			}
		}
		byID := map[string]ec2types.Instance{}
		for _, i := range a.instances {
			check("i", *i.InstanceId)
			byID[*i.InstanceId] = i
			if _, ok := a.images[*i.ImageId]; !ok {
				t.Errorf("%s: instance %s uses an unknown image", name, *i.InstanceId)
			}
			sn, ok := subnets[*i.SubnetId]
			if !ok || *sn.VpcId != *i.VpcId || *sn.AvailabilityZone != *i.Placement.AvailabilityZone {
				t.Errorf("%s: instance %s disagrees with its subnet", name, *i.InstanceId)
			}
			if i.PrivateIpAddress != nil && !netip.MustParsePrefix(*sn.CidrBlock).Contains(netip.MustParseAddr(*i.PrivateIpAddress)) {
				t.Errorf("%s: %s private ip is outside its subnet", name, *i.InstanceId)
			}
			if i.PublicIpAddress != nil && !testNet.Contains(netip.MustParseAddr(*i.PublicIpAddress)) {
				t.Errorf("%s: public ip %s must come from the documentation range", name, *i.PublicIpAddress)
			}
		}
		for id := range a.images {
			check("ami", id)
		}
		for _, g := range a.asgs { // an ASG's instances are exactly the instances tagged with it
			var tagged int
			for _, i := range a.instances {
				if hasTag(i.Tags, "aws:autoscaling:groupName", *g.AutoScalingGroupName) {
					tagged++
				}
			}
			if tagged != len(g.Instances) {
				t.Errorf("%s: asg %s lists %d instances but %d are tagged", name, *g.AutoScalingGroupName, len(g.Instances), tagged)
			}
		}
		for _, c := range a.clusters { // ECS counters are derived from the services and tasks
			cn := *c.ClusterName
			if int(c.ActiveServicesCount) != len(a.services[cn]) || int(c.RegisteredContainerInstancesCount) != len(a.containers[cn]) {
				t.Errorf("%s: cluster %s counters disagree with its data", name, cn)
			}
			for _, ci := range a.containers[cn] {
				if _, ok := byID[*ci.Ec2InstanceId]; !ok {
					t.Errorf("%s: container instance points at unknown instance %s", name, *ci.Ec2InstanceId)
				}
			}
		}
		asgNames := map[string]bool{}
		for _, g := range a.asgs {
			asgNames[*g.AutoScalingGroupName] = true
		}
		for cluster, ngs := range a.nodegroups {
			for _, ng := range ngs {
				if g := *ng.Resources.AutoScalingGroups[0].Name; !asgNames[g] {
					t.Errorf("%s: node group %s/%s points at unknown asg %s", name, cluster, *ng.NodegroupName, g)
				}
			}
		}
	}
}

// Names must look like a real company's, so the demo can be recorded as the
// project's GIF. Placeholder words give the game away.
func TestDemoNamesLookRealistic(t *testing.T) {
	banned := regexp.MustCompile(`(?i)\b(foo|bar|baz|test|testing|example|dummy|fake|sample|lorem|asdf|xxx|todo)\b`)
	nameRe := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	for _, a := range buildDemoAccounts(timeNow()) {
		var names []string
		names = append(names, a.profile.Name)
		for _, i := range a.instances {
			names = append(names, tagValues(i.Tags, "Name")...)
		}
		for _, g := range a.asgs {
			names = append(names, *g.AutoScalingGroupName)
		}
		for _, c := range a.clusters {
			names = append(names, *c.ClusterName)
		}
		for _, ss := range a.services {
			for _, s := range ss {
				names = append(names, *s.ServiceName)
			}
		}
		for _, c := range a.eksClusters {
			names = append(names, *c.Name)
		}
		for _, n := range names {
			if !nameRe.MatchString(n) || banned.MatchString(n) {
				t.Errorf("%s: %q does not look like a real resource name", a.profile.Name, n)
			}
		}
	}
}

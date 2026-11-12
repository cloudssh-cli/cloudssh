package aws

import (
	"fmt"
	"net/netip"
	"sort"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// Demo data (cloudssh --demo). Everything here is made of the AWS SDK's own
// response types, so the compiler enforces the schema, and the production
// code maps them exactly as it maps real responses. Rules that keep it honest
// (see .claude/skills/demo-data):
//   - IDs and ARNs have the real AWS shapes; account ids are made up and
//     addresses come from the RFC 5737 documentation ranges, so nothing here
//     can be mistaken for, or collide with, a real account.
//   - Data is consistent across services (an ASG's instances are the instances
//     tagged with it; ECS cluster counts are derived from services and tasks).
//   - Generation is deterministic (no randomness) so tests can pin values.

// demoAccount is everything one demo profile can see.
type demoAccount struct {
	profile  provider.ProfileInfo
	access   string
	region   string
	accounts string // 12-digit account id

	vpcs      []ec2types.Vpc
	subnets   []ec2types.Subnet
	instances []ec2types.Instance
	images    map[string]ec2types.Image
	asgs      []asgtypes.AutoScalingGroup

	clusters   []ecstypes.Cluster
	services   map[string][]ecstypes.Service // by cluster name
	tasks      map[string][]ecstypes.Task    // by cluster/service
	containers map[string][]ecstypes.ContainerInstance

	eksClusters []ekstypes.Cluster
	nodegroups  map[string][]ekstypes.Nodegroup
}

type demoBuilder struct {
	acc    *demoAccount
	seq    uint64
	seed   uint64
	now    time.Time
	azs    []string
	subnet map[string]*ec2types.Subnet // by short key, e.g. "private-a"
	hosts  map[string]int              // addresses handed out per subnet id
	public int                         // next public address (TEST-NET-3)
	ecsArn map[string]string           // cluster name -> arn
}

func mix(x uint64) uint64 { // splitmix64
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func (b *demoBuilder) hex(n int) string {
	s := ""
	for len(s) < n {
		b.seq++
		s += fmt.Sprintf("%016x", mix(b.seq*7919+b.seed))
	}
	return s[:n]
}

// id makes an EC2-style resource id: prefix-0 + 16 hex digits.
func (b *demoBuilder) id(prefix string) string { return prefix + "-0" + b.hex(16) }

func (b *demoBuilder) uuid() string {
	h := b.hex(32)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func tag(k, v string) ec2types.Tag {
	return ec2types.Tag{Key: awssdk.String(k), Value: awssdk.String(v)}
}

func newBuilder(seed uint64, now time.Time, region string, p provider.ProfileInfo, access, accountID string) *demoBuilder {
	return &demoBuilder{
		acc: &demoAccount{
			profile: p, access: access, region: region, accounts: accountID,
			images: map[string]ec2types.Image{}, services: map[string][]ecstypes.Service{}, tasks: map[string][]ecstypes.Task{},
			containers: map[string][]ecstypes.ContainerInstance{}, nodegroups: map[string][]ekstypes.Nodegroup{},
		},
		seed: seed, now: now, azs: []string{region + "a", region + "b", region + "c"},
		subnet: map[string]*ec2types.Subnet{}, hosts: map[string]int{}, public: 10, ecsArn: map[string]string{},
	}
}

func (b *demoBuilder) vpc(name, cidr string, isDefault bool) string {
	v := ec2types.Vpc{
		VpcId: awssdk.String(b.id("vpc")), CidrBlock: awssdk.String(cidr), IsDefault: awssdk.Bool(isDefault),
		State: ec2types.VpcStateAvailable, OwnerId: awssdk.String(b.acc.accounts), InstanceTenancy: ec2types.TenancyDefault,
	}
	if name != "" {
		v.Tags = []ec2types.Tag{tag("Name", name)}
	}
	b.acc.vpcs = append(b.acc.vpcs, v)
	return *v.VpcId
}

// sub registers a subnet under a short key ("private-a") used by instance().
func (b *demoBuilder) sub(key, name, vpcID, cidr string, azIdx int, public bool) {
	s := ec2types.Subnet{
		SubnetId: awssdk.String(b.id("subnet")), VpcId: awssdk.String(vpcID), CidrBlock: awssdk.String(cidr),
		AvailabilityZone: awssdk.String(b.azs[azIdx]), AvailabilityZoneId: awssdk.String(fmt.Sprintf("%s-az%d", b.acc.region[:2]+"1", azIdx+1)),
		MapPublicIpOnLaunch: awssdk.Bool(public), State: ec2types.SubnetStateAvailable, OwnerId: awssdk.String(b.acc.accounts),
	}
	if name != "" {
		s.Tags = []ec2types.Tag{tag("Name", name)}
	}
	b.acc.subnets = append(b.acc.subnets, s)
	b.subnet[key] = &b.acc.subnets[len(b.acc.subnets)-1]
}

func (b *demoBuilder) image(_, description string) string {
	id := b.id("ami")
	b.acc.images[id] = ec2types.Image{ImageId: awssdk.String(id), Description: awssdk.String(description), State: ec2types.ImageStateAvailable}
	return id
}

type inst struct {
	name, typ, subnet, image, key, asg string
	state                              ec2types.InstanceStateName
	public, spot, arm                  bool
	age                                time.Duration
	extra                              map[string]string
}

var stateCodes = map[ec2types.InstanceStateName]int32{
	ec2types.InstanceStateNamePending: 0, ec2types.InstanceStateNameRunning: 16, ec2types.InstanceStateNameShuttingDown: 32,
	ec2types.InstanceStateNameTerminated: 48, ec2types.InstanceStateNameStopping: 64, ec2types.InstanceStateNameStopped: 80,
}

func (b *demoBuilder) instance(o inst) string {
	if o.state == "" {
		o.state = ec2types.InstanceStateNameRunning
	}
	if o.age == 0 {
		o.age = 72 * time.Hour
	}
	sn := b.subnet[o.subnet]
	i := ec2types.Instance{
		InstanceId: awssdk.String(b.id("i")), InstanceType: ec2types.InstanceType(o.typ), ImageId: awssdk.String(o.image),
		KeyName: awssdk.String(o.key), LaunchTime: awssdk.Time(b.now.Add(-o.age)), PlatformDetails: awssdk.String("Linux/UNIX"),
		Placement: &ec2types.Placement{AvailabilityZone: sn.AvailabilityZone},
		State:     &ec2types.InstanceState{Name: o.state, Code: awssdk.Int32(stateCodes[o.state])},
		SubnetId:  sn.SubnetId, VpcId: sn.VpcId, Architecture: ec2types.ArchitectureValuesX8664,
		Tags: []ec2types.Tag{tag("Name", o.name)},
	}
	if o.arm {
		i.Architecture = ec2types.ArchitectureValuesArm64
	}
	if o.spot {
		i.InstanceLifecycle = ec2types.InstanceLifecycleTypeSpot
	}
	if o.asg != "" {
		i.Tags = append(i.Tags, tag("aws:autoscaling:groupName", o.asg))
	}
	keys := make([]string, 0, len(o.extra))
	for k := range o.extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		i.Tags = append(i.Tags, tag(k, o.extra[k]))
	}
	if o.state != ec2types.InstanceStateNameTerminated { // terminated instances lose their addresses
		prefix := netip.MustParsePrefix(*sn.CidrBlock)
		b.hosts[*sn.SubnetId]++
		addr := prefix.Masked().Addr()
		for n := 0; n < b.hosts[*sn.SubnetId]+3; n++ { // .0-.3 are reserved by AWS
			addr = addr.Next()
		}
		i.PrivateIpAddress = awssdk.String(addr.String())
		i.PrivateDnsName = awssdk.String("ip-" + dashed(addr.String()) + "." + b.acc.region + ".compute.internal")
		if o.public && o.state == ec2types.InstanceStateNameRunning {
			b.public++
			pub := fmt.Sprintf("203.0.113.%d", b.public) // RFC 5737 TEST-NET-3
			i.PublicIpAddress = awssdk.String(pub)
			i.PublicDnsName = awssdk.String("ec2-" + dashed(pub) + "." + b.acc.region + ".compute.amazonaws.com")
		}
	}
	b.acc.instances = append(b.acc.instances, i)
	return *i.InstanceId
}

func dashed(ip string) string {
	out := []byte(ip)
	for k := range out {
		if out[k] == '.' {
			out[k] = '-'
		}
	}
	return string(out)
}

// asg builds an Auto Scaling group from the instances already tagged with it.
func (b *demoBuilder) asg(name string, min, max, desired int32) {
	g := asgtypes.AutoScalingGroup{
		AutoScalingGroupName: awssdk.String(name),
		AutoScalingGroupARN:  awssdk.String(fmt.Sprintf("arn:aws:autoscaling:%s:%s:autoScalingGroup:%s:autoScalingGroupName/%s", b.acc.region, b.acc.accounts, b.uuid(), name)),
		MinSize:              awssdk.Int32(min), MaxSize: awssdk.Int32(max), DesiredCapacity: awssdk.Int32(desired),
		HealthCheckType: awssdk.String("EC2"), CreatedTime: awssdk.Time(b.now.Add(-30 * 24 * time.Hour)),
		LaunchTemplate: &asgtypes.LaunchTemplateSpecification{
			LaunchTemplateId: awssdk.String(b.id("lt")), LaunchTemplateName: awssdk.String(name + "-lt"), Version: awssdk.String("$Latest"),
		},
	}
	for _, i := range b.acc.instances {
		if hasTag(i.Tags, "aws:autoscaling:groupName", name) {
			g.Instances = append(g.Instances, asgtypes.Instance{
				InstanceId: i.InstanceId, InstanceType: awssdk.String(string(i.InstanceType)),
				AvailabilityZone: i.Placement.AvailabilityZone, HealthStatus: awssdk.String("Healthy"),
				LifecycleState: asgtypes.LifecycleStateInService, LaunchTemplate: g.LaunchTemplate,
			})
		}
	}
	b.acc.asgs = append(b.acc.asgs, g)
}

func hasTag(tags []ec2types.Tag, k, v string) bool {
	for _, t := range tags {
		if awssdk.ToString(t.Key) == k && awssdk.ToString(t.Value) == v {
			return true
		}
	}
	return false
}

// ---- ECS ----

func (b *demoBuilder) ecsCluster(name string) {
	b.ecsArn[name] = fmt.Sprintf("arn:aws:ecs:%s:%s:cluster/%s", b.acc.region, b.acc.accounts, name)
	b.acc.clusters = append(b.acc.clusters, ecstypes.Cluster{
		ClusterArn: awssdk.String(b.ecsArn[name]), ClusterName: awssdk.String(name), Status: awssdk.String("ACTIVE"),
	})
}

func (b *demoBuilder) containerInstance(cluster, ec2ID string) {
	arn := fmt.Sprintf("arn:aws:ecs:%s:%s:container-instance/%s/%s", b.acc.region, b.acc.accounts, cluster, b.hex(32))
	b.acc.containers[cluster] = append(b.acc.containers[cluster], ecstypes.ContainerInstance{
		ContainerInstanceArn: awssdk.String(arn), Ec2InstanceId: awssdk.String(ec2ID), Status: awssdk.String("ACTIVE"),
		AgentConnected: true, RegisteredAt: awssdk.Time(b.now.Add(-48 * time.Hour)),
		VersionInfo: &ecstypes.VersionInfo{AgentVersion: awssdk.String("1.79.2"), DockerVersion: awssdk.String("DockerVersion: 25.0.5")},
	})
}

type svc struct {
	name, taskDef, cpu, mem string
	launch                  ecstypes.LaunchType
	desired, running        int32
	pending                 int32
	daemon                  bool
}

func (b *demoBuilder) ecsService(cluster string, o svc) {
	strategy := ecstypes.SchedulingStrategyReplica
	if o.daemon {
		strategy = ecstypes.SchedulingStrategyDaemon
	}
	taskDef := fmt.Sprintf("arn:aws:ecs:%s:%s:task-definition/%s", b.acc.region, b.acc.accounts, o.taskDef)
	s := ecstypes.Service{
		ServiceArn:  awssdk.String(fmt.Sprintf("arn:aws:ecs:%s:%s:service/%s/%s", b.acc.region, b.acc.accounts, cluster, o.name)),
		ServiceName: awssdk.String(o.name), ClusterArn: awssdk.String(b.ecsArn[cluster]), Status: awssdk.String("ACTIVE"),
		DesiredCount: o.desired, RunningCount: o.running, PendingCount: o.pending, LaunchType: o.launch,
		TaskDefinition: awssdk.String(taskDef), SchedulingStrategy: strategy,
		Deployments: []ecstypes.Deployment{{
			Id: awssdk.String("ecs-svc/" + b.hex(19)), Status: awssdk.String("PRIMARY"), TaskDefinition: awssdk.String(taskDef),
			DesiredCount: o.desired, RunningCount: o.running, PendingCount: o.pending,
			RolloutState: ecstypes.DeploymentRolloutStateCompleted, CreatedAt: awssdk.Time(b.now.Add(-6 * time.Hour)),
		}},
	}
	b.acc.services[cluster] = append(b.acc.services[cluster], s)

	for n := int32(0); n < o.running+o.pending; n++ {
		t := ecstypes.Task{
			TaskArn:           awssdk.String(fmt.Sprintf("arn:aws:ecs:%s:%s:task/%s/%s", b.acc.region, b.acc.accounts, cluster, b.hex(32))),
			ClusterArn:        awssdk.String(b.ecsArn[cluster]),
			Group:             awssdk.String("service:" + o.name),
			TaskDefinitionArn: awssdk.String(taskDef), LaunchType: o.launch, Cpu: awssdk.String(o.cpu), Memory: awssdk.String(o.mem),
			DesiredStatus: awssdk.String("RUNNING"), CreatedAt: awssdk.Time(b.now.Add(-time.Duration(30+n*7) * time.Minute)),
		}
		if n < o.running {
			t.LastStatus = awssdk.String("RUNNING")
			t.StartedAt = awssdk.Time(b.now.Add(-time.Duration(20+n*7) * time.Minute))
		} else {
			t.LastStatus = awssdk.String("PROVISIONING") // no StartedAt yet, like the real API
		}
		if o.launch == ecstypes.LaunchTypeEc2 {
			if cis := b.acc.containers[cluster]; len(cis) > 0 {
				t.ContainerInstanceArn = cis[int(n)%len(cis)].ContainerInstanceArn
			}
		}
		key := cluster + "/" + o.name
		b.acc.tasks[key] = append(b.acc.tasks[key], t)
	}
}

// finishSubnets computes each subnet's free address count the way AWS reports
// it: all addresses minus the 5 AWS reserves minus what instances use.
func (b *demoBuilder) finishSubnets() {
	for i, s := range b.acc.subnets {
		bits := netip.MustParsePrefix(*s.CidrBlock).Bits()
		b.acc.subnets[i].AvailableIpAddressCount = awssdk.Int32(int32(1<<(32-bits)) - 5 - int32(b.hosts[*s.SubnetId])) //nolint:gosec // subnet sizes are small
	}
}

// finishECS derives each cluster's counters from its services, tasks and instances.
func (b *demoBuilder) finishECS() {
	for i, c := range b.acc.clusters {
		name := awssdk.ToString(c.ClusterName)
		var running, pending int32
		for _, s := range b.acc.services[name] {
			for _, t := range b.acc.tasks[name+"/"+awssdk.ToString(s.ServiceName)] {
				switch awssdk.ToString(t.LastStatus) {
				case "RUNNING":
					running++
				default:
					pending++
				}
			}
		}
		b.acc.clusters[i].ActiveServicesCount = int32(len(b.acc.services[name]))                 //nolint:gosec // demo-sized
		b.acc.clusters[i].RegisteredContainerInstancesCount = int32(len(b.acc.containers[name])) //nolint:gosec // demo-sized
		b.acc.clusters[i].RunningTasksCount, b.acc.clusters[i].PendingTasksCount = running, pending
	}
}

// ---- EKS ----

func (b *demoBuilder) eksCluster(name, version, platform string, status ekstypes.ClusterStatus) {
	b.acc.eksClusters = append(b.acc.eksClusters, ekstypes.Cluster{
		Name: awssdk.String(name), Arn: awssdk.String(fmt.Sprintf("arn:aws:eks:%s:%s:cluster/%s", b.acc.region, b.acc.accounts, name)),
		Version: awssdk.String(version), PlatformVersion: awssdk.String(platform), Status: status,
		Endpoint:  awssdk.String("https://" + b.hex(32) + ".gr7." + b.acc.region + ".eks.amazonaws.com"),
		CreatedAt: awssdk.Time(b.now.Add(-90 * 24 * time.Hour)),
	})
}

func (b *demoBuilder) nodegroup(cluster, name, asgName, release string, desired, min, max int32, types ...string) {
	b.acc.nodegroups[cluster] = append(b.acc.nodegroups[cluster], ekstypes.Nodegroup{
		NodegroupName: awssdk.String(name), ClusterName: awssdk.String(cluster),
		NodegroupArn: awssdk.String(fmt.Sprintf("arn:aws:eks:%s:%s:nodegroup/%s/%s/%s", b.acc.region, b.acc.accounts, cluster, name, b.uuid())),
		Status:       ekstypes.NodegroupStatusActive, InstanceTypes: types, ReleaseVersion: awssdk.String(release),
		ScalingConfig: &ekstypes.NodegroupScalingConfig{DesiredSize: awssdk.Int32(desired), MinSize: awssdk.Int32(min), MaxSize: awssdk.Int32(max)},
		Resources:     &ekstypes.NodegroupResources{AutoScalingGroups: []ekstypes.AutoScalingGroup{{Name: awssdk.String(asgName)}}},
	})
}

// buildDemoAccounts creates the demo profiles. Three accounts in three
// regions, from large to nearly empty, so every screen has something to show
// and every empty state can be seen too.
func buildDemoAccounts(now time.Time) []*demoAccount {
	return []*demoAccount{demoProd(now), demoStaging(now), demoSandbox(now)}
}

func demoProd(now time.Time) *demoAccount {
	b := newBuilder(1, now, "us-east-1",
		provider.ProfileInfo{Name: "acme-prod", Kind: "SSO", Region: "us-east-1", Detail: "123456789012 AdministratorAccess"},
		"SSO (acme-prod)", "123456789012")
	al2023 := b.image("al2023", "Amazon Linux 2023 AMI 2023.5.20240805.0 x86_64 HVM kernel-6.1")
	ubuntu := b.image("ubuntu", "Canonical, Ubuntu, 22.04 LTS, amd64 jammy image build on 2024-07-23")
	debianARM := b.image("debian", "Debian 12 (20240717-1811) arm64")
	ecsAMI := b.image("ecs", "Amazon Linux 2023 ECS-Optimized AMI 2023.0.20240802 x86_64")
	eksAMI := b.image("eks", "Amazon EKS Kubernetes 1.30 AMI 1.30.2-20240729 x86_64 (Amazon Linux 2023)")

	main := b.vpc("prod-main", "10.0.0.0/16", false)
	def := b.vpc("", "172.31.0.0/16", true)
	dr := b.vpc("prod-dr", "10.1.0.0/16", false)
	for i, az := range []string{"a", "b", "c"} {
		b.sub("public-"+az, "prod-public-"+az, main, fmt.Sprintf("10.0.%d.0/24", i), i, true)
		b.sub("private-"+az, "prod-private-"+az, main, fmt.Sprintf("10.0.%d.0/24", 10+i), i, false)
		b.sub("default-"+az, "", def, fmt.Sprintf("172.31.%d.0/20", i*16), i, true)
	}
	b.sub("data-a", "prod-data-a", main, "10.0.20.0/24", 0, false)
	b.sub("data-b", "prod-data-b", main, "10.0.21.0/24", 1, false)
	b.sub("dr-a", "prod-dr-a", dr, "10.1.0.0/24", 0, false)
	b.sub("dr-b", "prod-dr-b", dr, "10.1.1.0/24", 1, false)

	b.instance(inst{name: "bastion-01", typ: "t3.micro", subnet: "public-a", image: al2023, key: "prod-key", public: true, age: 200 * time.Hour})
	for n, az := range []string{"a", "b", "c", "a"} {
		b.instance(inst{name: fmt.Sprintf("web-prod-%d", n+1), typ: "t3.medium", subnet: "private-" + az, image: al2023, key: "prod-key", asg: "web-prod-asg", age: time.Duration(20+n*9) * time.Hour})
	}
	for n, az := range []string{"a", "b", "c"} {
		b.instance(inst{name: fmt.Sprintf("api-prod-%d", n+1), typ: "m5.large", subnet: "private-" + az, image: ubuntu, key: "prod-key", asg: "api-prod-asg", age: time.Duration(40+n*5) * time.Hour})
	}
	b.instance(inst{name: "postgres-primary", typ: "r6g.xlarge", subnet: "data-a", image: debianARM, key: "db-key", arm: true, age: 900 * time.Hour})
	b.instance(inst{name: "postgres-replica", typ: "r6g.large", subnet: "data-b", image: debianARM, key: "db-key", arm: true, state: ec2types.InstanceStateNameStopped, age: 900 * time.Hour})
	var ecsHosts []string
	for n, az := range []string{"a", "b", "c"} {
		ecsHosts = append(ecsHosts, b.instance(inst{name: fmt.Sprintf("ecs-prod-apps-%d", n+1), typ: "c5.xlarge", subnet: "private-" + az, image: ecsAMI, key: "prod-key", asg: "ecs-prod-apps-asg", age: 120 * time.Hour}))
	}
	general, spot := "eks-general-"+b.uuid(), "eks-spot-"+b.uuid()
	for n, az := range []string{"a", "b", "c"} {
		b.instance(inst{name: "prod-eks-general-node", typ: "m5.xlarge", subnet: "private-" + az, image: eksAMI, key: "prod-key", asg: general, age: time.Duration(60+n) * time.Hour,
			extra: map[string]string{"eks:cluster-name": "prod-eks", "eks:nodegroup-name": "general"}})
	}
	for n := 0; n < 2; n++ {
		b.instance(inst{name: "prod-eks-spot-node", typ: "c5.2xlarge", subnet: []string{"private-a", "private-b"}[n], image: eksAMI, key: "prod-key", asg: spot, spot: true, age: 8 * time.Hour,
			extra: map[string]string{"eks:cluster-name": "prod-eks", "eks:nodegroup-name": "spot"}})
	}
	b.instance(inst{name: "jenkins-01", typ: "t3.large", subnet: "private-a", image: al2023, key: "prod-key", age: 500 * time.Hour})
	b.instance(inst{name: "batch-worker-1", typ: "c5.2xlarge", subnet: "private-b", image: al2023, key: "prod-key", state: ec2types.InstanceStateNamePending, age: time.Minute})
	b.instance(inst{name: "report-generator", typ: "t3.medium", subnet: "private-c", image: al2023, key: "prod-key", state: ec2types.InstanceStateNameStopping, age: 300 * time.Hour})
	b.instance(inst{name: "old-build-agent", typ: "t3.small", subnet: "private-c", image: al2023, key: "prod-key", state: ec2types.InstanceStateNameTerminated, age: 2000 * time.Hour})
	b.instance(inst{name: "dr-web-1", typ: "t3.medium", subnet: "dr-a", image: al2023, key: "prod-key", state: ec2types.InstanceStateNameStopped, age: 1000 * time.Hour})
	b.instance(inst{name: "dr-db-1", typ: "r6g.large", subnet: "dr-b", image: debianARM, key: "db-key", arm: true, state: ec2types.InstanceStateNameStopped, age: 1000 * time.Hour})
	b.instance(inst{name: "legacy-ftp", typ: "t2.micro", subnet: "default-a", image: al2023, key: "legacy-key", public: true, age: 4000 * time.Hour})

	b.asg("web-prod-asg", 2, 8, 4)
	b.asg("api-prod-asg", 3, 6, 3)
	b.asg("ecs-prod-apps-asg", 3, 6, 3)
	b.asg(general, 3, 6, 3)
	b.asg(spot, 0, 10, 2)

	b.ecsCluster("prod-apps")
	for _, id := range ecsHosts {
		b.containerInstance("prod-apps", id)
	}
	b.ecsService("prod-apps", svc{name: "api", taskDef: "api:42", cpu: "1024", mem: "2048", launch: ecstypes.LaunchTypeEc2, desired: 3, running: 3})
	b.ecsService("prod-apps", svc{name: "worker", taskDef: "worker:17", cpu: "512", mem: "1024", launch: ecstypes.LaunchTypeEc2, desired: 3, running: 2, pending: 1})
	b.ecsService("prod-apps", svc{name: "log-agent", taskDef: "log-agent:5", cpu: "128", mem: "256", launch: ecstypes.LaunchTypeEc2, desired: 3, running: 3, daemon: true})
	b.ecsService("prod-apps", svc{name: "scheduler", taskDef: "scheduler:9", cpu: "256", mem: "512", launch: ecstypes.LaunchTypeFargate, desired: 1, running: 1})
	b.ecsService("prod-apps", svc{name: "migrations", taskDef: "migrations:3", cpu: "256", mem: "512", launch: ecstypes.LaunchTypeFargate})
	b.ecsCluster("prod-fargate")
	b.ecsService("prod-fargate", svc{name: "web-frontend", taskDef: "web-frontend:128", cpu: "512", mem: "1024", launch: ecstypes.LaunchTypeFargate, desired: 3, running: 3})
	b.ecsService("prod-fargate", svc{name: "image-resizer", taskDef: "image-resizer:21", cpu: "1024", mem: "3072", launch: ecstypes.LaunchTypeFargate, desired: 2, running: 2})
	b.ecsCluster("batch") // exists, nothing running: the "no services" case
	b.finishECS()

	b.eksCluster("prod-eks", "1.30", "eks.5", ekstypes.ClusterStatusActive)
	b.nodegroup("prod-eks", "general", general, "1.30.2-20240729", 3, 3, 6, "m5.xlarge")
	b.nodegroup("prod-eks", "spot", spot, "1.30.2-20240729", 2, 0, 10, "c5.2xlarge", "c5a.2xlarge")
	b.eksCluster("data-platform", "1.29", "eks.12", ekstypes.ClusterStatusActive)
	b.eksCluster("legacy-eks", "1.27", "eks.9", ekstypes.ClusterStatusUpdating) // no node groups
	b.finishSubnets()
	return b.acc
}

func demoStaging(now time.Time) *demoAccount {
	b := newBuilder(2, now, "eu-west-1",
		provider.ProfileInfo{Name: "acme-staging", Kind: "SSO", Region: "eu-west-1", Detail: "210987654321 PowerUserAccess"},
		"SSO (acme-staging)", "210987654321")
	ubuntu := b.image("ubuntu", "Canonical, Ubuntu, 24.04 LTS, amd64 noble image build on 2024-07-01")
	al2023 := b.image("al2023", "Amazon Linux 2023 AMI 2023.5.20240805.0 x86_64 HVM kernel-6.1")

	main := b.vpc("staging-main", "10.20.0.0/16", false)
	def := b.vpc("", "172.31.0.0/16", true)
	b.sub("public-a", "stg-public-a", main, "10.20.0.0/24", 0, true)
	b.sub("public-b", "stg-public-b", main, "10.20.1.0/24", 1, true)
	b.sub("private-a", "stg-private-a", main, "10.20.10.0/24", 0, false)
	b.sub("private-b", "stg-private-b", main, "10.20.11.0/24", 1, false)
	b.sub("default-a", "", def, "172.31.0.0/20", 0, true)

	b.instance(inst{name: "bastion-stg", typ: "t3.micro", subnet: "public-a", image: al2023, key: "stg-key", public: true})
	b.instance(inst{name: "web-stg-1", typ: "t3.small", subnet: "private-a", image: ubuntu, key: "stg-key", asg: "web-stg-asg"})
	b.instance(inst{name: "web-stg-2", typ: "t3.small", subnet: "private-b", image: ubuntu, key: "stg-key", asg: "web-stg-asg"})
	b.instance(inst{name: "api-stg-1", typ: "t3.medium", subnet: "private-a", image: ubuntu, key: "stg-key", asg: "api-stg-asg"})
	b.instance(inst{name: "postgres-stg", typ: "t3.large", subnet: "private-b", image: ubuntu, key: "stg-key"})
	b.instance(inst{name: "perf-runner-1", typ: "c5.large", subnet: "private-a", image: al2023, key: "stg-key", state: ec2types.InstanceStateNameStopped})
	b.asg("web-stg-asg", 1, 4, 2)
	b.asg("api-stg-asg", 1, 2, 1)

	b.ecsCluster("staging-apps")
	b.ecsService("staging-apps", svc{name: "api", taskDef: "api:41", cpu: "512", mem: "1024", launch: ecstypes.LaunchTypeFargate, desired: 2, running: 2})
	b.ecsService("staging-apps", svc{name: "worker", taskDef: "worker:16", cpu: "256", mem: "512", launch: ecstypes.LaunchTypeFargate, desired: 1, running: 1})
	b.finishECS()
	b.finishSubnets()
	return b.acc
}

func demoSandbox(now time.Time) *demoAccount {
	b := newBuilder(3, now, "us-west-2",
		provider.ProfileInfo{Name: "sandbox", Kind: "access keys", Region: "us-west-2"},
		"access keys (sandbox)", "555566667777")
	al2023 := b.image("al2023", "Amazon Linux 2023 AMI 2023.5.20240805.0 x86_64 HVM kernel-6.1")
	def := b.vpc("", "172.31.0.0/16", true)
	b.sub("default-a", "", def, "172.31.0.0/20", 0, true)
	b.sub("default-b", "", def, "172.31.16.0/20", 1, true)
	b.instance(inst{name: "playground", typ: "t3.micro", subnet: "default-a", image: al2023, key: "sandbox-key", public: true})
	b.instance(inst{name: "scratch", typ: "t3.nano", subnet: "default-b", image: al2023, key: "sandbox-key", state: ec2types.InstanceStateNameStopped})
	b.finishSubnets()
	return b.acc // no ASGs, ECS or EKS: the empty states
}

package aws

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// AWS API limits: DescribeServices <= 10 ARNs, DescribeTasks/Clusters <= 100.
const (
	ecsDescribeServicesMax = 10
	ecsListMax             = 100
)

func chunk[T any](in []T, n int) [][]T {
	var out [][]T
	for len(in) > n {
		out, in = append(out, in[:n]), in[n:]
	}
	if len(in) > 0 {
		out = append(out, in)
	}
	return out
}

func lastSegment(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- clusters ----

type ecsClusters struct{ p *Provider }

type ecsClusterRef struct {
	Name     string
	Services int32
}

func (ecsClusters) Key() string                       { return "ecs" }
func (ecsClusters) Label() string                     { return "ECS Clusters" }
func (ecsClusters) Name() string                      { return "ECS Clusters" }
func (ecsClusters) Crumb() string                     { return "ecs_clusters" }
func (ecsClusters) Filters() []provider.FilterDef     { return nil }
func (ecsClusters) DefaultFilters() []provider.Filter { return nil }
func (ecsClusters) DrillHint() string                 { return "Services View" }

func (ecsClusters) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name"},
		{Title: "Services", Numeric: true},
		{Title: "Tasks", Numeric: true, DefaultSort: "desc"},
		{Title: "Container Instances", Numeric: true},
	}
}

func (r ecsClusters) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ecs.ListClustersInput{MaxResults: awssdk.Int32(r.p.size(ecsListMax))}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	list, err := cl.ecs.ListClusters(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	var rows []provider.Row
	if len(list.ClusterArns) > 0 {
		desc, err := cl.ecs.DescribeClusters(ctx, &ecs.DescribeClustersInput{Clusters: list.ClusterArns})
		if err != nil {
			return provider.Page{}, err
		}
		for _, c := range desc.Clusters {
			rows = append(rows, ecsClusterRow(c))
		}
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(list.NextToken)}, nil
}

func ecsClusterRow(c types.Cluster) provider.Row {
	name := awssdk.ToString(c.ClusterName)
	return provider.Row{
		Cells: []string{
			name, fmt.Sprint(c.ActiveServicesCount), fmt.Sprint(c.RunningTasksCount),
			fmt.Sprint(c.RegisteredContainerInstancesCount),
		},
		Ref: ecsClusterRef{Name: name, Services: c.ActiveServicesCount},
	}
}

func (r ecsClusters) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(ecsClusterRef)
	if !ok {
		return provider.Drill{}, errors.New("no cluster selected")
	}
	if ref.Services <= 0 {
		return provider.Drill{}, fmt.Errorf("cluster %s has no services", ref.Name)
	}
	return provider.Drill{Resource: &ecsServices{p: r.p, cluster: ref.Name}}, nil
}

// ---- services (drill-only) ----

type ecsServices struct {
	p       *Provider
	cluster string
}

type ecsServiceRef struct {
	Cluster, Name    string
	Running, Pending int32
}

func (ecsServices) Key() string                       { return "ecs-services" }
func (ecsServices) Label() string                     { return "ECS Services" }
func (r ecsServices) Name() string                    { return r.cluster }
func (ecsServices) Crumb() string                     { return "services" }
func (ecsServices) Filters() []provider.FilterDef     { return nil }
func (ecsServices) DefaultFilters() []provider.Filter { return nil }
func (ecsServices) DrillHint() string                 { return "Tasks View" }

func (ecsServices) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name", DefaultSort: "asc"}, {Title: "Status"}, {Title: "Type"}, {Title: "Tasks"},
		{Title: "Last Deployment"}, {Title: "Task Definition"}, {Title: "Launch Type"},
	}
}

func (r ecsServices) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ecs.ListServicesInput{Cluster: &r.cluster, MaxResults: awssdk.Int32(r.p.size(ecsListMax))}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	list, err := cl.ecs.ListServices(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	var rows []provider.Row
	for _, arns := range chunk(list.ServiceArns, ecsDescribeServicesMax) {
		desc, err := cl.ecs.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: &r.cluster, Services: arns})
		if err != nil {
			return provider.Page{}, err
		}
		for _, s := range desc.Services {
			rows = append(rows, ecsServiceRow(r.cluster, s))
		}
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(list.NextToken)}, nil
}

func ecsServiceRow(cluster string, s types.Service) provider.Row {
	deployment := "unknown"
	if len(s.Deployments) > 0 {
		deployment = awssdk.ToString(s.Deployments[0].Status)
	}
	name := awssdk.ToString(s.ServiceName)
	return provider.Row{
		Cells: []string{
			name, awssdk.ToString(s.Status), string(s.SchedulingStrategy),
			fmt.Sprintf("%d running / %d desired", s.RunningCount, s.DesiredCount),
			deployment, lastSegment(awssdk.ToString(s.TaskDefinition)), orDash(string(s.LaunchType)),
		},
		Ref: ecsServiceRef{Cluster: cluster, Name: name, Running: s.RunningCount, Pending: s.PendingCount},
	}
}

func (r ecsServices) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(ecsServiceRef)
	if !ok {
		return provider.Drill{}, errors.New("no service selected")
	}
	if ref.Running <= 0 && ref.Pending <= 0 {
		return provider.Drill{}, fmt.Errorf("service %s has no running / pending tasks", ref.Name)
	}
	return provider.Drill{Resource: &ecsTasks{p: r.p, cluster: ref.Cluster, service: ref.Name}}, nil
}

// ---- tasks (drill-only) ----

type ecsTasks struct {
	p                *Provider
	cluster, service string
}

type ecsTaskRef struct{ Cluster, ContainerInstanceARN string }

func (ecsTasks) Key() string                       { return "ecs-tasks" }
func (ecsTasks) Label() string                     { return "ECS Tasks" }
func (r ecsTasks) Name() string                    { return r.service + "(" + r.cluster + ")" }
func (ecsTasks) Crumb() string                     { return "tasks" }
func (ecsTasks) Filters() []provider.FilterDef     { return nil }
func (ecsTasks) DefaultFilters() []provider.Filter { return nil }
func (ecsTasks) DrillHint() string                 { return "Instance View" }

func (ecsTasks) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Task ID", DefaultSort: "asc"}, {Title: "Current/Desired Status"}, {Title: "Started At"},
		{Title: "Container Instance"}, {Title: "Launch Type"}, {Title: "CPU"}, {Title: "Memory"},
	}
}

func (r ecsTasks) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ecs.ListTasksInput{Cluster: &r.cluster, ServiceName: &r.service, MaxResults: awssdk.Int32(r.p.size(ecsListMax))}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	list, err := cl.ecs.ListTasks(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	var rows []provider.Row
	if len(list.TaskArns) > 0 {
		desc, err := cl.ecs.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: &r.cluster, Tasks: list.TaskArns})
		if err != nil {
			return provider.Page{}, err
		}
		for _, t := range desc.Tasks {
			rows = append(rows, ecsTaskRow(r.cluster, t))
		}
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(list.NextToken)}, nil
}

func ecsTaskRow(cluster string, t types.Task) provider.Row {
	taskID := strings.SplitN(lastSegment(awssdk.ToString(t.TaskArn)), "-", 2)[0]
	started := "-"
	if t.StartedAt != nil {
		started = t.StartedAt.Local().Format(time.DateTime)
	}
	ci := awssdk.ToString(t.ContainerInstanceArn)
	ciShort := "-"
	if ci != "" {
		ciShort = lastSegment(ci)[:min(8, len(lastSegment(ci)))]
	}
	return provider.Row{
		Cells: []string{
			taskID[:min(8, len(taskID))],
			awssdk.ToString(t.LastStatus) + "/" + awssdk.ToString(t.DesiredStatus),
			started, ciShort, orDash(string(t.LaunchType)),
			humanCPU(awssdk.ToString(t.Cpu)), humanMemory(awssdk.ToString(t.Memory)),
		},
		Ref: ecsTaskRef{Cluster: cluster, ContainerInstanceARN: ci},
	}
}

// humanCPU renders ECS CPU units as vCPUs ("1024" -> "1", "256" -> "0.25").
func humanCPU(units string) string {
	n, err := strconv.Atoi(units)
	if err != nil {
		return orDash(units)
	}
	if n%1024 == 0 {
		return strconv.Itoa(n / 1024)
	}
	return fmt.Sprintf("%.2f", float64(n)/1024)
}

// humanMemory renders MiB as "512 MB" / "2 GB" / "1.50 GB".
func humanMemory(mib string) string {
	n, err := strconv.Atoi(mib)
	if err != nil {
		return orDash(mib)
	}
	switch {
	case n <= 1024:
		return fmt.Sprintf("%d MB", n)
	case n%1024 == 0:
		return fmt.Sprintf("%d GB", n/1024)
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/1024)
	}
}

// Drill resolves the task's container instance to its EC2 instance.
func (r ecsTasks) Drill(ctx context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(ecsTaskRef)
	if !ok {
		return provider.Drill{}, errors.New("no task selected")
	}
	if ref.ContainerInstanceARN == "" {
		return provider.Drill{}, errors.New("task has no EC2 container instance (Fargate?)")
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Drill{}, err
	}
	out, err := cl.ecs.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
		Cluster: &ref.Cluster, ContainerInstances: []string{ref.ContainerInstanceARN},
	})
	if err != nil {
		return provider.Drill{}, err
	}
	if len(out.ContainerInstances) == 0 {
		return provider.Drill{}, errors.New("container instance not found")
	}
	id := awssdk.ToString(out.ContainerInstances[0].Ec2InstanceId)
	return provider.Drill{
		Resource: &instances{p: r.p},
		Filters:  []provider.Filter{{Key: "instance-id", Value: id}},
	}, nil
}

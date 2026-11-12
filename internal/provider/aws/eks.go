package aws

import (
	"context"
	"errors"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// eksPageMax is EKS's List limit. Pages can be large because List returns names
// only; details are fetched lazily for visible rows (see provider.Enricher).
const eksPageMax = 100

type eksClusters struct{ p *Provider }

type eksClusterRef struct{ Name string }

func (eksClusters) Key() string                       { return "eks" }
func (eksClusters) Label() string                     { return "EKS Clusters" }
func (eksClusters) Name() string                      { return "EKS · Clusters" }
func (eksClusters) Crumb() string                     { return "eks_clusters" }
func (eksClusters) Filters() []provider.FilterDef     { return nil }
func (eksClusters) DefaultFilters() []provider.Filter { return nil }
func (eksClusters) DrillHint() string                 { return "Node Groups View" }

func (eksClusters) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name", DefaultSort: "asc"}, {Title: "Kubernetes Version"}, {Title: "Platform Version"}, {Title: "Status"},
	}
}

func (r eksClusters) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &eks.ListClustersInput{MaxResults: awssdk.Int32(r.p.size(eksPageMax)), Include: []string{"all"}}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	list, err := cl.eks.ListClusters(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	// Names only: version, platform and status need one DescribeCluster call per
	// cluster, which the UI makes lazily for the rows on screen (see Enrich).
	rows := make([]provider.Row, 0, len(list.Clusters))
	for _, name := range list.Clusters {
		rows = append(rows, provider.Row{
			Cells: []string{name, "…", "…", "…"}, Ref: eksClusterRef{Name: name}, ID: name, Partial: true,
		})
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(list.NextToken)}, nil
}

func eksClusterRow(c types.Cluster) provider.Row {
	name := awssdk.ToString(c.Name)
	return provider.Row{
		Cells: []string{name, awssdk.ToString(c.Version), awssdk.ToString(c.PlatformVersion), string(c.Status)},
		Ref:   eksClusterRef{Name: name}, ID: name,
	}
}

var _ provider.Enricher = (*eksClusters)(nil)

// Enrich fetches one cluster's details (called for visible rows only).
func (r eksClusters) Enrich(ctx context.Context, row provider.Row) (provider.Row, error) {
	ref, ok := row.Ref.(eksClusterRef)
	if !ok {
		return row, errors.New("no cluster selected")
	}
	cl, err := r.p.clients()
	if err != nil {
		return row, err
	}
	d, err := cl.eks.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: awssdk.String(ref.Name)})
	if err != nil {
		return row, err
	}
	if d.Cluster == nil {
		return row, errors.New("cluster not found")
	}
	return eksClusterRow(*d.Cluster), nil
}

func (r eksClusters) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(eksClusterRef)
	if !ok {
		return provider.Drill{}, errors.New("no cluster selected")
	}
	return provider.Drill{Resource: &eksNodeGroups{p: r.p, cluster: ref.Name}}, nil
}

// ---- node groups (drill-only) ----

type eksNodeGroups struct {
	p       *Provider
	cluster string
}

// nodeGroupRef identifies a node group; ASG is known once Loaded.
type nodeGroupRef struct {
	Cluster, Name, ASG string
	Loaded             bool
}

func (eksNodeGroups) Key() string                       { return "eks-nodegroups" }
func (eksNodeGroups) Label() string                     { return "EKS Node Groups" }
func (r eksNodeGroups) Name() string                    { return "EKS - " + r.cluster }
func (eksNodeGroups) Crumb() string                     { return "node_groups" }
func (eksNodeGroups) Filters() []provider.FilterDef     { return nil }
func (eksNodeGroups) DefaultFilters() []provider.Filter { return nil }
func (eksNodeGroups) DrillHint() string                 { return "Instances View" }

func (eksNodeGroups) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Group Name"},
		{Title: "Desired Size", Numeric: true, DefaultSort: "desc"},
		{Title: "Min/Max Size"}, {Title: "ASG Name"}, {Title: "AMI version"}, {Title: "Status"},
	}
}

func (r eksNodeGroups) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &eks.ListNodegroupsInput{ClusterName: &r.cluster, MaxResults: awssdk.Int32(r.p.size(eksPageMax))}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	list, err := cl.eks.ListNodegroups(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	rows := make([]provider.Row, 0, len(list.Nodegroups))
	for _, name := range list.Nodegroups {
		rows = append(rows, provider.Row{
			Cells: []string{name, "…", "…", "…", "…", "…"}, Ref: nodeGroupRef{Cluster: r.cluster, Name: name}, ID: name, Partial: true,
		})
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(list.NextToken)}, nil
}

func nodeGroupRow(n types.Nodegroup) provider.Row {
	var desired, minSize, maxSize int32
	if n.ScalingConfig != nil {
		desired = awssdk.ToInt32(n.ScalingConfig.DesiredSize)
		minSize = awssdk.ToInt32(n.ScalingConfig.MinSize)
		maxSize = awssdk.ToInt32(n.ScalingConfig.MaxSize)
	}
	asg := ""
	if n.Resources != nil && len(n.Resources.AutoScalingGroups) > 0 {
		asg = awssdk.ToString(n.Resources.AutoScalingGroups[0].Name)
	}
	return provider.Row{
		Cells: []string{
			awssdk.ToString(n.NodegroupName), fmt.Sprint(desired), fmt.Sprintf("%d/%d", minSize, maxSize),
			orDash(asg), awssdk.ToString(n.ReleaseVersion), string(n.Status),
		},
		Ref: nodeGroupRef{Cluster: awssdk.ToString(n.ClusterName), Name: awssdk.ToString(n.NodegroupName), ASG: asg, Loaded: true}, ID: awssdk.ToString(n.NodegroupName),
	}
}

var _ provider.Enricher = (*eksNodeGroups)(nil)

// Enrich fetches one node group's details (called for visible rows only).
func (r eksNodeGroups) Enrich(ctx context.Context, row provider.Row) (provider.Row, error) {
	ref, ok := row.Ref.(nodeGroupRef)
	if !ok {
		return row, errors.New("no node group selected")
	}
	cl, err := r.p.clients()
	if err != nil {
		return row, err
	}
	d, err := cl.eks.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{ClusterName: awssdk.String(r.cluster), NodegroupName: awssdk.String(ref.Name)})
	if err != nil {
		return row, err
	}
	if d.Nodegroup == nil {
		return row, errors.New("node group not found")
	}
	return nodeGroupRow(*d.Nodegroup), nil
}

func (r eksNodeGroups) Drill(ctx context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(nodeGroupRef)
	if !ok {
		return provider.Drill{}, errors.New("no node group selected")
	}
	if !ref.Loaded { // details have not arrived yet: fetch them now
		full, err := r.Enrich(ctx, row)
		if err != nil {
			return provider.Drill{}, err
		}
		ref = full.Ref.(nodeGroupRef)
	}
	if ref.ASG == "" {
		return provider.Drill{}, errors.New("node group has no auto scaling group")
	}
	return provider.Drill{
		Resource: &instances{p: r.p, sub: ref.Name + " · " + ref.ASG},
		Filters:  []provider.Filter{{Key: "auto-scaling-group", Value: ref.ASG}},
	}, nil
}

package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type asgs struct{ p *Provider }

type asgRef struct{ Name string }

func (asgs) Key() string                       { return "asg" }
func (asgs) Label() string                     { return "Auto Scaling Groups" }
func (asgs) Name() string                      { return "Auto Scaling · Groups" }
func (asgs) Crumb() string                     { return "autoscaling_groups" }
func (asgs) Filters() []provider.FilterDef     { return nil }
func (asgs) DefaultFilters() []provider.Filter { return nil }
func (asgs) DrillHint() string                 { return "Instances View" }

func (asgs) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name"},
		{Title: "Instances", Numeric: true},
		{Title: "Desired Capacity", Numeric: true, DefaultSort: "desc"},
		{Title: "Min/Max Size"},
	}
}

func (r asgs) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &autoscaling.DescribeAutoScalingGroupsInput{MaxRecords: awssdk.Int32(r.p.size(100))}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	out, err := cl.asg.DescribeAutoScalingGroups(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	rows := make([]provider.Row, 0, len(out.AutoScalingGroups))
	for _, g := range out.AutoScalingGroups {
		rows = append(rows, asgRow(g))
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(out.NextToken)}, nil
}

func asgRow(g types.AutoScalingGroup) provider.Row {
	name := awssdk.ToString(g.AutoScalingGroupName)
	return provider.Row{
		Cells: []string{
			name,
			fmt.Sprint(len(g.Instances)),
			fmt.Sprint(awssdk.ToInt32(g.DesiredCapacity)),
			fmt.Sprintf("%d/%d", awssdk.ToInt32(g.MinSize), awssdk.ToInt32(g.MaxSize)),
		},
		Ref: asgRef{Name: name},
	}
}

// Drill opens the instances of the selected group.
func (r asgs) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(asgRef)
	if !ok {
		return provider.Drill{}, fmt.Errorf("no auto scaling group selected")
	}
	return provider.Drill{
		Resource: &instances{p: r.p, sub: "asg " + ref.Name},
		Filters:  []provider.Filter{{Key: "auto-scaling-group", Value: ref.Name}},
	}, nil
}

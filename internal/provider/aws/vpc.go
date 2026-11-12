package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func nameTag(tags []types.Tag) string {
	for _, t := range tags {
		if awssdk.ToString(t.Key) == "Name" {
			return awssdk.ToString(t.Value)
		}
	}
	return ""
}

// ---- VPCs ----

type vpcs struct{ p *Provider }

type vpcRef struct{ ID string }

func (vpcs) Key() string                       { return "vpc" }
func (vpcs) Label() string                     { return "Virtual Private Clouds" }
func (vpcs) Name() string                      { return "Virtual Private Clouds" }
func (vpcs) Crumb() string                     { return "vpcs" }
func (vpcs) Filters() []provider.FilterDef     { return defsOf(vpcSpecs) }
func (vpcs) DefaultFilters() []provider.Filter { return nil }
func (vpcs) DrillHint() string                 { return "Subnets View" }

func (vpcs) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name", DefaultSort: "asc"}, {Title: "ID"}, {Title: "IPv4 CIDR"}, {Title: "Default"},
	}
}

func (r vpcs) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ec2.DescribeVpcsInput{
		MaxResults: awssdk.Int32(max(r.p.size(1000), 5)),
		Filters:    buildFilters(vpcSpecs, q.Filters),
	}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	out, err := cl.ec2.DescribeVpcs(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	rows := make([]provider.Row, 0, len(out.Vpcs))
	for _, v := range dropMismatched(out.Vpcs, vpcSpecs, q.Filters, vpcField) {
		rows = append(rows, vpcRow(v))
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(out.NextToken)}, nil
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func vpcRow(v types.Vpc) provider.Row {
	id := awssdk.ToString(v.VpcId)
	return provider.Row{
		Cells: []string{nameTag(v.Tags), id, awssdk.ToString(v.CidrBlock), yesNo(awssdk.ToBool(v.IsDefault))},
		Ref:   vpcRef{ID: id},
	}
}

func (r vpcs) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(vpcRef)
	if !ok {
		return provider.Drill{}, fmt.Errorf("no vpc selected")
	}
	return provider.Drill{
		Resource: &subnets{p: r.p, vpc: ref.ID},
		Filters:  []provider.Filter{{Key: "vpc-id", Value: ref.ID}},
	}, nil
}

// ---- Subnets (drill-only) ----

type subnets struct {
	p   *Provider
	vpc string
}

type subnetRef struct{ ID string }

func (subnets) Key() string                       { return "subnets" }
func (subnets) Label() string                     { return "Subnets" }
func (subnets) Name() string                      { return "Subnets" }
func (subnets) Crumb() string                     { return "subnets" }
func (subnets) Filters() []provider.FilterDef     { return defsOf(subnetSpecs) }
func (subnets) DefaultFilters() []provider.Filter { return nil }
func (subnets) DrillHint() string                 { return "Instances View" }

func (subnets) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Name"}, {Title: "ID"}, {Title: "VPC"}, {Title: "AZ"}, {Title: "IPv4 CIDR"},
		{Title: "Available Ips", Numeric: true, DefaultSort: "asc"},
	}
}

func (r subnets) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ec2.DescribeSubnetsInput{
		MaxResults: awssdk.Int32(max(r.p.size(1000), 5)),
		Filters:    buildFilters(subnetSpecs, q.Filters),
	}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	out, err := cl.ec2.DescribeSubnets(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	rows := make([]provider.Row, 0, len(out.Subnets))
	for _, s := range dropMismatched(out.Subnets, subnetSpecs, q.Filters, subnetField) {
		rows = append(rows, subnetRow(s))
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(out.NextToken)}, nil
}

func subnetRow(s types.Subnet) provider.Row {
	id := awssdk.ToString(s.SubnetId)
	return provider.Row{
		Cells: []string{
			nameTag(s.Tags), id, awssdk.ToString(s.VpcId), awssdk.ToString(s.AvailabilityZone),
			awssdk.ToString(s.CidrBlock), fmt.Sprint(awssdk.ToInt32(s.AvailableIpAddressCount)),
		},
		Ref: subnetRef{ID: id},
	}
}

func (r subnets) Drill(_ context.Context, row provider.Row) (provider.Drill, error) {
	ref, ok := row.Ref.(subnetRef)
	if !ok {
		return provider.Drill{}, fmt.Errorf("no subnet selected")
	}
	return provider.Drill{
		Resource: &instances{p: r.p},
		Filters:  []provider.Filter{{Key: "subnet-id", Value: ref.ID}},
	}, nil
}

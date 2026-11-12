package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// instances lists EC2 instances. sub, when set, is shown in the title
// (e.g. the ASG the list was drilled from).
type instances struct {
	p   *Provider
	sub string
}

func (instances) Key() string   { return "ec2" }
func (instances) Label() string { return "EC2 Instances" }
func (i instances) Name() string {
	if i.sub != "" {
		return "Instances(" + i.sub + ")"
	}
	return "Instances"
}
func (instances) Crumb() string { return "ec2_instances" }

func (instances) Columns() []provider.Column {
	return []provider.Column{
		{Title: "ID"}, {Title: "Name", DefaultSort: "asc"}, {Title: "Type"}, {Title: "AZ"},
		{Title: "Private IP"}, {Title: "Public IP"}, {Title: "ASG Name"}, {Title: "State"},
	}
}

func (instances) Filters() []provider.FilterDef { return defsOf(ec2Specs) }

func (instances) DefaultFilters() []provider.Filter {
	return []provider.Filter{{Key: "state", Value: "running"}}
}

func (r instances) List(ctx context.Context, q provider.Query) (provider.Page, error) {
	in := &ec2.DescribeInstancesInput{
		MaxResults: awssdk.Int32(r.p.size(1000)),
		Filters:    buildFilters(ec2Specs, q.Filters),
	}
	if q.Token != "" {
		in.NextToken = awssdk.String(q.Token)
	}
	cl, err := r.p.clients()
	if err != nil {
		return provider.Page{}, err
	}
	out, err := cl.ec2.DescribeInstances(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	var all []types.Instance
	for _, res := range out.Reservations {
		all = append(all, res.Instances...)
	}
	var rows []provider.Row
	for _, i := range dropExcluded(all, ec2Specs, q.Filters, instanceField) {
		rows = append(rows, instanceRow(i))
	}
	return provider.Page{Rows: rows, NextToken: awssdk.ToString(out.NextToken)}, nil
}

func instanceRow(i types.Instance) provider.Row {
	var name, asg string
	for _, t := range i.Tags {
		switch awssdk.ToString(t.Key) {
		case "Name":
			name = awssdk.ToString(t.Value)
		case "aws:autoscaling:groupName":
			asg = awssdk.ToString(t.Value)
		}
	}
	var az, state string
	if i.Placement != nil {
		az = awssdk.ToString(i.Placement.AvailabilityZone)
	}
	if i.State != nil {
		state = string(i.State.Name)
	}
	id := awssdk.ToString(i.InstanceId)
	priv, pub := awssdk.ToString(i.PrivateIpAddress), awssdk.ToString(i.PublicIpAddress)
	return provider.Row{
		Cells: []string{id, name, string(i.InstanceType), az, priv, pub, asg, state},
		Ref: provider.Host{
			ID: id, Name: name, PrivateIP: priv, PublicIP: pub,
			KeyName: awssdk.ToString(i.KeyName), ImageID: awssdk.ToString(i.ImageId), AZ: az,
		},
	}
}

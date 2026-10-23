package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type instances struct{ p *Provider }

func (instances) Name() string { return "EC2" }

func (instances) Columns() []provider.Column {
	return []provider.Column{
		{Title: "ID"}, {Title: "Name"}, {Title: "Type"}, {Title: "AZ"},
		{Title: "Private IP"}, {Title: "Public IP"}, {Title: "ASG"}, {Title: "State"},
	}
}

func (r *instances) List(ctx context.Context, nextToken string) (provider.Page, error) {
	in := &ec2.DescribeInstancesInput{MaxResults: awssdk.Int32(r.p.pageSize)}
	if nextToken != "" {
		in.NextToken = awssdk.String(nextToken)
	}
	out, err := r.p.ec2.DescribeInstances(ctx, in)
	if err != nil {
		return provider.Page{}, err
	}
	var rows []provider.Row
	for _, res := range out.Reservations {
		for _, i := range res.Instances {
			rows = append(rows, instanceRow(i))
		}
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
			ID: id, PrivateIP: priv, PublicIP: pub,
			KeyName: awssdk.ToString(i.KeyName), ImageID: awssdk.ToString(i.ImageId),
		},
	}
}

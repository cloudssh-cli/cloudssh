package aws

import "github.com/cloudssh-cli/cloudssh/internal/cloud"

// The AWS backend registers itself; the CLI only has to import this package.
func init() {
	cloud.Register(cloud.Cloud{Name: "aws", Description: "Amazon Web Services (EC2, Auto Scaling, VPC, ECS, EKS)", Open: Open})
}

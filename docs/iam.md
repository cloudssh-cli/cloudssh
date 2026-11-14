# IAM permissions

cloudssh only reads. Attach this policy to the role or user you sign in with and you can use every screen. Drop the statements for the screens you do not use.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "Ec2AndVpc",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVpcs",
        "ec2:DescribeSubnets",
        "ec2:DescribeImages"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AutoScaling",
      "Effect": "Allow",
      "Action": ["autoscaling:DescribeAutoScalingGroups"],
      "Resource": "*"
    },
    {
      "Sid": "Ecs",
      "Effect": "Allow",
      "Action": [
        "ecs:ListClusters",
        "ecs:DescribeClusters",
        "ecs:ListServices",
        "ecs:DescribeServices",
        "ecs:ListTasks",
        "ecs:DescribeTasks",
        "ecs:DescribeContainerInstances"
      ],
      "Resource": "*"
    },
    {
      "Sid": "Eks",
      "Effect": "Allow",
      "Action": [
        "eks:ListClusters",
        "eks:DescribeCluster",
        "eks:ListNodegroups",
        "eks:DescribeNodegroup"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AccountAlias",
      "Effect": "Allow",
      "Action": ["iam:ListAccountAliases"],
      "Resource": "*"
    }
  ]
}
```

Notes:

- Identity (`sts:GetCallerIdentity`) needs no permission.
- `ec2:DescribeImages` is used to find the default SSH user of an AMI.
- `iam:ListAccountAliases` only adds the account alias to the header; without it cloudssh shows the account id.
- A missing permission shows up as an error on that screen; the other screens keep working.

## EC2 Instance Connect (optional)

Only needed if you choose the *EC2 Instance Connect* method in the SSH dialog. This is the one action that changes something: it pushes a short-lived public key (valid 60 seconds) to the instance.

```json
{
  "Effect": "Allow",
  "Action": "ec2-instance-connect:SendSSHPublicKey",
  "Resource": "arn:aws:ec2:*:*:instance/*"
}
```

Scope `Resource` (or add a tag condition) to the instances people may connect to.

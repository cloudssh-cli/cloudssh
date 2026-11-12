package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/query"
)

// filterSpec maps a UI filter to an EC2 API filter name.
type filterSpec struct {
	key, name string // provider.FilterDef
	api       string // EC2 filter name
	exact     bool   // exact match; otherwise the value is wrapped in *...*
}

// shortNames are the compact header labels ("name=web").
var shortNames = map[string]string{
	"instance-name": "name", "auto-scaling-group": "asg", "private-ip": "ip", "instance-type": "type", "public-ip": "public-ip",
	"availability-zone": "az", "state": "state", "instance-id": "id", "subnet-id": "subnet",
	"vpc-name": "name", "vpc-id": "vpc", "vpc-cidr": "cidr", "vpc-default": "default", "vpc-state": "state", "subnet-name": "name",
}

func defsOf(specs []filterSpec) []provider.FilterDef {
	out := make([]provider.FilterDef, len(specs))
	for i, s := range specs {
		out[i] = provider.FilterDef{Key: s.key, Name: s.name, Short: shortNames[s.key]}
	}
	return out
}

func (s filterSpec) wrap(v string) string {
	if s.exact {
		return v
	}
	return "*" + v + "*"
}

// buildFilters converts the positive part of UI filters to EC2 filters: one
// value per OR alternative (its first term), which is a superset the API can
// apply. Anything it cannot express (AND, NOT) is checked afterwards by
// dropMismatched. Empty values and unknown keys are skipped.
func buildFilters(specs []filterSpec, fs []provider.Filter) []types.Filter {
	var out []types.Filter
	for _, f := range fs {
		terms := query.Parse(f.Value).ServerTerms()
		if len(terms) == 0 {
			continue
		}
		for _, s := range specs {
			if s.key != f.Key {
				continue
			}
			vals := make([]string, len(terms))
			for i, v := range terms {
				vals[i] = s.wrap(v)
			}
			name := s.api
			out = append(out, types.Filter{Name: &name, Values: vals})
			break
		}
	}
	return out
}

// dropMismatched keeps the items that satisfy the full query of every filter,
// for the parts the API could not check (AND within an alternative, NOT).
func dropMismatched[T any](items []T, specs []filterSpec, fs []provider.Filter, field func(T, string) ([]string, bool)) []T {
	type rule struct {
		api  string
		spec filterSpec
		expr query.Expr
	}
	var rules []rule
	for _, f := range fs {
		expr := query.Parse(f.Value)
		if expr.Empty() || expr.ExactOnServer() {
			continue
		}
		for _, s := range specs {
			if s.key == f.Key {
				rules = append(rules, rule{api: s.api, spec: s, expr: expr})
			}
		}
	}
	if len(rules) == 0 {
		return items
	}
	out := items[:0:0]
next:
	for _, it := range items {
		for _, r := range rules {
			have, _ := field(it, r.api)
			if !r.expr.Match(func(term string) bool {
				for _, h := range have {
					if wild(r.spec.wrap(term), h) {
						return true
					}
				}
				return false
			}) {
				continue next
			}
		}
		out = append(out, it)
	}
	return out
}

var ec2Specs = []filterSpec{
	{"instance-name", "Instance Name", "tag:Name", false},
	{"auto-scaling-group", "AutoScaling Group Name", "tag:aws:autoscaling:groupName", false},
	{"private-ip", "Private IP", "private-ip-address", false},
	{"instance-type", "Instance Type", "instance-type", false},
	{"public-ip", "Public IP", "network-interface.association.public-ip", false},
	{"availability-zone", "Availability Zone", "availability-zone", false},
	{"state", "Instance State", "instance-state-name", false},
	{"instance-id", "Instance ID", "instance-id", true},
	{"subnet-id", "Subnet ID", "subnet-id", true},
}

var vpcSpecs = []filterSpec{
	{"vpc-name", "Vpc Name", "tag:Name", false},
	{"vpc-id", "Vpc ID", "vpc-id", true},
	{"vpc-cidr", "Vpc Cidr", "cidr", false},
	{"vpc-default", "Is Default Vpc", "is-default", false},
	{"vpc-state", "Vpc State", "state", false},
}

var subnetSpecs = []filterSpec{
	{"subnet-name", "Subnet Name", "tag:Name", false},
	{"subnet-id", "Subnet ID", "subnet-id", true},
	{"vpc-id", "Vpc ID", "vpc-id", true},
}

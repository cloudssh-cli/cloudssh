package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
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

// splitValues splits a filter value on commas: plain items are alternatives
// (or), items starting with "!" are exclusions (not).
func splitValues(v string) (include, exclude []string) {
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
		case strings.HasPrefix(p, "!"):
			if rest := strings.TrimSpace(p[1:]); rest != "" {
				exclude = append(exclude, rest)
			}
		default:
			include = append(include, p)
		}
	}
	return include, exclude
}

func (s filterSpec) wrap(v string) string {
	if s.exact {
		return v
	}
	return "*" + v + "*"
}

// buildFilters converts the positive part of UI filters to EC2 filters
// ("a,b" becomes two alternative values). Empty values and unknown keys are
// skipped; exclusions are applied afterwards by dropExcluded because the EC2
// API cannot express "not".
func buildFilters(specs []filterSpec, fs []provider.Filter) []types.Filter {
	var out []types.Filter
	for _, f := range fs {
		include, _ := splitValues(f.Value)
		if len(include) == 0 {
			continue
		}
		for _, s := range specs {
			if s.key != f.Key {
				continue
			}
			vals := make([]string, len(include))
			for i, v := range include {
				vals[i] = s.wrap(v)
			}
			name := s.api
			out = append(out, types.Filter{Name: &name, Values: vals})
			break
		}
	}
	return out
}

// dropExcluded removes items matching any "!" value of the given filters.
func dropExcluded[T any](items []T, specs []filterSpec, fs []provider.Filter, field func(T, string) ([]string, bool)) []T {
	type rule struct {
		api  string
		vals []string
	}
	var rules []rule
	for _, f := range fs {
		_, exclude := splitValues(f.Value)
		for _, s := range specs {
			if s.key == f.Key && len(exclude) > 0 {
				r := rule{api: s.api}
				for _, v := range exclude {
					r.vals = append(r.vals, s.wrap(v))
				}
				rules = append(rules, r)
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
			for _, v := range r.vals {
				for _, h := range have {
					if wild(v, h) {
						continue next
					}
				}
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

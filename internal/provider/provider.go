// Package provider defines the cloud-agnostic contracts the UI depends on.
// Each cloud (aws, later azure/gcp) implements Provider in its own subpackage.
package provider

import "context"

// Column describes one table column.
type Column struct {
	Title string
	// Numeric columns sort by value instead of lexically.
	Numeric bool
	// DefaultSort marks the column the table is sorted by on first load:
	// "" (none), "asc" or "desc".
	DefaultSort string
}

// Row is one table row; Cells align with Resource.Columns. Ref carries the
// provider-specific object so actions (ssh, drill-down) can act on the selection.
type Row struct {
	Cells []string
	Ref   any
}

// Page is one page of results. An empty NextToken means no more pages.
type Page struct {
	Rows      []Row
	NextToken string
}

// FilterDef describes a server-side filter a resource accepts.
type FilterDef struct {
	Key  string // stable identifier, e.g. "instance-name"
	Name string // human label, e.g. "Instance Name"
}

// Filter is an applied filter: a FilterDef.Key plus the user's value.
type Filter struct {
	Key   string
	Value string
}

// Query is what the UI asks a Resource for.
type Query struct {
	Token   string // pagination token from the previous Page ("" = first page)
	Filters []Filter
}

// Resource is a browsable resource type (EC2 instances, ECS clusters, ...).
type Resource interface {
	// Key is the short mode id ("ec2"); Label the dashboard label ("EC2 Instances").
	Key() string
	Label() string
	// Name is the table title; Crumb the breadcrumb segment.
	Name() string
	Crumb() string
	Columns() []Column
	// Filters lists the filters the user may apply (empty = not filterable).
	Filters() []FilterDef
	// DefaultFilters are applied on first load and restored by "reset".
	DefaultFilters() []Filter
	List(ctx context.Context, q Query) (Page, error)
}

// Drill is the result of drilling into a row: a child resource and the
// filters it starts with.
type Drill struct {
	Resource Resource
	Filters  []Filter
}

// Drillable is implemented by resources whose rows open another resource.
// It may call the cloud API (e.g. resolve an ECS task to its EC2 instance),
// so the UI runs it off the update loop. Returning an error shows it to the user.
type Drillable interface {
	// DrillHint describes the action for the key-hint header, e.g. "Subnets View".
	DrillHint() string
	Drill(ctx context.Context, row Row) (Drill, error)
}

// Provider is a cloud backend.
type Provider interface {
	Name() string
	// Resources are the top-level modes, in shortcut order (1, 2, ...).
	Resources() []Resource
}

// Host is anything that can be connected to over SSH.
type Host struct {
	ID        string
	Name      string
	PrivateIP string
	PublicIP  string
	KeyName   string
	ImageID   string
	AZ        string // needed by short-lived key services such as EC2 Instance Connect
	// DefaultUser is a best-effort guess derived from the image, may be empty.
	DefaultUser string
}

// HostResolver is optionally implemented by a Provider to enrich a Host just
// before connecting (e.g. look up the default SSH user from the AMI).
type HostResolver interface {
	ResolveHost(ctx context.Context, h Host) (Host, error)
}

// Authorizer is optionally implemented by a Provider that can grant
// short-lived SSH access without a pre-shared key (AWS EC2 Instance Connect):
// it installs publicKey for user on the host for a short time.
type Authorizer interface {
	// AuthLabel names the mechanism in the UI, e.g. "EC2 Instance Connect".
	AuthLabel() string
	AuthorizeSSH(ctx context.Context, h Host, user, publicKey string) error
}

// ProfileInfo describes one locally configured profile.
type ProfileInfo struct {
	Name   string
	Kind   string // how it signs in: "SSO", "access keys", "assumed role", ...
	Region string
	Detail string // e.g. the SSO account/role or the role ARN
}

// Authenticator is optionally implemented by a Provider whose credentials or
// profile can be chosen at runtime. The provider starts even when it cannot
// sign in; the UI then explains the problem and offers the way forward.
type Authenticator interface {
	// AuthError is nil when ready. Otherwise it is a friendly explanation:
	// the first line is a headline, the rest is guidance.
	AuthError() error
	// Access says how the app is signed in, for the header: "profile prod · SSO",
	// "environment keys", "instance role", ...
	Access() string
	// Region is the region in use, if known.
	Region() string
	// Profile is the profile in use ("" when none, e.g. environment keys).
	Profile() string
	// Profiles lists the profiles configured on this machine (may be empty).
	Profiles() []ProfileInfo
	// UseProfile (re)connects with profile ("" = default credential chain) and
	// returns the same friendly error AuthError would. Calling it with the
	// current profile retries, e.g. after the user logged in elsewhere.
	UseProfile(ctx context.Context, profile string) error
}

// Package provider defines the cloud-agnostic contracts the UI depends on.
// Each cloud lives in its own subpackage (provider/aws, ...), implements these
// interfaces and registers a factory with internal/cloud; the UI and CLI never
// import a cloud directly.
//
// Required: Provider (name + top-level Resources) and Resource.
//
// Optional capabilities, discovered by type assertion. Implement only what the
// cloud can do; the UI adapts:
//
//	Drillable      a row opens a child resource (e.g. group -> machines)
//	HostResolver   enrich a Host just before ssh (e.g. default user from the image)
//	Authenticator  sign-in state, profile list and switching, "how am I signed in"
//	Authorizer     a short-lived key service (AWS: EC2 Instance Connect)
//	Demoer         the provider runs on built-in sample data (cloudssh --demo)
//
// providertest.Conformance checks a provider against this contract.
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
	// Short is a compact label for the header ("name"); defaults to Key.
	Short string
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

// Resource is a browsable resource type (virtual machines, clusters, networks, ...).
type Resource interface {
	// Key is the short, unique mode id ("vm"); Label the dashboard label ("Virtual Machines").
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
// It may call the cloud API (e.g. resolve a container task to its host machine),
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
	AZ        string // zone; needed by some short-lived key services
	// DefaultUser is a best-effort guess derived from the image, may be empty.
	DefaultUser string
}

// HostResolver is optionally implemented by a Provider to enrich a Host just
// before connecting (e.g. look up the default SSH user from the AMI).
type HostResolver interface {
	ResolveHost(ctx context.Context, h Host) (Host, error)
}

// Authorizer is optionally implemented by a Provider that can grant
// short-lived SSH access without a pre-shared key (e.g. AWS EC2 Instance Connect):
// it installs publicKey for user on the host for a short time.
type Authorizer interface {
	// AuthLabel names the mechanism in the UI, e.g. "EC2 Instance Connect" on AWS.
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
	// Access says how the app is signed in, for the header: the kind of
	// credential and, when there is one, the profile: "SSO (prod)",
	// "environment keys", "instance role".
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

// Demoer is implemented by providers running on built-in sample data
// (cloudssh --demo). The UI marks the session and never starts real ssh.
type Demoer interface{ Demo() bool }

// Options is what the CLI hands a cloud's factory (see internal/cloud).
type Options struct {
	Profile  string // sign-in profile ("" = the cloud's default chain)
	Region   string
	PageSize int32
	Demo     bool // run on built-in sample data instead of a real account
}

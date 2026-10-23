// Package provider defines the cloud-agnostic contracts the UI depends on.
// Each cloud (aws, later azure/gcp) implements Provider in its own subpackage.
package provider

import "context"

// Column describes one table column.
type Column struct {
	Title string
	// Numeric columns sort by value instead of lexically.
	Numeric bool
}

// Row is one table row; Cells align with Resource.Columns. Ref carries the
// provider-specific object so actions (e.g. ssh) can act on the selection.
type Row struct {
	Cells []string
	Ref   any
}

// Page is one page of results. An empty NextToken means no more pages.
type Page struct {
	Rows      []Row
	NextToken string
}

// Resource is a browsable resource type (EC2 instances, ECS clusters, ...).
type Resource interface {
	Name() string
	Columns() []Column
	List(ctx context.Context, nextToken string) (Page, error)
}

// Provider is a cloud backend.
type Provider interface {
	Name() string
	Resources() []Resource
}

// Host is anything that can be connected to over SSH.
type Host struct {
	ID        string
	PrivateIP string
	PublicIP  string
	KeyName   string
	ImageID   string
	// DefaultUser is a best-effort guess derived from the image, may be empty.
	DefaultUser string
}

// HostResolver is optionally implemented by a Provider to enrich a Host just
// before connecting (e.g. look up the default SSH user from the AMI).
type HostResolver interface {
	ResolveHost(ctx context.Context, h Host) (Host, error)
}

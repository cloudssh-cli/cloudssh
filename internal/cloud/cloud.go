// Package cloud is the registry of cloud backends. Each cloud lives in its own
// package under internal/provider/<name> and registers a Factory from init();
// the CLI imports that package for its side effect (see internal/cli/clouds.go)
// and then only ever calls Open. Nothing outside a cloud's own package knows
// which clouds exist, so adding one never touches the UI or the CLI logic.
package cloud

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// Factory builds a ready-to-use provider. It must not fail merely because the
// user is not signed in: return a provider that implements
// provider.Authenticator and reports the reason through AuthError, so the UI
// can start and explain. With o.Demo it must return a provider backed by
// built-in sample data (or an error saying demo data is not available yet).
type Factory func(ctx context.Context, o provider.Options) (provider.Provider, error)

// Cloud describes one registered backend.
type Cloud struct {
	Name        string // flag/config value, lower-case: "aws", "gcp", "azure"
	Description string // shown in --help
	Open        Factory
}

var (
	mu       sync.RWMutex
	registry = map[string]Cloud{}
)

// Register adds a cloud. It panics on an empty or duplicate name or a nil
// factory: those are programming errors caught the first time tests run.
func Register(c Cloud) {
	mu.Lock()
	defer mu.Unlock()
	switch {
	case c.Name == "" || c.Name != strings.ToLower(c.Name):
		panic(fmt.Sprintf("cloud: invalid name %q (must be non-empty lower-case)", c.Name))
	case c.Open == nil:
		panic(fmt.Sprintf("cloud: %q has no factory", c.Name))
	}
	if _, dup := registry[c.Name]; dup {
		panic(fmt.Sprintf("cloud: %q registered twice", c.Name))
	}
	registry[c.Name] = c
}

// Names lists the registered clouds, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All lists the registered clouds sorted by name.
func All() []Cloud {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Cloud, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Open builds the provider for the named cloud.
func Open(ctx context.Context, name string, o provider.Options) (provider.Provider, error) {
	mu.RLock()
	c, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("cloud %q is not supported (available: %s)", name, strings.Join(Names(), ", "))
	}
	return c.Open(ctx, o)
}

package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type stub struct{ opts provider.Options }

func (stub) Name() string                   { return "stub" }
func (stub) Resources() []provider.Resource { return nil }

// isolate swaps in an empty registry for one test.
func isolate(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := registry
	registry = map[string]Cloud{}
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); registry = saved; mu.Unlock() })
}

func TestRegisterAndOpenPassOptionsThrough(t *testing.T) {
	isolate(t)
	var got provider.Options
	Register(Cloud{Name: "fake", Description: "test", Open: func(_ context.Context, o provider.Options) (provider.Provider, error) {
		got = o
		return stub{o}, nil
	}})
	want := provider.Options{Profile: "p", Region: "r", PageSize: 7, Demo: true}
	p, err := Open(context.Background(), "fake", want)
	if err != nil || p == nil || got != want {
		t.Fatalf("p=%v err=%v got=%+v", p, err, got)
	}
}

func TestOpenUnknownCloudListsWhatIsAvailable(t *testing.T) {
	isolate(t)
	Register(Cloud{Name: "zeta", Open: func(context.Context, provider.Options) (provider.Provider, error) { return stub{}, nil }})
	Register(Cloud{Name: "alpha", Open: func(context.Context, provider.Options) (provider.Provider, error) { return stub{}, nil }})
	_, err := Open(context.Background(), "nope", provider.Options{})
	if err == nil || !strings.Contains(err.Error(), `"nope" is not supported`) || !strings.Contains(err.Error(), "alpha, zeta") {
		t.Fatalf("error: %v", err)
	}
	if n := Names(); len(n) != 2 || n[0] != "alpha" {
		t.Fatalf("Names must be sorted: %v", n)
	}
}

func TestRegisterRejectsMistakes(t *testing.T) {
	isolate(t)
	ok := func(context.Context, provider.Options) (provider.Provider, error) { return stub{}, nil }
	for name, c := range map[string]Cloud{
		"empty name": {Open: ok}, "upper case": {Name: "AWS", Open: ok}, "no factory": {Name: "x"},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: Register should panic", name)
				}
			}()
			Register(c)
		}()
	}
	Register(Cloud{Name: "dup", Open: ok})
	defer func() {
		if recover() == nil {
			t.Error("duplicate registration should panic")
		}
	}()
	Register(Cloud{Name: "dup", Open: ok})
}

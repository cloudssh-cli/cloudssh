package providertest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// recorder is a TB that collects failures instead of failing the real test.
type recorder struct{ errs []string }

type stop struct{}

func (r *recorder) Helper()                   {}
func (r *recorder) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) {
	r.errs = append(r.errs, fmt.Sprintf(f, a...))
	panic(stop{})
}
func (r *recorder) Fatal(a ...any) { r.errs = append(r.errs, fmt.Sprint(a...)); panic(stop{}) }
func (r *recorder) joined() string { return strings.Join(r.errs, "\n") }

func run(p provider.Provider, o Options) *recorder {
	r := &recorder{}
	func() {
		defer func() {
			if v := recover(); v != nil {
				if _, ok := v.(stop); !ok {
					panic(v)
				}
			}
		}()
		Conformance(r, p, o)
	}()
	return r
}

// toyResource is a configurable resource used to build good and bad providers.
type toyResource struct {
	key      string
	cols     []provider.Column
	filters  []provider.FilterDef
	defaults []provider.Filter
	pages    map[string]provider.Page
	listErr  error
	drill    func(provider.Row) (provider.Drill, error)
	hint     string
}

func (t toyResource) Key() string                       { return t.key }
func (t toyResource) Label() string                     { return "Toy " + t.key }
func (t toyResource) Name() string                      { return "Toys" }
func (t toyResource) Crumb() string                     { return "toys_" + t.key }
func (t toyResource) Columns() []provider.Column        { return t.cols }
func (t toyResource) Filters() []provider.FilterDef     { return t.filters }
func (t toyResource) DefaultFilters() []provider.Filter { return t.defaults }
func (t toyResource) List(_ context.Context, q provider.Query) (provider.Page, error) {
	if t.listErr != nil {
		return provider.Page{}, t.listErr
	}
	pg, ok := t.pages[q.Token]
	if !ok {
		return provider.Page{}, nil
	}
	if len(q.Filters) > 0 && q.Filters[0].Value == "nomatch" {
		return provider.Page{}, nil
	}
	return pg, nil
}

type drillToy struct{ toyResource }

func (d drillToy) DrillHint() string { return d.hint }
func (d drillToy) Drill(_ context.Context, r provider.Row) (provider.Drill, error) {
	return d.drill(r)
}

type toyProvider struct{ res []provider.Resource }

func (toyProvider) Name() string                     { return "toy" }
func (p toyProvider) Resources() []provider.Resource { return p.res }

func goodCols() []provider.Column {
	return []provider.Column{{Title: "ID", DefaultSort: "asc"}, {Title: "Name"}}
}
func row(id string) provider.Row {
	return provider.Row{Cells: []string{id, "name-" + id}, Ref: provider.Host{ID: id}}
}
func goodPages() map[string]provider.Page {
	return map[string]provider.Page{"": {Rows: []provider.Row{row("a")}, NextToken: "t2"}, "t2": {Rows: []provider.Row{row("b")}}}
}

func TestSuiteAcceptsACorrectProvider(t *testing.T) {
	leaf := toyResource{key: "leaf", cols: goodCols(), pages: goodPages(), filters: []provider.FilterDef{{Key: "id", Name: "ID"}}}
	root := drillToy{toyResource{key: "root", cols: goodCols(), pages: goodPages(), hint: "Leaf View",
		filters: []provider.FilterDef{{Key: "name", Name: "Name"}},
		drill: func(r provider.Row) (provider.Drill, error) {
			return provider.Drill{Resource: leaf, Filters: []provider.Filter{{Key: "id", Value: r.Cells[0]}}}, nil
		}}}
	rec := run(toyProvider{[]provider.Resource{root}}, Options{
		WantKeys: []string{"root", "leaf"}, FilterSamples: map[string]string{"name": "@col:Name"},
	})
	if len(rec.errs) != 0 {
		t.Fatalf("a correct provider should pass:\n%s", rec.joined())
	}
}

func TestSuiteCatchesTheMistakesCommunityCodeMakes(t *testing.T) {
	cases := map[string]struct {
		p    provider.Provider
		o    Options
		want string
	}{
		"row has fewer cells than columns": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"only"}, Ref: 1}}}}}}},
			Options{}, "cells for 2 columns"},
		"missing Ref": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "b"}}}}}}}},
			Options{}, "no Ref"},
		"duplicate column titles": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: []provider.Column{{Title: "ID"}, {Title: "ID"}}, pages: map[string]provider.Page{"": {}}}}},
			Options{}, "duplicate column title"},
		"two default sorts": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: []provider.Column{{Title: "A", DefaultSort: "asc"}, {Title: "B", DefaultSort: "desc"}}, pages: map[string]provider.Page{"": {}}}}},
			Options{}, "only one may"},
		"duplicate top-level keys": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols()}, toyResource{key: "x", cols: goodCols()}}},
			Options{}, "duplicate top-level resource key"},
		"default filter not advertised": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), defaults: []provider.Filter{{Key: "ghost", Value: "1"}}}}},
			Options{}, "default filter \"ghost\" is not in Filters()"},
		"page token never advances": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{row("a")}, NextToken: "same"}, "same": {Rows: []provider.Row{row("b")}, NextToken: "same"}}}}},
			Options{}, "did not advance"},
		"list error": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), listErr: errors.New("boom")}}},
			Options{}, "boom"},
		"resource without sample data": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {}}}}},
			Options{WantKeys: []string{"x"}}, "has no rows"},
		"resource never reached": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: goodPages()}}},
			Options{WantKeys: []string{"x", "orphan"}}, `"orphan" was never reached`},
		"filter nothing proves": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: goodPages(), filters: []provider.FilterDef{{Key: "id", Name: "ID"}}}}},
			Options{}, "is not proven by the sample data"},
		"filter sample matches nothing": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: goodPages(), filters: []provider.FilterDef{{Key: "id", Name: "ID"}}}}},
			Options{FilterSamples: map[string]string{"id": "nomatch"}}, "returned no rows although unfiltered data exists"},
		"drill applies a filter the target does not list": {
			toyProvider{[]provider.Resource{drillToy{toyResource{key: "root", cols: goodCols(), pages: goodPages(), hint: "Go",
				drill: func(provider.Row) (provider.Drill, error) {
					return provider.Drill{Resource: toyResource{key: "kid", cols: goodCols(), pages: goodPages()}, Filters: []provider.Filter{{Key: "secret", Value: "1"}}}, nil
				}}}}},
			Options{}, "does not list in Filters()"},
		"unexplained drill error": {
			toyProvider{[]provider.Resource{drillToy{toyResource{key: "root", cols: goodCols(), pages: goodPages(), hint: "Go",
				drill: func(provider.Row) (provider.Drill, error) { return provider.Drill{}, errors.New("kaboom") }}}}},
			Options{}, "Drill failed"},
		"drill without hint": {
			toyProvider{[]provider.Resource{drillToy{toyResource{key: "root", cols: goodCols(), pages: goodPages(),
				drill: func(provider.Row) (provider.Drill, error) { return provider.Drill{}, errors.New("x") }}}}},
			Options{DrillErrors: []string{"x"}}, "DrillHint must not be empty"},
		"partial row without an enricher": {
			toyProvider{[]provider.Resource{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "…"}, Ref: 1, ID: "a", Partial: true}}}}}}},
			Options{}, "does not implement Enricher"},
		"partial row without an id": {
			toyProvider{[]provider.Resource{enrichToy{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "…"}, Ref: 1, Partial: true}}}}}, func(r provider.Row) provider.Row { return r }}}},
			Options{}, "needs an ID"},
		"enrich leaves the row partial": {
			toyProvider{[]provider.Resource{enrichToy{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "…"}, Ref: 1, ID: "a", Partial: true}}}}}, func(r provider.Row) provider.Row { return r }}}},
			Options{}, "still Partial"},
		"enrich returns the wrong number of cells": {
			toyProvider{[]provider.Resource{enrichToy{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "…"}, Ref: 1, ID: "a", Partial: true}}}}}, func(r provider.Row) provider.Row { r.Partial, r.Cells = false, []string{"a"}; return r }}}},
			Options{}, "cells for 2 columns"},
	}
	for name, c := range cases {
		rec := run(c.p, c.o)
		if !strings.Contains(rec.joined(), c.want) {
			t.Errorf("%s: suite should report %q, got:\n%s", name, c.want, rec.joined())
		}
	}
}

func TestSuiteAllowsDeclaredDrillGuards(t *testing.T) {
	root := drillToy{toyResource{key: "root", cols: goodCols(), pages: goodPages(), hint: "Go",
		drill: func(provider.Row) (provider.Drill, error) {
			return provider.Drill{}, errors.New("cluster has no services")
		}}}
	if rec := run(toyProvider{[]provider.Resource{root}}, Options{DrillErrors: []string{"has no services"}}); len(rec.errs) != 0 {
		t.Fatalf("declared guards are fine:\n%s", rec.joined())
	}
}

// enrichToy adds an Enricher to a toyResource.
type enrichToy struct {
	toyResource
	fn func(provider.Row) provider.Row
}

func (e enrichToy) Enrich(_ context.Context, r provider.Row) (provider.Row, error) {
	return e.fn(r), nil
}

func TestSuiteAcceptsAWellBehavedEnricher(t *testing.T) {
	res := enrichToy{toyResource{key: "x", cols: goodCols(), pages: map[string]provider.Page{"": {Rows: []provider.Row{{Cells: []string{"a", "…"}, Ref: 1, ID: "a", Partial: true}}}}},
		func(r provider.Row) provider.Row { r.Partial, r.Cells = false, []string{"a", "full"}; return r }}
	if rec := run(toyProvider{[]provider.Resource{res}}, Options{WantKeys: []string{"x"}}); len(rec.errs) != 0 {
		t.Fatalf("a correct enricher should pass:\n%s", rec.joined())
	}
}

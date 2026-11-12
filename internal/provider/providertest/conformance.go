// Package providertest holds the conformance suite every cloud provider must
// pass. A cloud adds one test that runs Conformance over its demo provider
// (built-in sample data), which proves, without any real account, that every
// resource lists, pages, filters and drills the way the UI expects.
//
//	func TestDemoConformance(t *testing.T) {
//	    providertest.Conformance(t, NewDemo(8), providertest.Options{ ... })
//	}
package providertest

import (
	"context"
	"sort"
	"strings"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// Options tell the suite what the provider's sample data must demonstrate.
type Options struct {
	// WantKeys lists every resource key (top-level and drill-only) that must
	// be reached and show at least one row. Build it from the same list your
	// metadata test uses, so adding a resource without sample data fails.
	WantKeys []string
	// EmptyOK lists keys from WantKeys that may legitimately have no rows.
	EmptyOK []string
	// FilterSamples proves each filter works. Key is "resourceKey:filterKey"
	// (filter keys such as "vpc-id" can repeat across resources) or just the
	// filterKey. The value is what to filter for; "@col:Title" means "the first
	// non-empty cell of that column in the unfiltered rows". A filter that a
	// drill-down applies successfully needs no sample.
	FilterSamples map[string]string
	// DrillErrors are substrings of acceptable drill refusals ("no services",
	// "Fargate"): guards for rows that have nothing to open.
	DrillErrors []string
	MaxDepth    int // default 5
	MaxPages    int // default 100
}

// TB is the slice of *testing.T the suite needs, so the suite itself can be
// tested against deliberately broken providers.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Fatal(args ...any)
}

type walker struct {
	t       TB
	o       Options
	ctx     context.Context
	seen    map[string]bool // key|filters already walked
	checked map[string]bool // resource keys whose filters were proven
	data    map[string]bool // resource key -> had rows
	visited map[string]bool
	res     map[string]provider.Resource // every resource reached, by key
	drilled map[string]bool              // "resourceKey:filterKey" applied by a drill that returned rows
	host    *provider.Host
}

func Conformance(t TB, p provider.Provider, o Options) {
	t.Helper()
	if o.MaxDepth == 0 {
		o.MaxDepth = 5
	}
	if o.MaxPages == 0 {
		o.MaxPages = 100
	}
	w := &walker{t: t, o: o, ctx: context.Background(), seen: map[string]bool{}, checked: map[string]bool{}, data: map[string]bool{}, visited: map[string]bool{}, res: map[string]provider.Resource{}, drilled: map[string]bool{}}

	if p.Name() == "" {
		t.Fatal("Provider.Name must not be empty")
	}
	res := p.Resources()
	if len(res) == 0 {
		t.Fatal("Provider.Resources must list at least one top-level resource")
	}
	keys := map[string]bool{}
	for _, r := range res {
		if keys[r.Key()] {
			t.Errorf("duplicate top-level resource key %q", r.Key())
		}
		keys[r.Key()] = true
		w.walk(r, r.DefaultFilters(), 0)
	}

	for _, k := range o.WantKeys {
		switch {
		case !w.visited[k]:
			t.Errorf("resource %q was never reached: the sample data (or its drill-down) must cover it", k)
		case !w.data[k] && !contains(o.EmptyOK, k):
			t.Errorf("resource %q was reached but has no rows in the sample data", k)
		}
	}
	w.checkAuthAndHosts(p)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (w *walker) checkMetadata(r provider.Resource) {
	t := w.t
	t.Helper()
	for what, v := range map[string]string{"Key": r.Key(), "Label": r.Label(), "Name": r.Name(), "Crumb": r.Crumb()} {
		if strings.TrimSpace(v) == "" {
			t.Errorf("%T: %s must not be empty", r, what)
		}
	}
	cols := r.Columns()
	if len(cols) == 0 {
		t.Fatalf("%s: no columns", r.Key())
	}
	titles, sorts := map[string]bool{}, 0
	for _, c := range cols {
		switch {
		case strings.TrimSpace(c.Title) == "":
			t.Errorf("%s: empty column title", r.Key())
		case titles[c.Title]:
			t.Errorf("%s: duplicate column title %q (sort keys and filters refer to titles)", r.Key(), c.Title)
		}
		titles[c.Title] = true
		switch c.DefaultSort {
		case "":
		case "asc", "desc":
			sorts++
		default:
			t.Errorf("%s: column %q has DefaultSort %q (want asc, desc or empty)", r.Key(), c.Title, c.DefaultSort)
		}
	}
	if sorts > 1 {
		t.Errorf("%s: %d columns set DefaultSort; only one may", r.Key(), sorts)
	}
	fk := map[string]bool{}
	for _, f := range r.Filters() {
		if f.Key == "" || f.Name == "" {
			t.Errorf("%s: filter needs Key and Name: %+v", r.Key(), f)
		}
		if fk[f.Key] {
			t.Errorf("%s: duplicate filter key %q", r.Key(), f.Key)
		}
		fk[f.Key] = true
	}
	for _, f := range r.DefaultFilters() {
		if !fk[f.Key] {
			t.Errorf("%s: default filter %q is not in Filters()", r.Key(), f.Key)
		}
	}
}

// list reads every page, checking each row against the columns.
func (w *walker) list(r provider.Resource, filters []provider.Filter) []provider.Row {
	t := w.t
	t.Helper()
	var rows []provider.Row
	token := ""
	for page := 0; ; page++ {
		if page >= w.o.MaxPages {
			t.Fatalf("%s: still paging after %d pages; NextToken must end", r.Key(), page)
		}
		pg, err := r.List(w.ctx, provider.Query{Token: token, Filters: filters})
		if err != nil {
			t.Fatalf("%s: List(token=%q, filters=%v): %v", r.Key(), token, filters, err)
		}
		if page > 0 && len(pg.Rows) == 0 && pg.NextToken != "" {
			t.Errorf("%s: an empty page must not announce another page", r.Key())
		}
		for i, row := range pg.Rows {
			if len(row.Cells) != len(r.Columns()) {
				t.Fatalf("%s: row %d has %d cells for %d columns: %v", r.Key(), i, len(row.Cells), len(r.Columns()), row.Cells)
			}
			if row.Ref == nil {
				t.Errorf("%s: row %d has no Ref (drill-down and ssh act on it)", r.Key(), i)
			}
			if h, ok := row.Ref.(provider.Host); ok {
				if h.ID == "" {
					t.Errorf("%s: Host Ref without ID", r.Key())
				}
				if w.host == nil {
					hh := h
					w.host = &hh
				}
			}
		}
		rows = append(rows, pg.Rows...)
		if pg.NextToken == "" {
			return rows
		}
		if pg.NextToken == token {
			t.Fatalf("%s: NextToken did not advance (%q)", r.Key(), token)
		}
		token = pg.NextToken
	}
}

func sig(r provider.Resource, f []provider.Filter) string {
	parts := make([]string, len(f))
	for i, x := range f {
		parts[i] = x.Key + "=" + x.Value
	}
	sort.Strings(parts)
	return r.Key() + "|" + r.Name() + "|" + strings.Join(parts, ",")
}

func (w *walker) walk(r provider.Resource, filters []provider.Filter, depth int) {
	t := w.t
	t.Helper()
	if depth > w.o.MaxDepth {
		t.Errorf("%s: drill-down deeper than %d levels (a cycle?)", r.Key(), w.o.MaxDepth)
		return
	}
	s := sig(r, filters)
	if w.seen[s] {
		return
	}
	w.seen[s] = true
	w.checkMetadata(r)
	w.visited[r.Key()] = true
	w.res[r.Key()] = r
	rows := w.list(r, filters)
	if len(rows) > 0 {
		w.data[r.Key()] = true
	}

	if !w.checked[r.Key()] {
		w.checked[r.Key()] = true
		w.checkFilters(r, w.list(r, nil)) // proven against the resource's own unfiltered rows
	}
	d, ok := r.(provider.Drillable)
	if !ok {
		return
	}
	if strings.TrimSpace(d.DrillHint()) == "" {
		t.Errorf("%s: DrillHint must not be empty", r.Key())
	}
	for _, row := range rows {
		dr, err := d.Drill(w.ctx, row)
		if err != nil {
			if !w.allowed(err) {
				t.Errorf("%s: Drill failed with %q; if this is a guard for rows with nothing to open, list it in Options.DrillErrors", r.Key(), err)
			}
			continue
		}
		if dr.Resource == nil {
			t.Errorf("%s: Drill returned no resource", r.Key())
			continue
		}
		known := map[string]bool{}
		for _, f := range dr.Resource.Filters() {
			known[f.Key] = true
		}
		for _, f := range dr.Filters {
			if !known[f.Key] {
				t.Errorf("%s -> %s: drill applies filter %q that the target does not list in Filters(); the filters dialog could not show it", r.Key(), dr.Resource.Key(), f.Key)
			}
		}
		child := dr.Filters
		if len(child) == 0 {
			child = dr.Resource.DefaultFilters()
		}
		if got := w.list(dr.Resource, child); len(got) > 0 {
			for _, f := range dr.Filters {
				w.drilled[dr.Resource.Key()+":"+f.Key] = true
			}
		}
		w.walk(dr.Resource, child, depth+1)
	}
}

func (w *walker) allowed(err error) bool {
	for _, s := range w.o.DrillErrors {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}

// checkFilters proves every filter the resource advertises returns data.
func (w *walker) checkFilters(r provider.Resource, unfiltered []provider.Row) {
	t := w.t
	t.Helper()
	for _, f := range r.Filters() {
		if w.drilled[r.Key()+":"+f.Key] {
			continue // a drill-down already applied it and got rows
		}
		sample, ok := w.o.FilterSamples[r.Key()+":"+f.Key]
		if !ok {
			sample, ok = w.o.FilterSamples[f.Key]
		}
		if !ok {
			// it may be proven later by a drill that has not run yet: re-checked at the end
			continue
		}
		val, ok := w.resolve(r, sample, unfiltered)
		if !ok {
			if len(unfiltered) > 0 {
				t.Errorf("%s: filter %q sample %q could not be resolved from the rows", r.Key(), f.Key, sample)
			}
			continue
		}
		if len(unfiltered) == 0 {
			continue
		}
		if got := w.list(r, []provider.Filter{{Key: f.Key, Value: val}}); len(got) == 0 {
			t.Errorf("%s: filter %q with %q returned no rows although unfiltered data exists", r.Key(), f.Key, val)
		}
		w.drilled[r.Key()+":"+f.Key] = true
	}
}

func (w *walker) resolve(r provider.Resource, sample string, rows []provider.Row) (string, bool) {
	col, ok := strings.CutPrefix(sample, "@col:")
	if !ok {
		return sample, true
	}
	idx := -1
	for i, c := range r.Columns() {
		if c.Title == col {
			idx = i
		}
	}
	if idx < 0 {
		return "", false
	}
	for _, row := range rows {
		if row.Cells[idx] != "" {
			return row.Cells[idx], true
		}
	}
	return "", false
}

// checkAuthAndHosts covers the optional capabilities the sample data exposes.
func (w *walker) checkAuthAndHosts(p provider.Provider) {
	t := w.t
	t.Helper()
	// every advertised filter must be proven by a sample or a successful drill
	for _, r := range w.res {
		for _, f := range r.Filters() {
			if !w.drilled[r.Key()+":"+f.Key] && w.data[r.Key()] {
				t.Errorf("filter %s:%s is not proven by the sample data: add Options.FilterSamples[%q] or make a drill-down apply it", r.Key(), f.Key, r.Key()+":"+f.Key)
			}
		}
	}
	if hr, ok := p.(provider.HostResolver); ok && w.host != nil {
		h, err := hr.ResolveHost(w.ctx, *w.host)
		if err != nil || h.ID != w.host.ID {
			t.Errorf("ResolveHost(%s) = %+v, %v", w.host.ID, h, err)
		}
	}
	a, ok := p.(provider.Authenticator)
	if !ok {
		return
	}
	if err := a.AuthError(); err != nil {
		t.Errorf("sample provider must start signed in, got: %v", err)
	}
	if a.Access() == "" || a.Region() == "" {
		t.Errorf("Authenticator: Access=%q Region=%q must both be set when signed in", a.Access(), a.Region())
	}
	profiles := a.Profiles()
	if d, isDemo := p.(provider.Demoer); isDemo && d.Demo() && len(profiles) == 0 {
		t.Errorf("%s", "a demo provider should offer sample profiles so switching can be shown")
	}
	original := a.Profile()
	for _, pr := range profiles {
		if pr.Name == "" || pr.Kind == "" {
			t.Errorf("profile needs Name and Kind: %+v", pr)
		}
		if err := a.UseProfile(w.ctx, pr.Name); err != nil || a.AuthError() != nil {
			t.Errorf("UseProfile(%q): %v", pr.Name, err)
			continue
		}
		if a.Profile() != pr.Name || a.Access() == "" {
			t.Errorf("after UseProfile(%q): Profile=%q Access=%q", pr.Name, a.Profile(), a.Access())
		}
		for _, r := range p.Resources() { // every profile must list without errors (may be empty)
			w.list(r, r.DefaultFilters())
		}
	}
	if err := a.UseProfile(w.ctx, "definitely-not-a-profile"); err == nil || a.AuthError() == nil {
		t.Errorf("%s", "UseProfile with an unknown profile must fail and be reported by AuthError, not panic or succeed")
	} else if first, _, _ := strings.Cut(err.Error(), "\n"); first == "" || strings.Contains(strings.ToLower(first), "failed to") {
		t.Errorf("auth problems need a friendly headline, got %q", first)
	}
	if err := a.UseProfile(w.ctx, original); err != nil {
		t.Errorf("returning to %q: %v", original, err)
	}
}

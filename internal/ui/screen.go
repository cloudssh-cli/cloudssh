package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
	"github.com/cloudssh-cli/cloudssh/internal/query"
)

// screen is one level of a mode's navigation stack: a resource, the filters
// applied to it, its loaded rows and its table state.
type screen struct {
	id       int
	res      provider.Resource
	filters  []provider.Filter
	defaults []provider.Filter // restored by "reset"

	tbl        table
	rows, view []provider.Row
	search     string
	nextToken  string
	pages      int
	loading    bool
	gen        int // bumped on reload so late responses are dropped
	sortKeys   []string
	// focusActive puts the cursor on the active row the next time rows arrive
	// (the profiles screen: opening it should rest on the profile in use).
	focusActive bool
	// budget is how many more pages may still load in the background.
	budget int
	// enriching holds the IDs of rows whose details are being fetched.
	enriching map[string]bool
}

func newScreen(id int, res provider.Resource, filters []provider.Filter, w, h int) *screen {
	s := &screen{id: id, res: res, defaults: cloneFilters(filters), loading: true, tbl: newTable()}
	s.filters = cloneFilters(filters)
	s.tbl.setCols(res.Columns())
	s.sortKeys = assignSortKeys(res.Columns())
	s.applyDefaultSort()
	s.tbl.resize(w, h)
	return s
}

func cloneFilters(f []provider.Filter) []provider.Filter {
	return append([]provider.Filter(nil), f...)
}

func (s *screen) applyDefaultSort() {
	for i, c := range s.res.Columns() {
		if c.DefaultSort != "" {
			s.tbl.setSort(i, c.DefaultSort == "desc")
			return
		}
	}
	s.tbl.setSort(-1, false)
}

// refreshView re-applies search and sort over all loaded rows.
func (s *screen) refreshView() {
	needle := strings.ToLower(s.search)
	filtered := make([]provider.Row, 0, len(s.rows))
	for _, r := range s.rows {
		if matchSearch(needle, strings.ToLower(strings.Join(r.Cells, "\x00"))) {
			filtered = append(filtered, r)
		}
	}
	numeric := false
	if c := s.tbl.sortCol; c >= 0 && c < len(s.tbl.cols) {
		numeric = s.tbl.cols[c].Numeric
	}
	s.view = sortRows(filtered, s.tbl.sortCol, s.tbl.sortDesc, numeric)
	s.tbl.setRows(s.view)
}

// sortBy sorts by column col; sorting by the active column flips the order.
func (s *screen) sortBy(col int) {
	if col == s.tbl.sortCol {
		s.tbl.setSort(col, !s.tbl.sortDesc)
	} else {
		s.tbl.setSort(col, false)
	}
	s.refreshView()
	s.tbl.home()
}

// load fetches a page. The result is routed back by screen id + gen.
func (s *screen) load(token string, appendRows bool) tea.Cmd {
	res, id, gen := s.res, s.id, s.gen
	q := provider.Query{Token: token, Filters: cloneFilters(s.filters)}
	return func() tea.Msg {
		page, err := res.List(context.Background(), q)
		return loadedMsg{sid: id, gen: gen, page: page, append: appendRows, err: err}
	}
}

func (s *screen) reload() tea.Cmd {
	s.gen++
	s.enriching = nil
	s.loading = true
	s.nextToken = ""
	return s.load("", false)
}

func (s *screen) selected() (provider.Row, bool) {
	i := s.tbl.cursor
	if i < 0 || i >= len(s.view) {
		return provider.Row{}, false
	}
	return s.view[i], true
}

func (s *screen) drillable() (provider.Drillable, bool) {
	d, ok := s.res.(provider.Drillable)
	return d, ok
}

// sshable reports whether this screen lists connectable hosts.
func (s *screen) sshable() bool {
	for _, r := range s.rows {
		_, ok := r.Ref.(provider.Host)
		return ok
	}
	return false
}

// short is the compact label of a filter key ("name"), falling back to the key.
func (s *screen) short(key string) string {
	for _, d := range s.res.Filters() {
		if d.Key == key && d.Short != "" {
			return d.Short
		}
	}
	return key
}

// filterChips is the active filters as "state=running name=web", fitted to
// width with a "+N" for what does not fit. Empty when no filter is active.
func (s *screen) filterChips(width int) string {
	var chips []string
	for _, f := range s.filters {
		if f.Value != "" {
			chips = append(chips, s.short(f.Key)+"="+f.Value)
		}
	}
	out := ""
	for i, c := range chips {
		next := out
		if next != "" {
			next += " "
		}
		next += c
		rest := len(chips) - i - 1
		extra := 0
		if rest > 0 {
			extra = len(fmt.Sprintf(" +%d", rest))
		}
		if len(next)+extra > width && out != "" {
			return fmt.Sprintf("%s +%d", out, len(chips)-i)
		}
		out = next
	}
	return fit(strings.TrimRight(out, " "), min(len(out), width))
}

// matchSearch reports whether text (already lower-case) satisfies the search
// query (lower-case); the grammar is shared with filters (see internal/query).
func matchSearch(q, text string) bool {
	return query.Parse(q).Match(func(term string) bool { return strings.Contains(text, term) })
}

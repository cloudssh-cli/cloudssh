package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
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
		if needle == "" || strings.Contains(strings.ToLower(strings.Join(r.Cells, "\x00")), needle) {
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

func (s *screen) filterName(key string) string {
	for _, d := range s.res.Filters() {
		if d.Key == key {
			return d.Name
		}
	}
	return key
}

// filterSummary is e.g. "Instance State=running, Instance Name=web".
func (s *screen) filterSummary() string {
	var parts []string
	for _, f := range s.filters {
		if f.Value != "" {
			parts = append(parts, s.filterName(f.Key)+"="+f.Value)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

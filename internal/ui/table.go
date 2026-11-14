package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

const colGap = 3

// table is a k9s-style data table: bordered panel, bold header row, a
// highlighted cursor row, sortable columns and horizontal column scrolling.
type table struct {
	cols     []provider.Column
	rows     []provider.Row
	widths   []int
	cursor   int
	top      int // first visible row
	colStart int // first visible column
	sortCol  int // -1 = unsorted (API order)
	sortDesc bool

	width, height int // outer size, including the border
}

func newTable() table { return table{sortCol: -1} }

func (t *table) innerWidth() int { return max(t.width-2, 1) }

// rowsH is how many data rows fit (border top/bottom and header excluded).
func (t *table) rowsH() int { return max(t.height-3, 1) }

func (t *table) label(i int) string {
	l := t.cols[i].Title
	if i == t.sortCol {
		if t.sortDesc {
			return l + " ▼"
		}
		return l + " ▲"
	}
	return l
}

func (t *table) recomputeWidths() {
	t.widths = make([]int, len(t.cols))
	for i := range t.cols {
		t.widths[i] = lipgloss.Width(t.label(i))
	}
	for _, r := range t.rows {
		for i, c := range r.Cells {
			if i < len(t.widths) {
				t.widths[i] = max(t.widths[i], lipgloss.Width(c))
			}
		}
	}
}

func (t *table) setCols(cols []provider.Column) {
	t.cols = cols
	t.rows = nil
	t.sortCol, t.sortDesc = -1, false
	t.cursor, t.top, t.colStart = 0, 0, 0
	t.recomputeWidths()
}

func (t *table) setRows(rows []provider.Row) {
	t.rows = rows
	t.recomputeWidths()
	t.cursor = min(max(t.cursor, 0), max(len(rows)-1, 0))
	t.colStart = min(t.colStart, max(len(t.cols)-1, 0))
	t.ensureVisible()
}

func (t *table) setSort(col int, desc bool) {
	t.sortCol, t.sortDesc = col, desc
	t.recomputeWidths()
}

func (t *table) resize(w, h int) {
	t.width, t.height = w, h
	t.ensureVisible()
}

func (t *table) ensureVisible() {
	h := t.rowsH()
	if t.cursor < t.top {
		t.top = t.cursor
	}
	if t.cursor >= t.top+h {
		t.top = t.cursor - h + 1
	}
	t.top = min(max(t.top, 0), max(len(t.rows)-1, 0))
}

func (t *table) move(delta int) {
	if len(t.rows) == 0 {
		return
	}
	t.cursor = min(max(t.cursor+delta, 0), len(t.rows)-1)
	t.ensureVisible()
}

func (t *table) pageUp()   { t.move(-t.rowsH()) }
func (t *table) pageDown() { t.move(t.rowsH()) }
func (t *table) home()     { t.cursor, t.top = 0, 0 }
func (t *table) end()      { t.move(len(t.rows)) }

type vcol struct{ idx, w int }

// visibleCols picks the columns that fit starting at colStart. The last one
// may be shown truncated.
func (t *table) visibleCols() []vcol {
	avail := t.innerWidth() - gutter - 2 // left gutter, then room for a space and the "more columns" marker
	var out []vcol
	used := 0
	for i := t.colStart; i < len(t.cols); i++ {
		gap := 0
		if len(out) > 0 {
			gap = colGap
		}
		w := t.widths[i]
		if used+gap+w <= avail {
			out = append(out, vcol{i, w})
			used += gap + w
			continue
		}
		rem := avail - used - gap
		if len(out) == 0 || rem >= min(w, 8) {
			out = append(out, vcol{i, max(rem, 1)})
		}
		break
	}
	return out
}

func (t *table) scrollRight() {
	vis := t.visibleCols()
	if len(vis) == 0 {
		return
	}
	last := vis[len(vis)-1]
	if last.idx == len(t.cols)-1 && last.w == t.widths[last.idx] {
		return // everything already visible
	}
	t.colStart = min(t.colStart+1, len(t.cols)-1)
}

// gutter is the blank margin left of every row; the header shows « in it when
// columns are scrolled out on the left.
const gutter = 2

func (t *table) scrollLeft() { t.colStart = max(t.colStart-1, 0) }

func (t *table) headerLine(vis []vcol) string {
	parts := make([]string, len(vis))
	for i, v := range vis {
		st := headStyle
		if v.idx == t.sortCol {
			st = headSortStyle
		}
		parts[i] = st.Render(fit(t.label(v.idx), v.w))
	}
	lead := strings.Repeat(" ", gutter)
	// markers say there is more off screen: « columns to the left, » to the right;
	// bold chips so they stand out from the headings, with a space either side
	if t.colStart > 0 {
		lead = hintKeyStyle.Render("«") + strings.Repeat(" ", gutter-1)
	}
	line := lead + strings.Join(parts, strings.Repeat(" ", colGap))
	if n := len(vis); n > 0 {
		last := vis[n-1]
		if last.idx < len(t.cols)-1 || last.w < t.widths[last.idx] {
			if pad := t.innerWidth() - lipgloss.Width(line) - 1; pad >= 0 {
				line += strings.Repeat(" ", pad) + hintKeyStyle.Render("»")
			}
		}
	}
	return line
}

func (t *table) rowLine(i int, vis []vcol) string {
	r := t.rows[i]
	cell := func(v vcol) string {
		s := ""
		if v.idx < len(r.Cells) {
			s = r.Cells[v.idx]
		}
		return fit(s, v.w)
	}
	if i == t.cursor {
		parts := make([]string, len(vis))
		for j, v := range vis {
			parts[j] = cell(v)
		}
		line := strings.Repeat(" ", gutter) + strings.Join(parts, strings.Repeat(" ", colGap))
		return selectedStyle.Render(fit(line, t.innerWidth()))
	}
	parts := make([]string, len(vis))
	for j, v := range vis {
		c := cell(v)
		parts[j] = styleFor(c).Render(c)
	}
	return strings.Repeat(" ", gutter) + strings.Join(parts, strings.Repeat(" ", colGap))
}

// render draws the panel. empty is shown in the middle when there are no rows.
func (t *table) render(title, empty string) string {
	inner := t.innerWidth()
	bodyH := t.height - 2
	lines := make([]string, 0, bodyH)

	if len(t.rows) == 0 {
		// the message is centred as one block; its lines stay left-aligned so lists read well
		msg := strings.Split(empty, "\n")
		blockW := 0
		for _, l := range msg {
			blockW = max(blockW, lipgloss.Width(l))
		}
		pad := strings.Repeat(" ", max((inner-blockW)/2, 0))
		first := max((bodyH-len(msg))/2, 0)
		for i := 0; i < bodyH; i++ {
			l := ""
			if i >= first && i-first < len(msg) {
				l = statusStyle.Render(fit(pad+msg[i-first], inner))
			}
			lines = append(lines, l)
		}
		return panel(title, lines, t.width, t.height)
	}

	vis := t.visibleCols()
	lines = append(lines, t.headerLine(vis))
	end := min(t.top+t.rowsH(), len(t.rows))
	for i := t.top; i < end; i++ {
		lines = append(lines, t.rowLine(i, vis))
	}
	return panel(title, lines, t.width, t.height)
}

// sortRows sorts a copy of rows by column col. Numeric columns compare by
// value (non-numbers last); everything else compares case-insensitively.
func sortRows(rows []provider.Row, col int, desc, numeric bool) []provider.Row {
	out := make([]provider.Row, len(rows))
	copy(out, rows)
	if col < 0 {
		return out
	}
	cellOf := func(r provider.Row) string {
		if col < len(r.Cells) {
			return r.Cells[col]
		}
		return ""
	}
	less := func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }
	if numeric {
		less = func(a, b string) bool {
			x, errA := strconv.ParseFloat(a, 64)
			y, errB := strconv.ParseFloat(b, 64)
			switch {
			case errA != nil && errB != nil:
				return a < b
			case errA != nil:
				return false
			case errB != nil:
				return true
			}
			return x < y
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if desc {
			return less(cellOf(out[j]), cellOf(out[i]))
		}
		return less(cellOf(out[i]), cellOf(out[j]))
	})
	return out
}

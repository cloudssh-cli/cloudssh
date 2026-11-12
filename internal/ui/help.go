package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	helpGap       = 3  // blank columns between help columns
	helpMinColW   = 44 // narrowest a help column may get before we use fewer columns
	helpMaxKeyW   = 16 // widest key column inside a section
	helpMaxColumn = 3
)

type helpEntry struct{ key, desc string }

type helpSection struct {
	title   string
	entries []helpEntry
}

// helpModal lists every key binding, including the per-column sort keys of
// the screen it was opened from. Sections flow into as many columns as the
// terminal is wide; anything that still does not fit scrolls (↑↓ j k pgup
// pgdn g G home end) and long descriptions wrap instead of being cut off.
type helpModal struct {
	sections []helpSection
	offset   int
}

func helpSections(m *Model) []helpSection {
	views := []helpEntry{{"d  0", "Dashboard"}}
	var names []string
	for i, r := range m.resources {
		if i < 9 {
			names = append(names, fmt.Sprintf("%d %s", i+1, r.Label()))
		}
	}
	if len(names) > 0 {
		views = append(views, helpEntry{"1-" + fmt.Sprint(len(names)), strings.Join(names, " · ")})
	}
	if m.auth != nil && m.hasProfiles {
		views = append(views, helpEntry{"p", "Profiles: switch the sign-in profile (enter)"})
	}
	views = append(views, helpEntry{"?", "This help"}, helpEntry{"q  ctrl+c", "Quit"})

	secs := []helpSection{
		{"Table", []helpEntry{
			{"↑ ↓  j k", "Move selection"},
			{"pgup pgdn", "Page up / down (ctrl+b / ctrl+f)"},
			{"g  home", "Jump to the top"},
			{"G  end", "Jump to the bottom"},
			{"← →  h l", "Scroll columns"},
			{"enter", "Open row (drill down) / SSH into host"},
			{"S", "SSH into the host"},
			{"esc", "Clear search, then go back"},
		}},
		{"Data", []helpEntry{
			{"/  f  s", "Search rows (↑ ↓ recall earlier searches)"},
			{"F", "Update server-side filters"},
			{"r", "Refresh data"},
			{"R", "Reset filters, search, sort"},
			{"n  shift+→", "Load the next page"},
		}},
	}
	if s := m.cur(); s != nil {
		var keys []string
		for i, c := range s.res.Columns() {
			if k := s.sortKeys[i]; k != "" {
				mark := ""
				if i == s.tbl.sortCol {
					mark = "▲"
					if s.tbl.sortDesc {
						mark = "▼"
					}
				}
				keys = append(keys, k+"="+strings.ReplaceAll(c.Title, " ", "\u00a0")+mark) // no wrapping inside a name
			}
		}
		secs = append(secs, helpSection{"Sorting", []helpEntry{
			{"shift+key", strings.Join(keys, "   ") + "   (press again to reverse)"},
		}})
	}
	secs = append(secs,
		helpSection{"Views", views},
		helpSection{"Filters dialog (F)", []helpEntry{
			{"type", "Fill in the filters you want"},
			{"↑ ↓  tab", "Move between filters"},
			{"ctrl+x", "Clear all values"},
			{"enter  esc", "Apply / cancel"},
		}},
		helpSection{"SSH dialog (S)", []helpEntry{
			{"tab", "Switch private / public IP"},
			{"ctrl+t", "SSH key / EC2 Instance Connect key"},
			{"ctrl+o", "Direct / your running tunnel"},
			{"enter  esc", "Connect / cancel"},
		}},
	)
	return secs
}

func newHelp(m *Model) *helpModal { return &helpModal{sections: helpSections(m)} }

// wrap word-wraps plain text to w columns (hard-wrapping words that are longer).
func wrap(s string, w int) []string {
	if w < 1 {
		return []string{""}
	}
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}

// renderSection draws one section at width w: a heading and its entries with
// the key column sized to the longest key and descriptions wrapped.
func renderSection(sec helpSection, w int) []string {
	kw := 1
	for _, e := range sec.entries {
		kw = max(kw, lipgloss.Width(e.key))
	}
	kw = min(kw, min(helpMaxKeyW, w/3))
	descW := max(w-2-kw-2, 8)

	out := []string{headStyle.Render(fit(sec.title, w))}
	for _, e := range sec.entries {
		keys, desc := wrap(e.key, kw), wrap(e.desc, descW)
		for i := 0; i < max(len(keys), len(desc)); i++ {
			k, d := "", ""
			if i < len(keys) {
				k = keys[i]
			}
			if i < len(desc) {
				d = desc[i]
			}
			out = append(out, "  "+hintKeyStyle.Render(fit(k, kw))+"  "+hintDescStyle.Render(fit(d, descW)))
		}
	}
	return append(out, "")
}

// helpLayout flows the sections into columns that fit width and returns the
// finished lines (each exactly width wide).
func helpLayout(secs []helpSection, width int) []string {
	avail := width - 1 // one leading space
	cols := 1
	for c := helpMaxColumn; c >= 2; c-- {
		if (avail-(c-1)*helpGap)/c >= helpMinColW {
			cols = c
			break
		}
	}
	colW := (avail - (cols-1)*helpGap) / cols
	blocks := make([][]string, len(secs))
	for i, s := range secs {
		blocks[i] = renderSection(s, colW)
	}
	groups := bestSplit(blocks, cols)
	columns := make([][]string, cols)
	for c, g := range groups {
		for _, bi := range g {
			columns[c] = append(columns[c], blocks[bi]...)
		}
		// the last section of a column needs no separating blank line
		if n := len(columns[c]); n > 0 && columns[c][n-1] == "" {
			columns[c] = columns[c][:n-1]
		}
	}
	height := 0
	for _, c := range columns {
		height = max(height, len(c))
	}
	lines := make([]string, height)
	for r := 0; r < height; r++ {
		parts := make([]string, cols)
		for c := range columns {
			l := ""
			if r < len(columns[c]) {
				l = columns[c][r]
			}
			parts[c] = fit(l, colW)
		}
		lines[r] = fit(" "+strings.Join(parts, strings.Repeat(" ", helpGap)), width)
	}
	return lines
}

// bestSplit divides the blocks, keeping their order, into cols contiguous
// groups so the tallest column is as short as possible. It returns block
// indexes per column. Sections are few (<10), so trying every cut is cheap.
func bestSplit(blocks [][]string, cols int) [][]int {
	n := len(blocks)
	height := func(from, to int) int { // lines in blocks[from:to] minus the final separator
		h := 0
		for _, b := range blocks[from:to] {
			h += len(b)
		}
		return max(h-1, 0)
	}
	var best [][]int
	bestMax := 1 << 30
	var walk func(start, left int, cuts []int)
	walk = func(start, left int, cuts []int) {
		if left == 1 {
			all := append(append([]int(nil), cuts...), n)
			tallest, prev := 0, 0
			for _, c := range all {
				tallest = max(tallest, height(prev, c))
				prev = c
			}
			if tallest < bestMax {
				bestMax = tallest
				best = best[:0]
				prev = 0
				for _, c := range all {
					var g []int
					for i := prev; i < c; i++ {
						g = append(g, i)
					}
					best = append(best, g)
					prev = c
				}
			}
			return
		}
		for c := start + 1; c <= n-(left-1); c++ {
			walk(c, left-1, append(cuts, c))
		}
	}
	walk(0, cols, nil)
	for len(best) < cols {
		best = append(best, nil)
	}
	return best
}

func (hm *helpModal) key(_ *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		return true, nil
	case "up", "k":
		hm.offset = max(hm.offset-1, 0)
	case "down", "j":
		hm.offset++
	case "pgup", "ctrl+b":
		hm.offset = max(hm.offset-10, 0)
	case "pgdown", "ctrl+f":
		hm.offset += 10
	case "g", "home":
		hm.offset = 0
	case "G", "end":
		hm.offset = 1 << 30 // clamped to the last page when rendered
	}
	return false, nil
}

func (hm *helpModal) view(w, h int) string {
	inner, rows := max(w-2, 1), max(h-2, 1)
	lines := helpLayout(hm.sections, inner)
	hm.offset = min(hm.offset, max(len(lines)-rows, 0))
	end := min(hm.offset+rows, len(lines))
	body := append([]string(nil), lines[hm.offset:end]...)

	hint := "  (esc to close"
	if len(lines) > rows {
		hint += fmt.Sprintf(" · ↑↓ g G to scroll · %d/%d", min(hm.offset+rows, len(lines)), len(lines))
	}
	title := headSortStyle.Render("Help") + hintDescStyle.Render(hint+")")
	return panel(title, body, w, h)
}

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type filterRow struct {
	def int
	in  textinput.Model
}

// filtersModal edits a screen's server-side filters (the Python app's F screen).
type filtersModal struct {
	sid   int
	defs  []provider.FilterDef
	rows  []filterRow
	focus int
}

func newFiltersModal(s *screen) (*filtersModal, tea.Cmd) {
	defs := append([]provider.FilterDef(nil), s.res.Filters()...)
	index := func(key string) int {
		for i, d := range defs {
			if d.Key == key {
				return i
			}
		}
		// a drill can apply a filter the list doesn't advertise; keep it visible
		defs = append(defs, provider.FilterDef{Key: key, Name: key})
		return len(defs) - 1
	}
	fm := &filtersModal{sid: s.id}
	for _, f := range s.filters {
		fm.rows = append(fm.rows, filterRow{def: index(f.Key), in: newValueInput(f.Value)})
	}
	fm.defs = defs
	if len(fm.rows) == 0 && len(defs) > 0 {
		fm.rows = append(fm.rows, filterRow{def: 0, in: newValueInput("")})
	}
	return fm, fm.sync()
}

func newValueInput(v string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 30
	in.SetValue(v)
	in.CursorEnd()
	return in
}

// sync focuses only the active row's input.
func (fm *filtersModal) sync() tea.Cmd {
	var cmd tea.Cmd
	for i := range fm.rows {
		if i == fm.focus {
			cmd = fm.rows[i].in.Focus()
		} else {
			fm.rows[i].in.Blur()
		}
	}
	return cmd
}

func (fm *filtersModal) move(d int) {
	if n := len(fm.rows); n > 0 {
		fm.focus = (fm.focus + d + n) % n
	}
}

func (fm *filtersModal) key(m *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return true, nil
	case "enter":
		var fs []provider.Filter
		for _, r := range fm.rows {
			if v := strings.TrimSpace(r.in.Value()); v != "" {
				fs = append(fs, provider.Filter{Key: fm.defs[r.def].Key, Value: v})
			}
		}
		s := m.find(fm.sid)
		if s == nil {
			return true, nil
		}
		s.filters = fs
		s.search = ""
		m.filter.SetValue("")
		return true, tea.Batch(s.reload(), m.setStatus("filters updated, refreshing..."))
	case "up", "shift+tab":
		fm.move(-1)
		return false, fm.sync()
	case "down", "tab":
		fm.move(1)
		return false, fm.sync()
	case "ctrl+n", "ctrl+p":
		if len(fm.rows) == 0 || len(fm.defs) == 0 {
			return false, nil
		}
		d := 1
		if msg.String() == "ctrl+p" {
			d = -1
		}
		r := &fm.rows[fm.focus]
		r.def = (r.def + d + len(fm.defs)) % len(fm.defs)
		r.in.SetValue("") // a new filter kind starts empty, like the Python app
		return false, nil
	case "ctrl+a":
		if len(fm.defs) == 0 {
			return false, nil
		}
		used := map[int]bool{}
		for _, r := range fm.rows {
			used[r.def] = true
		}
		next := 0
		for i := range fm.defs {
			if !used[i] {
				next = i
				break
			}
		}
		fm.rows = append(fm.rows, filterRow{def: next, in: newValueInput("")})
		fm.focus = len(fm.rows) - 1
		return false, fm.sync()
	case "ctrl+d":
		if len(fm.rows) == 0 {
			return false, nil
		}
		fm.rows = append(fm.rows[:fm.focus], fm.rows[fm.focus+1:]...)
		fm.focus = min(fm.focus, max(len(fm.rows)-1, 0))
		return false, fm.sync()
	}
	if len(fm.rows) == 0 {
		return false, nil
	}
	var cmd tea.Cmd
	fm.rows[fm.focus].in, cmd = fm.rows[fm.focus].in.Update(msg)
	return false, cmd
}

func (fm *filtersModal) view(w, h int) string {
	label := 0
	for _, d := range fm.defs {
		label = max(label, lipgloss.Width(d.Name))
	}
	var body []string
	body = append(body, "")
	if len(fm.rows) == 0 {
		body = append(body, hintDescStyle.Render("  No filters. Press ctrl+a to add one."))
	}
	for i, r := range fm.rows {
		mark := "  "
		if i == fm.focus {
			mark = hintKeyStyle.Render("▶ ")
		}
		name := fit(fm.defs[r.def].Name, label)
		if i == fm.focus {
			name = headSortStyle.Render(name)
		} else {
			name = infoValStyle.Render(name)
		}
		body = append(body, mark+name+"  "+r.in.View())
	}
	body = append(body, "",
		hintDescStyle.Render("  enter apply · esc cancel · ↑↓/tab rows · ctrl+n/ctrl+p filter kind"),
		hintDescStyle.Render("  ctrl+a add filter · ctrl+d delete filter · empty values are ignored"))
	bw := 0
	for _, l := range body {
		bw = max(bw, lipgloss.Width(l))
	}
	bw = min(bw+4, w)
	title := lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render("Update data filters")
	return placeCenter(panel(title, body, bw, len(body)+2), w, h)
}

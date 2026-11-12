package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// filtersModal edits a screen's server-side filters. Every filter the
// resource supports gets its own row, so there is nothing to cycle through:
// move to a row, type a value, enter applies. Empty rows are ignored.
type filtersModal struct {
	sid    int
	defs   []provider.FilterDef
	inputs []textinput.Model // one per def
	focus  int
	offset int // first visible row when the list is taller than the dialog
}

func newFiltersModal(s *screen) (*filtersModal, tea.Cmd) {
	defs := append([]provider.FilterDef(nil), s.res.Filters()...)
	active := map[string]string{}
	for _, f := range s.filters {
		active[f.Key] = f.Value
		known := false
		for _, d := range defs {
			known = known || d.Key == f.Key
		}
		if !known {
			// a drill can apply a filter the list doesn't advertise; keep it editable
			defs = append(defs, provider.FilterDef{Key: f.Key, Name: f.Key})
		}
	}
	fm := &filtersModal{sid: s.id, defs: defs}
	for _, d := range defs {
		in := textinput.New()
		in.Prompt = ""
		in.Width = 32
		in.Placeholder = "(any)"
		in.SetValue(active[d.Key])
		in.CursorEnd()
		fm.inputs = append(fm.inputs, in)
	}
	// start on the first filter that is already set, else the first row
	for i, in := range fm.inputs {
		if in.Value() != "" {
			fm.focus = i
			break
		}
	}
	return fm, fm.sync()
}

// sync focuses only the active row's input.
func (fm *filtersModal) sync() tea.Cmd {
	var cmd tea.Cmd
	for i := range fm.inputs {
		if i == fm.focus {
			cmd = fm.inputs[i].Focus()
		} else {
			fm.inputs[i].Blur()
		}
	}
	return cmd
}

func (fm *filtersModal) move(d int) {
	if n := len(fm.inputs); n > 0 {
		fm.focus = (fm.focus + d + n) % n
	}
}

func (fm *filtersModal) key(m *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return true, nil
	case "enter":
		var fs []provider.Filter
		for i, in := range fm.inputs {
			if v := strings.TrimSpace(in.Value()); v != "" {
				fs = append(fs, provider.Filter{Key: fm.defs[i].Key, Value: v})
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
	}
	if len(fm.inputs) == 0 {
		return false, nil
	}
	var cmd tea.Cmd
	fm.inputs[fm.focus], cmd = fm.inputs[fm.focus].Update(msg)
	return false, cmd
}

func (fm *filtersModal) view(w, h int) string {
	label := 0
	for _, d := range fm.defs {
		label = max(label, lipgloss.Width(d.Name))
	}
	footer := []string{
		"",
		hintDescStyle.Render("  enter apply · esc cancel · ↑↓ or tab to move"),
		hintDescStyle.Render("  space and  ·  a|b or  ·  !a not  ·  (a|b) group  ·  \\ escapes  ·  R in the table resets all"),
	}
	// the dialog needs: border(2) + blank(1) + rows + footer
	room := max(h-2-1-len(footer), 1)
	fm.offset = min(max(fm.offset, fm.focus-room+1), fm.focus)
	fm.offset = min(fm.offset, max(len(fm.defs)-room, 0))

	body := []string{""}
	if len(fm.defs) == 0 {
		body = append(body, hintDescStyle.Render("  This resource has no filters."))
	}
	end := min(fm.offset+room, len(fm.defs))
	for i := fm.offset; i < end; i++ {
		mark, name := "  ", infoValStyle.Render(fit(fm.defs[i].Name, label))
		if i == fm.focus {
			mark, name = hintKeyStyle.Render("▶ "), headSortStyle.Render(fit(fm.defs[i].Name, label))
		}
		body = append(body, mark+name+"  "+fm.inputs[i].View())
	}
	body = append(body, footer...)

	bw := 0
	for _, l := range body {
		bw = max(bw, lipgloss.Width(l))
	}
	bw = min(bw+4, w)
	title := lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render("Update data filters")
	if len(fm.defs) > room {
		title += hintDescStyle.Render("  (scroll with ↑↓)")
	}
	return placeCenter(panel(title, body, bw, len(body)+2), w, h)
}

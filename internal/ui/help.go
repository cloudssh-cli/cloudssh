package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// helpModal lists every key binding, including the per-column sort keys of
// the screen it was opened from.
type helpModal struct {
	lines  []string
	offset int
}

func helpEntry(key, desc string) string {
	return "  " + hintKeyStyle.Render(fit(key, 18)) + hintDescStyle.Render(desc)
}

func helpHeading(s string) string { return " " + headStyle.Render(s) }

func newHelp(m *Model) *helpModal {
	var l []string
	add := func(ls ...string) { l = append(l, ls...) }

	if s := m.cur(); s != nil {
		add(helpHeading("Sorting (shift+key, press again to reverse)"))
		for i, c := range s.res.Columns() {
			if k := s.sortKeys[i]; k != "" {
				mark := ""
				if i == s.tbl.sortCol {
					mark = "  (current)"
				}
				add(helpEntry(k, c.Title+mark))
			}
		}
		add("")
	}

	add(helpHeading("Views"),
		helpEntry("d / 0", "Dashboard"))
	for i, r := range m.resources {
		if i < 9 {
			add(helpEntry(fmt.Sprint(i+1), r.Label()))
		}
	}
	add(helpEntry("?", "This help"), helpEntry("q / ctrl+c", "Quit"), "")

	add(helpHeading("Table"),
		helpEntry("↑ ↓ / j k", "Move selection"),
		helpEntry("pgup pgdn / ctrl+b ctrl+f", "Page up / down"),
		helpEntry("g / home", "Jump to top"),
		helpEntry("G / end", "Jump to bottom"),
		helpEntry("← → / h l", "Scroll columns when the table is wider than the screen"),
		helpEntry("enter", "Open the selected row (drill down) or SSH into a host"),
		helpEntry("S", "SSH into the selected host"),
		helpEntry("esc", "Clear search, then go back one level"),
		"")

	add(helpHeading("Data"),
		helpEntry("/ f s", "Search loaded rows (esc clears, ↑ ↓ recall previous searches)"),
		helpEntry("F", "Update server-side filters"),
		helpEntry("r", "Refresh data"),
		helpEntry("R", "Reset filters, search and sort to defaults"),
		helpEntry("n / shift+→", "Load next page"),
		"")

	add(helpHeading("Filters dialog (F)"),
		helpEntry("type", "Every available filter is listed; fill in the ones you want"),
		helpEntry("↑ ↓ / tab", "Move between filters"),
		helpEntry("ctrl+x", "Clear all values"),
		helpEntry("enter / esc", "Apply / cancel"),
		"",
		helpHeading("SSH dialog"),
		helpEntry("tab", "Switch private / public IP"),
		helpEntry("ctrl+t", "Switch between your SSH key and EC2 Instance Connect (when available)"),
		helpEntry("ctrl+o", "Connect through a tunnel you already run (shown only when one matches the host)"),
		helpEntry("enter / esc", "Connect / cancel"))
	return &helpModal{lines: l}
}

func (hm *helpModal) key(_ *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		return true, nil
	case "up", "k":
		hm.offset = max(hm.offset-1, 0)
	case "down", "j":
		hm.offset++
	case "pgup":
		hm.offset = max(hm.offset-10, 0)
	case "pgdown":
		hm.offset += 10
	}
	return false, nil
}

func (hm *helpModal) view(w, h int) string {
	inner := max(h-2, 1)
	hm.offset = min(hm.offset, max(len(hm.lines)-inner, 0))
	end := min(hm.offset+inner, len(hm.lines))
	body := append([]string(nil), hm.lines[hm.offset:end]...)
	title := headSortStyle.Render("Help") + hintDescStyle.Render("  (esc to close"+scrollHint(len(hm.lines) > inner)+")")
	return panel(title, body, w, h)
}

func scrollHint(scrollable bool) string {
	if scrollable {
		return ", ↑↓ to scroll"
	}
	return ""
}

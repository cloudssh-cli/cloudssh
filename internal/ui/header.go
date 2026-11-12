package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type hint struct{ key, desc string }
type info struct{ label, value string }

// renderHeader draws the k9s-style top block: context info on the left and
// key hints (filled column-major, right-aligned) on the right. It always
// returns exactly headerHeight lines, each at most width wide.
func renderHeader(infos []info, hints []hint, width int) string {
	lines := make([]string, headerHeight)

	infoW := 0
	for i, in := range infos[:min(len(infos), headerHeight)] {
		l := infoKeyStyle.Render(in.label+":") + " " + infoValStyle.Render(in.value)
		lines[i] = l
		infoW = max(infoW, lipgloss.Width(l))
	}

	// hints, column-major, headerHeight per column
	var cols [][]hint
	for i := 0; i < len(hints); i += headerHeight {
		cols = append(cols, hints[i:min(i+headerHeight, len(hints))])
	}
	hintLines := make([]string, headerHeight)
	hintW := 0
	for ci, col := range cols {
		// stop adding columns once they would crowd the info block
		kw0, dw0 := 0, 0
		for _, h := range col {
			kw0 = max(kw0, lipgloss.Width("<"+h.key+">"))
			dw0 = max(dw0, lipgloss.Width(h.desc))
		}
		add := kw0 + 1 + dw0
		if ci > 0 {
			add += 3
		}
		if ci > 0 && infoW+2+hintW+add >= width {
			break
		}
		kw, dw := 0, 0
		for _, h := range col {
			kw = max(kw, lipgloss.Width("<"+h.key+">"))
			dw = max(dw, lipgloss.Width(h.desc))
		}
		for r := 0; r < headerHeight; r++ {
			cell := strings.Repeat(" ", kw+1+dw)
			if r < len(col) {
				k := "<" + col[r].key + ">"
				cell = hintKeyStyle.Render(fit(k, kw)) + " " + hintDescStyle.Render(fit(col[r].desc, dw))
			}
			if ci > 0 {
				cell = "   " + cell
			}
			hintLines[r] += cell
		}
		hintW += kw + 1 + dw
		if ci > 0 {
			hintW += 3
		}
	}

	pad := width - infoW - hintW - 1 // trailing margin
	for i := range lines {
		l := lines[i]
		if pad >= 2 {
			l = fit(l, infoW) + strings.Repeat(" ", pad) + hintLines[i]
		} else {
			l = fit(l, infoW) + "  " + hintLines[i]
		}
		lines[i] = ansi.Truncate(l, width, "")
	}
	return strings.Join(lines, "\n")
}

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette ported from the Python app's default theme.
var (
	colFg        = lipgloss.Color("#6FD0FF")
	colPrimary   = lipgloss.Color("#FF00FF")
	colSecondary = lipgloss.Color("#00FFFF")
	colAccent    = lipgloss.Color("#FFFFFF")
	colMuted     = lipgloss.Color("#808080")
	colOK        = lipgloss.Color("#A3BE8C")
	colWarn      = lipgloss.Color("#EBCB8B")
	colErr       = lipgloss.Color("#BF616A")
	colBlack     = lipgloss.Color("#000000")
)

var (
	borderStyle   = lipgloss.NewStyle().Foreground(colFg)
	hintKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	hintDescStyle = lipgloss.NewStyle().Foreground(colMuted)
	infoKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	infoValStyle  = lipgloss.NewStyle().Foreground(colFg)
	headStyle     = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	headSortStyle = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colSecondary)
	cellStyle     = lipgloss.NewStyle().Foreground(colFg)
	crumbStyle    = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colFg)
	crumbActive   = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colSecondary)
	statusStyle   = lipgloss.NewStyle().Foreground(colSecondary)
	errorStyle    = lipgloss.NewStyle().Foreground(colErr)
)

// stateStyles colours well-known resource states, k9s style.
var stateStyles = map[string]lipgloss.Style{
	"running":       lipgloss.NewStyle().Foreground(colOK),
	"available":     lipgloss.NewStyle().Foreground(colOK),
	"active":        lipgloss.NewStyle().Foreground(colOK),
	"pending":       lipgloss.NewStyle().Foreground(colWarn),
	"stopping":      lipgloss.NewStyle().Foreground(colWarn),
	"stopped":       lipgloss.NewStyle().Foreground(colErr),
	"terminated":    lipgloss.NewStyle().Foreground(colErr),
	"shutting-down": lipgloss.NewStyle().Foreground(colWarn),
}

func styleFor(cell string) lipgloss.Style {
	if s, ok := stateStyles[strings.ToLower(strings.TrimSpace(cell))]; ok {
		return s
	}
	return cellStyle
}

// fit truncates or right-pads s so its visible width is exactly w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// center centres s in w columns (visible width), truncating if too wide.
func center(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return fit(s, w)
	}
	left := (w - sw) / 2
	return fit(strings.Repeat(" ", left)+s, w)
}

// panel draws a square-cornered box with title centred in the top border.
// Body lines must already be inner-width; they are fitted regardless.
func panel(title string, body []string, width, height int) string {
	inner := width - 2
	if inner < 1 || height < 2 {
		return ""
	}
	b := func(s string) string { return borderStyle.Render(s) }

	top := b("┌" + strings.Repeat("─", inner) + "┐")
	if title != "" {
		t := " " + title + " "
		if lipgloss.Width(t) > inner {
			t = ansi.Truncate(t, inner, "…")
		}
		left := (inner - lipgloss.Width(t)) / 2
		right := inner - left - lipgloss.Width(t)
		top = b("┌"+strings.Repeat("─", left)) + t + b(strings.Repeat("─", right)+"┐")
	}

	lines := make([]string, 0, height)
	lines = append(lines, top)
	for i := 0; i < height-2; i++ {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		lines = append(lines, b("│")+fit(l, inner)+b("│"))
	}
	lines = append(lines, b("└"+strings.Repeat("─", inner)+"┘"))
	return strings.Join(lines, "\n")
}

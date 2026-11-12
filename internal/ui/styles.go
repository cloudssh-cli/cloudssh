package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The palette and the styles built from it. They are filled in by applyTheme
// (theme.go), at start and whenever the theme changes.
var (
	colFg, colPrimary, colSecondary, colAccent, colMuted lipgloss.Color
	colOK, colWarn, colErr, colBlack                     lipgloss.Color
	colHelpKey, colHelpHead                              lipgloss.Color
	fadeTo                                               string

	borderStyle, hintKeyStyle, hintDescStyle, infoKeyStyle, infoValStyle lipgloss.Style
	headStyle, headSortStyle, selectedStyle, cellStyle                   lipgloss.Style
	crumbStyle, crumbActive, statusStyle, errorStyle, warnStyle          lipgloss.Style
	helpKeyStyle, helpHeadStyle                                          lipgloss.Style

	// stateStyles colours well-known resource states, k9s style.
	stateStyles map[string]lipgloss.Style
)

func applyTheme(t Theme) {
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	colFg, colPrimary, colSecondary, colAccent, colMuted = c(t.Border), c(t.Title), c(t.Highlight), c(t.Strong), c(t.Muted)
	colOK, colWarn, colErr, colBlack = c(t.OK), c(t.Warn), c(t.Err), c(t.OnFill)
	colHelpKey, colHelpHead, fadeTo = c(t.HelpKey), c(t.HelpHead), t.Fade

	fg := lipgloss.NewStyle().Foreground
	borderStyle = fg(colFg)
	hintKeyStyle = fg(colPrimary).Bold(true)
	hintDescStyle = fg(colMuted)
	infoKeyStyle = fg(colPrimary).Bold(true)
	infoValStyle = fg(colFg)
	headStyle = fg(colAccent).Bold(true)
	headSortStyle = fg(colPrimary).Bold(true)
	selectedStyle = fg(colBlack).Background(colSecondary).Bold(true)
	cellStyle = fg(colFg)
	crumbStyle = fg(colBlack).Background(colFg).Bold(true)
	crumbActive = fg(colBlack).Background(colSecondary).Bold(true)
	statusStyle = fg(colSecondary)
	errorStyle = fg(colErr)
	warnStyle = fg(colWarn)
	helpKeyStyle = fg(colHelpKey).Bold(true)
	helpHeadStyle = fg(colHelpHead).Bold(true)

	stateStyles = map[string]lipgloss.Style{
		"running":       fg(colOK),
		"available":     fg(colOK),
		"active":        fg(colOK),
		"pending":       fg(colWarn),
		"stopping":      fg(colWarn),
		"stopped":       fg(colErr),
		"terminated":    fg(colErr),
		"shutting-down": fg(colWarn),
	}
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

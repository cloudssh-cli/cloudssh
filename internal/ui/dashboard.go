package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// logo is "CLOUD SSH" in figlet's slant font.
const logo = `   ________    ____  __  ______     __________ __  __
  / ____/ /   / __ \/ / / / __ \   / ___/ ___// / / /
 / /   / /   / / / / / / / / / /   \__ \\__ \/ /_/ /
/ /___/ /___/ /_/ / /_/ / /_/ /   ___/ /__/ / __  /
\____/_____/\____/\____/_____/   /____/____/_/ /_/`

func logoLines(style lipgloss.Style, w int) []string {
	raw := strings.Split(logo, "\n")
	pad := 0
	for _, l := range raw {
		pad = max(pad, lipgloss.Width(l))
	}
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = center(style.Render(fit(l, pad)), w)
	}
	return out
}

// dashboardLines renders the logo and the shortcut table (shortcut, screen,
// description) inside a w x h area.
func dashboardLines(res []provider.Resource, profiles bool, w, h int) []string {
	type entry struct{ key, name, desc string }
	entries := []entry{{"d, 0", "Dashboard", "This dashboard"}}
	for i, r := range res {
		if i < 9 {
			entries = append(entries, entry{fmt.Sprint(i + 1), r.Label(), "List and open " + strings.ToLower(r.Label())})
		}
	}
	if profiles {
		entries = append(entries, entry{"p", "Profiles", "Switch the sign-in profile"})
	}
	entries = append(entries, entry{"?", "Help", "All key bindings"}, entry{"q", "Quit", "Quit application"})

	kw, nw, dw := len("Shortcut(s)"), len("Screen"), len("Description")
	for _, e := range entries {
		kw, nw, dw = max(kw, len(e.key)), max(nw, len(e.name)), max(dw, len(e.desc))
	}
	line := func(a, b, c string, ka, kb, kc lipgloss.Style) string {
		return center(ka.Render(fit(a, kw))+"   "+kb.Render(fit(b, nw))+"   "+kc.Render(fit(c, dw)), w)
	}

	lines := logoLines(infoValStyle, w)
	lines = append(lines, "")
	lines = append(lines, line("Shortcut(s)", "Screen", "Description", headStyle, headStyle, headStyle))
	spaced := len(lines)+len(entries)*2 <= h
	for _, e := range entries {
		if spaced {
			lines = append(lines, "")
		}
		lines = append(lines, line(e.key, e.name, e.desc, hintKeyStyle, statusStyle, infoValStyle))
	}
	if len(lines) > h {
		lines = lines[len(lines)-h:] // small terminal: keep the shortcuts, drop the logo
	}
	top := (h - len(lines)) / 2
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	return append(out, lines...)
}

// ---- splash ----

const splashFadeSteps = 10

// splashView draws the logo centred on the whole screen, fading out as step
// goes from 0 to splashFadeSteps.
func splashView(w, h, step int) string {
	st := lipgloss.NewStyle().Bold(true).Foreground(lerpColor("#6FD0FF", "#0B0B0B", float64(step)/splashFadeSteps))
	lines := logoLines(st, w)
	top := max((h-len(lines))/2, 0)
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for len(out) < h {
		out = append(out, "")
	}
	return strings.Join(out[:h], "\n")
}

func lerpColor(from, to string, t float64) lipgloss.Color {
	var fr, fg, fb, tr, tg, tb int
	_, _ = fmt.Sscanf(from, "#%02x%02x%02x", &fr, &fg, &fb)
	_, _ = fmt.Sscanf(to, "#%02x%02x%02x", &tr, &tg, &tb)
	mix := func(a, b int) int { return a + int(float64(b-a)*t) }
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", mix(fr, tr), mix(fg, tg), mix(fb, tb)))
}

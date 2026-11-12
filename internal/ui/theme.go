package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a complete colour scheme. Every colour in the UI comes from one of
// these roles, so adding a theme means adding one value to themes (later also
// loading one from a file) and nothing else.
type Theme struct {
	Border    string // panel borders and ordinary cell text
	Title     string // panel titles, sort marker, hint keys in the header
	Highlight string // selection background, status line, search chip
	Strong    string // column headings and emphasised text
	Muted     string // secondary text
	OK, Warn  string // healthy / in-between states, warnings
	Err       string // failed states and errors
	OnFill    string // text drawn on a filled background (selection, breadcrumbs)
	HelpKey   string // key names on the help page (kept apart from the header's)
	HelpHead  string // section headings on the help page
	Fade      string // what the splash logo fades to: the terminal background
}

var themes = map[string]Theme{
	"dark": {
		Border: "#6FD0FF", Title: "#FF00FF", Highlight: "#00FFFF", Strong: "#FFFFFF", Muted: "#808080",
		OK: "#A3BE8C", Warn: "#EBCB8B", Err: "#BF616A", OnFill: "#000000",
		HelpKey: "#EBCB8B", HelpHead: "#88C0D0", Fade: "#0B0B0B",
	},
	"light": {
		Border: "#1F6FB2", Title: "#A10087", Highlight: "#007C89", Strong: "#1A1A1A", Muted: "#6B6B6B",
		OK: "#2E7D32", Warn: "#9A6700", Err: "#B3261E", OnFill: "#FFFFFF",
		HelpKey: "#8A5A00", HelpHead: "#0B5C80", Fade: "#F5F5F5",
	},
}

// Theme names the user may choose besides the concrete ones.
const (
	ThemeAuto    = "auto" // follow the terminal's background
	ThemeDefault = "dark" // used when auto cannot tell
)

// ThemeNames lists the choices for --theme / theme:, "auto" first.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for n := range themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return append([]string{ThemeAuto}, names...)
}

// ResolveTheme turns a user choice into a concrete theme name. "" and "auto"
// follow the system: systemDark reports whether the terminal background is dark
// (nil, or not knowing, gives the dark default).
func ResolveTheme(choice string, systemDark func() bool) (string, error) {
	choice = strings.ToLower(strings.TrimSpace(choice))
	switch choice {
	case "", ThemeAuto:
		if systemDark != nil && !systemDark() {
			return "light", nil
		}
		return ThemeDefault, nil
	}
	if _, ok := themes[choice]; !ok {
		return "", fmt.Errorf("unknown theme %q (choose one of: %s)", choice, strings.Join(ThemeNames(), ", "))
	}
	return choice, nil
}

// SystemDark reports whether the terminal has a dark background. When it cannot
// ask (no terminal, no answer) it says dark.
func SystemDark() bool { return lipgloss.HasDarkBackground() }

// SetTheme applies a theme by name ("auto" is resolved first). It is meant to
// run once at start, before the UI draws.
func SetTheme(choice string) error {
	name, err := ResolveTheme(choice, SystemDark)
	if err != nil {
		return err
	}
	applyTheme(themes[name])
	return nil
}

func init() { applyTheme(themes[ThemeDefault]) }

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
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
		err := fmt.Errorf("unknown theme %q (choose one of: %s)", choice, strings.Join(ThemeNames(), ", "))
		if bad, ok := themeProblems[choice]; ok {
			err = fmt.Errorf("theme %q could not be loaded: %w", choice, bad)
		}
		return "", err
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

// ---- themes from files ----

// themeProblems remembers files that failed to load, by theme name, so choosing
// one reports why instead of just "unknown".
var themeProblems = map[string]error{}

var (
	hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	themeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// themeFile is the YAML a user writes in <config dir>/themes/<name>.yaml:
//
//	base: dark            # optional, dark (default) or light: unlisted colours come from it
//	colors:
//	  border: "#6FD0FF"   # any of the roles below; #rrggbb only
//	  title: "#FF00FF"
type themeFile struct {
	Base   string            `yaml:"base"`
	Colors map[string]string `yaml:"colors"`
}

// roles maps a colour name in a theme file to its field.
func (t *Theme) roles() map[string]*string {
	return map[string]*string{
		"border": &t.Border, "title": &t.Title, "highlight": &t.Highlight, "strong": &t.Strong,
		"muted": &t.Muted, "ok": &t.OK, "warn": &t.Warn, "err": &t.Err, "on_fill": &t.OnFill,
		"help_key": &t.HelpKey, "help_head": &t.HelpHead, "fade": &t.Fade,
	}
}

// ParseTheme reads one theme file's content. Colours it does not set come from
// its base theme.
func ParseTheme(data []byte) (Theme, error) {
	var f themeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Theme{}, err
	}
	base := strings.ToLower(strings.TrimSpace(f.Base))
	if base == "" {
		base = ThemeDefault
	}
	t, ok := themes[base]
	if !ok {
		return Theme{}, fmt.Errorf("base %q is not a theme (use dark or light)", f.Base)
	}
	roles := t.roles()
	for name, val := range f.Colors {
		dst, ok := roles[strings.ToLower(name)]
		if !ok {
			known := make([]string, 0, len(roles))
			for k := range roles {
				known = append(known, k)
			}
			sort.Strings(known)
			return Theme{}, fmt.Errorf("unknown colour %q (known: %s)", name, strings.Join(known, ", "))
		}
		if !hexColour.MatchString(val) {
			return Theme{}, fmt.Errorf("colour %s: %q is not a #rrggbb value", name, val)
		}
		*dst = val
	}
	return t, nil
}

// LoadThemes registers every *.yaml / *.yml in dir as a theme named after the
// file (dark.yaml replaces the built-in dark). A missing dir is fine. Files
// that fail to load are skipped, and why is reported if that theme is chosen.
// It returns the names it added.
func LoadThemes(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	sort.Strings(files)
	var added []string
	for _, path := range files {
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
		if !themeName.MatchString(name) || name == ThemeAuto {
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // the user's own theme directory
		if err == nil {
			var t Theme
			if t, err = ParseTheme(data); err == nil {
				themes[name] = t
				delete(themeProblems, name)
				added = append(added, name)
				continue
			}
		}
		themeProblems[name] = fmt.Errorf("%s: %w", path, err)
	}
	return added
}

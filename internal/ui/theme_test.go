package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

func withTheme(t *testing.T, name string) {
	t.Helper()
	if err := SetTheme(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { applyTheme(themes[ThemeDefault]) })
}

func TestResolveThemeFollowsSystemAndDefaultsToDark(t *testing.T) {
	dark, light := func() bool { return true }, func() bool { return false }
	for _, c := range []struct {
		choice string
		sys    func() bool
		want   string
	}{
		{"", dark, "dark"}, {"auto", light, "light"}, {"AUTO", dark, "dark"}, {"auto", nil, "dark"}, // unknown system: dark
		{"light", dark, "light"}, {"dark", light, "dark"}, // an explicit choice ignores the system
	} {
		got, err := ResolveTheme(c.choice, c.sys)
		if err != nil || got != c.want {
			t.Errorf("ResolveTheme(%q) = %q, %v; want %q", c.choice, got, err, c.want)
		}
	}
	if _, err := ResolveTheme("neon", dark); err == nil || !strings.Contains(err.Error(), "auto, dark, light") {
		t.Errorf("an unknown theme should list the choices, got %v", err)
	}
}

func TestThemesAreCompleteAndDistinct(t *testing.T) {
	for name, th := range themes {
		for field, v := range map[string]string{
			"Border": th.Border, "Title": th.Title, "Highlight": th.Highlight, "Strong": th.Strong, "Muted": th.Muted,
			"OK": th.OK, "Warn": th.Warn, "Err": th.Err, "OnFill": th.OnFill, "HelpKey": th.HelpKey, "HelpHead": th.HelpHead, "Fade": th.Fade,
		} {
			if len(v) != 7 || v[0] != '#' {
				t.Errorf("theme %s: %s = %q is not a #rrggbb colour", name, field, v)
			}
		}
		// the help page must not reuse the header's colours
		if th.HelpKey == th.Title || th.HelpHead == th.Title {
			t.Errorf("theme %s: help colours equal the header's", name)
		}
	}
	if themes["dark"] == themes["light"] {
		t.Fatal("light and dark are the same")
	}
}

func TestSetThemeChangesTheColoursInUse(t *testing.T) {
	before := colFg
	withTheme(t, "light")
	if colFg == before || colFg != lipgloss.Color(themes["light"].Border) {
		t.Fatalf("light theme not applied: %v", colFg)
	}
	if stateStyles["running"].GetForeground() != lipgloss.Color(themes["light"].OK) {
		t.Error("state colours must follow the theme")
	}
	if err := SetTheme("nope"); err == nil {
		t.Error("an unknown theme must be an error")
	}
	if colFg != lipgloss.Color(themes["light"].Border) {
		t.Error("a failed SetTheme must leave the current theme alone")
	}
}

func TestHelpUsesItsOwnColoursNotTheHeaders(t *testing.T) {
	if helpKeyStyle.GetForeground() == hintKeyStyle.GetForeground() || helpHeadStyle.GetForeground() == hintKeyStyle.GetForeground() {
		t.Fatal("help keys/headings must be coloured differently from the header's keys")
	}
}

func TestEveryThemeRendersAScreenThatFits(t *testing.T) {
	for name := range themes {
		withTheme(t, name)
		m := loaded(t, threeRows())
		m = sized(m, 100, 24)
		for i, l := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(l) > 100 {
				t.Fatalf("%s: line %d is %d wide", name, i, lipgloss.Width(l))
			}
		}
	}
}

func TestBracketsGoBackAndForwardInAViewAndBracesThroughViews(t *testing.T) {
	_, parent, _ := twoLevel()
	other := fakeResource{key: "other", pages: map[string]provider.Page{"": {Rows: rows("o1")}}}
	m := newModelWith(testCfg(), parent, other)
	m = settle(m, m.Init())
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	if len(m.stacks[1]) != 2 {
		t.Fatal("drill expected")
	}
	m, _ = send(m, key("["))
	if len(m.stacks[1]) != 1 || len(m.forward[1]) != 1 {
		t.Fatalf("[ should go back and remember: stack=%d forward=%d", len(m.stacks[1]), len(m.forward[1]))
	}
	if !strings.Contains(m.View(), "Go forward") {
		t.Error("the header should offer ] once there is somewhere to go forward to")
	}
	m, _ = send(m, key("["))
	if !strings.Contains(m.status, "nothing to go back") {
		t.Errorf("status: %q", m.status)
	}
	m, _ = send(m, key("]"))
	if len(m.stacks[1]) != 2 || len(m.forward[1]) != 0 || m.cur().res.Key() != "child" {
		t.Fatalf("] should bring the drill back: %d %d %s", len(m.stacks[1]), len(m.forward[1]), m.cur().res.Key())
	}
	m, _ = send(m, key("]"))
	if !strings.Contains(m.status, "nothing to go forward") {
		t.Errorf("status: %q", m.status)
	}
	// a new drill forgets the old forward path
	m, _ = send(m, key("esc"))
	if len(m.forward[1]) != 1 {
		t.Fatal("esc is [")
	}
	m, cmd = send(m, key("enter"))
	m = settle(m, cmd)
	if len(m.forward[1]) != 0 {
		t.Fatal("a fresh drill must clear forward")
	}

	// { and } go back and forward through the views visited, like a browser
	m, _ = send(m, key("}"))
	if m.mode != 1 || !strings.Contains(m.status, "no later view") {
		t.Fatalf("nothing is ahead yet: mode=%d %q", m.mode, m.status)
	}
	m, _ = send(m, key("2"))
	m, _ = send(m, key("d"))
	m, _ = send(m, key("{"))
	if m.mode != 2 {
		t.Fatalf("{ should return to view 2, mode=%d", m.mode)
	}
	m, _ = send(m, key("{"))
	if m.mode != 1 || len(m.stacks[1]) != 2 {
		t.Fatalf("{ again returns to view 1, still on its drill: mode=%d depth=%d", m.mode, len(m.stacks[1]))
	}
	m, _ = send(m, key("{"))
	if m.mode != 1 || !strings.Contains(m.status, "no earlier view") {
		t.Fatalf("no wrap-around: mode=%d %q", m.mode, m.status)
	}
	m, _ = send(m, key("}"))
	m, _ = send(m, key("}"))
	if m.mode != 0 {
		t.Fatalf("} twice reaches the dashboard again, mode=%d", m.mode)
	}
	m, _ = send(m, key("{"))
	m, _ = send(m, key("3")) // out of range: ignored, keeps history
	m, _ = send(m, key("1")) // a new move drops the forward part
	m, _ = send(m, key("}"))
	if m.mode != 1 || !strings.Contains(m.status, "no later view") {
		t.Fatalf("a new move clears forward: mode=%d %q", m.mode, m.status)
	}
}

func TestFooterSaysWhatToDoNextInsteadOfCountingPages(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a")}}})
	if v := m.View(); !strings.Contains(v, "all loaded") || strings.Contains(v, "Pages:") {
		t.Fatalf("a complete list should say so:\n%s", v)
	}
	m = loaded(t, threeRows()) // more pages exist
	if v := m.View(); strings.Contains(v, "all loaded") || !strings.Contains(v, "Press n to load more...") {
		t.Fatalf("an incomplete list tells how to get more:\n%s", v)
	}
	m.cur().loading = true // a later page is in flight
	if v := m.View(); !strings.Contains(v, "Loading more...") {
		t.Fatalf("a background page says it is loading:\n%s", v)
	}
	fresh := newModel(threeRows())
	if v := fresh.View(); !strings.Contains(v, "Loading...") {
		t.Fatalf("the first page shows a loading marker:\n%s", v)
	}
}

func TestParseThemeInheritsFromItsBase(t *testing.T) {
	th, err := ParseTheme([]byte("base: light\ncolors:\n  border: \"#112233\"\n  Title: \"#AABBCC\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Border != "#112233" || th.Title != "#AABBCC" || th.Muted != themes["light"].Muted {
		t.Fatalf("overrides on top of light: %+v", th)
	}
	d, err := ParseTheme([]byte("colors: {ok: \"#00ff00\"}"))
	if err != nil || d.Border != themes["dark"].Border || d.OK != "#00ff00" {
		t.Fatalf("base defaults to dark: %+v %v", d, err)
	}
	for _, bad := range []string{"colors: {border: red}", "colors: {borderr: \"#000000\"}", "base: neon", "colors: {border: \"#12\"}", ": : :"} {
		if _, err := ParseTheme([]byte(bad)); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestLoadThemesFromADirectory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("Nord.yaml", "colors: {border: \"#88C0D0\"}")
	write("broken.yml", "colors: {border: nope}")
	write("notes.txt", "ignored")
	write("auto.yaml", "colors: {border: \"#000000\"}") // "auto" is reserved
	t.Cleanup(func() { delete(themes, "nord"); delete(themeProblems, "broken") })

	got := LoadThemes(dir)
	if len(got) != 1 || got[0] != "nord" {
		t.Fatalf("loaded %v", got)
	}
	if LoadThemes(filepath.Join(dir, "missing")) != nil {
		t.Fatal("a missing directory is not an error")
	}
	if !slices.Contains(ThemeNames(), "nord") {
		t.Fatal("the new theme should be listed")
	}
	withTheme(t, "nord")
	if colFg != lipgloss.Color("#88C0D0") {
		t.Fatalf("custom colour not applied: %v", colFg)
	}
	err := SetTheme("broken")
	if err == nil || !strings.Contains(err.Error(), "broken.yml") || !strings.Contains(err.Error(), "#rrggbb") {
		t.Fatalf("choosing a broken theme should say why: %v", err)
	}
}

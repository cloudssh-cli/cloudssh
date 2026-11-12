package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
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

func TestBracketsGoBackAndForwardAndBracesSwitchViews(t *testing.T) {
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

	// { and } walk dashboard, view 1, view 2 and wrap around
	m, _ = send(m, key("}"))
	if m.mode != 2 {
		t.Fatalf("} from view 1 should open view 2, mode=%d", m.mode)
	}
	m, _ = send(m, key("}"))
	if m.mode != 0 {
		t.Fatalf("} should wrap to the dashboard, mode=%d", m.mode)
	}
	m, _ = send(m, key("{"))
	if m.mode != 2 {
		t.Fatalf("{ should wrap back to the last view, mode=%d", m.mode)
	}
}

func TestFooterSaysAllLoadedAndShowsLoadingOnlyForTheFirstPage(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a")}}})
	if v := m.View(); !strings.Contains(v, "Pages: 1") || !strings.Contains(v, "all loaded") {
		t.Fatalf("a complete list should say so:\n%s", v)
	}
	m = loaded(t, threeRows()) // more pages exist
	if v := m.View(); strings.Contains(v, "all loaded") || !strings.Contains(v, "Pages: 1+") {
		t.Fatalf("an incomplete list must not claim to be complete:\n%s", v)
	}
	// a later page in flight leaves the count showing; only the first page shows "..."
	m.cur().loading = true
	if v := m.View(); !strings.Contains(v, "Pages: 1+") {
		t.Fatalf("background pages must not blank the counter:\n%s", v)
	}
	fresh := newModel(threeRows())
	if v := fresh.View(); !strings.Contains(v, "Pages: ...") {
		t.Fatalf("the first page shows a loading marker:\n%s", v)
	}
}

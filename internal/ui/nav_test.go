package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func twoLevel() (fakeResource, drillRes, *recorder) {
	rec := &recorder{}
	child := fakeResource{
		key: "child", rec: rec,
		filters:  []provider.FilterDef{{Key: "parent", Name: "Parent"}, {Key: "state", Name: "State"}},
		defaults: []provider.Filter{{Key: "state", Value: "running"}},
		pages:    map[string]provider.Page{"": {Rows: rows("c1", "c2")}},
	}
	parent := drillRes{
		fakeResource: fakeResource{key: "parent", pages: map[string]provider.Page{"": {Rows: rows("p1", "p2")}}},
		child:        child,
	}
	return child, parent, rec
}

func TestDrillPushesScreenAndEscPops(t *testing.T) {
	_, parent, rec := twoLevel()
	m := newModelWith(testCfg(), parent)
	m = settle(m, m.Init())
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	if len(m.stacks[m.mode]) != 2 {
		t.Fatalf("stack depth %d", len(m.stacks[m.mode]))
	}
	// drill filters replace the child's default state filter
	if q := rec.last(); len(q.Filters) != 1 || q.Filters[0].Key != "parent" {
		t.Fatalf("child query filters: %+v", q.Filters)
	}
	if got := m.crumbs(); len(got) != 2 || got[0] != "fake_parent" || got[1] != "fake_child" {
		t.Fatalf("crumbs: %v", got)
	}
	v := m.View()
	for _, want := range []string{"<fake_parent>", "<fake_child>", "Go back"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	m, _ = send(m, key("esc"))
	if len(m.stacks[m.mode]) != 1 {
		t.Fatal("esc should pop the drill")
	}
	m, _ = send(m, key("esc")) // at the root esc is harmless
	if len(m.stacks[m.mode]) != 1 {
		t.Fatal("root must not pop")
	}
}

func TestDrillShowsHintAndErrors(t *testing.T) {
	child, parent, _ := twoLevel()
	parent.err = errString2("cluster has no services")
	parent.child = child
	m := newModelWith(testCfg(), parent)
	m = settle(m, m.Init())
	if strings.Contains(m.View(), "Child View") {
		t.Fatal("drill hints belong in help, not the header")
	}
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	if len(m.stacks[m.mode]) != 1 || !strings.Contains(m.status, "no services") {
		t.Fatalf("depth=%d status=%q", len(m.stacks[m.mode]), m.status)
	}
}

type errString2 string

func (e errString2) Error() string { return string(e) }

func TestResetRestoresDrillFilters(t *testing.T) {
	_, parent, rec := twoLevel()
	m := newModelWith(testCfg(), parent)
	m = settle(m, m.Init())
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	s := m.cur()
	s.filters = []provider.Filter{{Key: "state", Value: "stopped"}}
	_, cmd = send(m, key("R"))
	settle(m, cmd)
	if q := rec.last(); len(q.Filters) != 1 || q.Filters[0] != (provider.Filter{Key: "parent", Value: "p1"}) {
		t.Fatalf("reset query: %+v", q.Filters)
	}
}

func TestTopLevelStartsWithDefaultFilters(t *testing.T) {
	rec := &recorder{}
	res := fakeResource{rec: rec, defaults: []provider.Filter{{Key: "state", Value: "running"}}, pages: map[string]provider.Page{"": {}}}
	m := newModelWith(testCfg(), res)
	settle(m, m.Init())
	if q := rec.last(); len(q.Filters) != 1 || q.Filters[0].Value != "running" {
		t.Fatalf("default filters not applied: %+v", q.Filters)
	}
}

func TestSwitchModesPreservesStacksAndRoutesLoads(t *testing.T) {
	a := fakeResource{key: "a", pages: map[string]provider.Page{"": {Rows: rows("a1")}}}
	b := fakeResource{key: "b", pages: map[string]provider.Page{"": {Rows: rows("b1", "b2")}}}
	m := newModelWith(testCfg(), a, b)
	// the load for mode 1 is still in flight when the user switches to mode 2
	inflight := run(m.Init())
	m, cmd := send(m, key("2"))
	if m.mode != 2 {
		t.Fatalf("mode=%d", m.mode)
	}
	for _, msg := range inflight { // late response for the hidden screen
		m, _ = send(m, msg)
	}
	m = settle(m, cmd)
	if len(m.stacks[1][0].rows) != 1 || len(m.stacks[2][0].rows) != 2 {
		t.Fatalf("rows routed wrongly: a=%d b=%d", len(m.stacks[1][0].rows), len(m.stacks[2][0].rows))
	}
	if m.cur().res.Key() != "b" || len(m.cur().tbl.cols) != 2 {
		t.Fatal("late load clobbered the visible screen")
	}
	m, cmd = send(m, key("1"))
	if cmd != nil && len(run(cmd)) > 0 {
		t.Fatal("returning to a loaded mode must not reload")
	}
	if m.cur().res.Key() != "a" {
		t.Fatal("did not return to mode 1")
	}
	m, _ = send(m, key("9")) // no such mode
	if m.mode != 1 {
		t.Fatalf("out-of-range digit changed mode to %d", m.mode)
	}
}

func TestDashboard(t *testing.T) {
	a := fakeResource{key: "a"}
	b := fakeResource{key: "b"}
	cfg := testCfg()
	cfg.View = "dashboard"
	m := newModelWith(cfg, a, b)
	if m.mode != 0 || m.cur() != nil {
		t.Fatal("should start on the dashboard")
	}
	v := m.View()
	for _, want := range []string{"Dashboard", "Shortcut(s)", "Fake a", "Fake b", "Help", "Quit", "<dashboard>", "/ ____/"} {
		if !strings.Contains(v, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	m, cmd := send(m, key("2"))
	if m.mode != 2 || cmd == nil {
		t.Fatalf("digit should open view 2 and load it (mode=%d)", m.mode)
	}
	m, _ = send(m, key("d"))
	if m.mode != 0 {
		t.Fatal("d should return to the dashboard")
	}
	m, _ = send(m, key("n")) // screen keys are inert on the dashboard
	if m.mode != 0 {
		t.Fatal("unexpected mode change")
	}
}

func TestStartViewSelectsResource(t *testing.T) {
	a, b := fakeResource{key: "a"}, fakeResource{key: "b"}
	cfg := testCfg()
	cfg.View = "b"
	if m := newModelWith(cfg, a, b); m.mode != 2 {
		t.Fatalf("mode=%d", m.mode)
	}
	cfg.View = "nonsense"
	if m := newModelWith(cfg, a, b); m.mode != 1 {
		t.Fatalf("unknown view should fall back to first resource, mode=%d", m.mode)
	}
}

func TestHelpListsKeysAndSortColumns(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("?"))
	if m.modal == nil {
		t.Fatal("help not open")
	}
	v := m.View()
	for _, want := range []string{"Help", "Dashboard", "Sorting", "ID", "Name"} {
		if !strings.Contains(v, want) {
			t.Errorf("help missing %q", want)
		}
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyPgDown}) // the rest is reachable by scrolling
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(m.View(), "Update server-side filters") {
		t.Error("help should scroll to the Data section")
	}
	m, cmd := send(m, key("q")) // q closes help, it must not quit
	if m.modal != nil {
		t.Fatal("q should close help")
	}
	for _, msg := range run(cmd) {
		if _, quit := msg.(tea.QuitMsg); quit {
			t.Fatal("q in help quit the app")
		}
	}
}

func TestFiltersUnavailable(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("F"))
	if m.modal != nil || !strings.Contains(m.status, "no filters") {
		t.Fatalf("modal=%v status=%q", m.modal, m.status)
	}
}

func filterable(rec *recorder) fakeResource {
	return fakeResource{
		rec:      rec,
		filters:  []provider.FilterDef{{Key: "name", Name: "Instance Name"}, {Key: "state", Name: "Instance State"}, {Key: "az", Name: "AZ"}},
		defaults: []provider.Filter{{Key: "state", Value: "running"}},
		pages:    map[string]provider.Page{"": {Rows: rows("a", "b")}},
	}
}

func TestFiltersModalListsEveryFilterAtOnce(t *testing.T) {
	rec := &recorder{}
	m := newModelWith(testCfg(), filterable(rec))
	m = settle(m, m.Init())
	m, _ = send(m, key("F"))
	v := m.View()
	for _, want := range []string{"Update data filters", "Instance Name", "Instance State", "AZ", "running", "(any)"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q", want)
		}
	}
	if fm := m.modal.(*filtersModal); fm.defs[fm.focus].Key != "state" {
		t.Fatalf("focus should start on the filter that is already set, got %s", fm.defs[fm.focus].Key)
	}
}

func TestFiltersModalAppliesAndReloads(t *testing.T) {
	rec := &recorder{}
	m := newModelWith(testCfg(), filterable(rec))
	m = settle(m, m.Init())
	m, _ = send(m, key("F"))
	m = typeKeys(m, "x") // state: running -> runningx
	m, _ = send(m, key("up"))
	m = typeKeys(m, "web") // name
	m, cmd := send(m, key("enter"))
	if m.modal != nil {
		t.Fatal("enter should close the modal")
	}
	m = settle(m, cmd)
	q := rec.last()
	want := []provider.Filter{{Key: "name", Value: "web"}, {Key: "state", Value: "runningx"}}
	if len(q.Filters) != 2 || q.Filters[0] != want[0] || q.Filters[1] != want[1] {
		t.Fatalf("applied filters: %+v", q.Filters)
	}
	if !strings.Contains(m.View(), "Instance Name=web") {
		t.Fatal("header should summarise active filters")
	}
}

func TestFiltersModalCancelAndClearAll(t *testing.T) {
	rec := &recorder{}
	m := newModelWith(testCfg(), filterable(rec))
	m = settle(m, m.Init())
	m, _ = send(m, key("F"))
	m = typeKeys(m, "zzz")
	m, _ = send(m, key("esc"))
	if m.modal != nil || m.cur().filters[0].Value != "running" {
		t.Fatal("esc must cancel without changing filters")
	}
	m, _ = send(m, key("F"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyCtrlX})
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	if len(rec.last().Filters) != 0 || len(m.cur().filters) != 0 {
		t.Fatalf("filters not cleared: %+v", rec.last().Filters)
	}
}

func TestFiltersModalKeepsUnknownActiveFilterEditable(t *testing.T) {
	rec := &recorder{}
	res := filterable(rec)
	res.defaults = []provider.Filter{{Key: "subnet-id", Value: "subnet-1"}} // not in Filters()
	m := newModelWith(testCfg(), res)
	m = settle(m, m.Init())
	m, _ = send(m, key("F"))
	fm := m.modal.(*filtersModal)
	if len(fm.defs) != 4 || fm.defs[3].Key != "subnet-id" || !strings.Contains(m.View(), "subnet-1") {
		t.Fatal("unknown filter should be listed with its value")
	}
	m, cmd := send(m, key("enter"))
	settle(m, cmd)
	if q := rec.last(); len(q.Filters) != 1 || q.Filters[0].Key != "subnet-id" {
		t.Fatalf("drill filter lost on apply: %+v", q.Filters)
	}
}

func TestFiltersModalScrollsWhenTallerThanScreen(t *testing.T) {
	var defs []provider.FilterDef
	for i := 0; i < 15; i++ {
		defs = append(defs, provider.FilterDef{Key: "k" + string(rune('a'+i)), Name: "Filter " + string(rune('A'+i))})
	}
	res := fakeResource{filters: defs, pages: map[string]provider.Page{"": {}}}
	m := newModelWith(testCfg(), res)
	m = sized(settle(m, m.Init()), 90, 18)
	m, _ = send(m, key("F"))
	for i := 0; i < 14; i++ {
		m, _ = send(m, key("down"))
	}
	if !strings.Contains(m.View(), "Filter O") { // the last row, focused
		t.Fatal("focused row must stay visible")
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) != 18 {
		t.Fatalf("frame is %d lines", len(lines))
	}
}

func TestModalSwallowsGlobalKeys(t *testing.T) {
	rec := &recorder{}
	m := newModelWith(testCfg(), filterable(rec))
	m = settle(m, m.Init())
	m, _ = send(m, key("F"))
	for _, k := range []string{"q", "d", "1", "?", "/"} {
		var cmd tea.Cmd
		m, cmd = send(m, key(k))
		for _, msg := range run(cmd) {
			if _, quit := msg.(tea.QuitMsg); quit {
				t.Fatalf("%q quit from the filters modal", k)
			}
		}
	}
	if m.mode != 1 || m.modal == nil {
		t.Fatalf("mode=%d modal=%v", m.mode, m.modal)
	}
	_, cmd := send(m, key("ctrl+c")) // ctrl+c always quits
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c should quit")
	}
}

func TestSplashShowsFadesAndCanBeSkipped(t *testing.T) {
	cfg := testCfg()
	cfg.SkipSplash = false
	m := newModelWith(cfg, threeRows())
	if !strings.Contains(m.View(), "/ ____/") || strings.Contains(m.View(), "Pages:") {
		t.Fatal("splash should show only the logo")
	}
	m, cmd := send(m, splashMsg{1})
	if !m.splashing || cmd == nil {
		t.Fatal("fade should continue")
	}
	m, _ = send(m, splashMsg{splashFadeSteps + 1})
	if m.splashing {
		t.Fatal("splash should end after the fade")
	}

	m = newModelWith(cfg, threeRows())
	m, _ = send(m, key("x")) // any key skips and is consumed
	if m.splashing || m.mode != 1 {
		t.Fatal("key should skip the splash")
	}
}

func TestViewHasHeaderPanelTitleAndBreadcrumbs(t *testing.T) {
	v := loaded(t, threeRows()).View()
	for _, want := range []string{"Cloud:", "Filters:", "Loaded:", "<enter/S>", "<r>", "<?>", "Fake[3]<all>", "<fake_fake>", "Pages: 1+", "alpha"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	if strings.Contains(v, "Scroll columns") || strings.Contains(v, "Sort order") {
		t.Error("scroll/sort hints should live in help only")
	}
}

func TestTitleShowsSearch(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("/"))
	m = typeKeys(m, "alp")
	if !strings.Contains(m.View(), "Fake[1]</alp>") {
		t.Fatal("title should show count and search")
	}
}

// Every screen state must fill the terminal exactly, at several sizes.
func TestEveryStateFitsTheScreen(t *testing.T) {
	rec := &recorder{}
	child, parent, _ := twoLevel()
	_ = child
	states := map[string]func(Model) Model{
		"table":     func(m Model) Model { return m },
		"search":    func(m Model) Model { m, _ = send(m, key("/")); return typeKeys(m, "a") },
		"help":      func(m Model) Model { m, _ = send(m, key("?")); return m },
		"filters":   func(m Model) Model { m, _ = send(m, key("F")); return m },
		"dashboard": func(m Model) Model { m, _ = send(m, key("d")); return m },
		"ssh": func(m Model) Model {
			m, _ = send(m, resolvedMsg{host: provider.Host{PrivateIP: "10.0.0.1", PublicIP: "3.3.3.3"}})
			return m
		},
		"drilled": func(m Model) Model {
			m, cmd := send(m, key("enter"))
			return settle(m, cmd)
		},
	}
	for name, setup := range states {
		for _, size := range [][2]int{{120, 30}, {80, 24}, {50, 12}, {200, 50}} {
			var res provider.Resource = filterable(rec)
			if name == "drilled" {
				res = parent
			}
			m := newModelWith(testCfg(), res)
			m = settle(m, m.Init())
			m = sized(m, size[0], size[1])
			m = sized(setup(m), size[0], size[1])
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s %v: got %d lines, want %d", name, size, len(lines), size[1])
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%s %v: line %d is %d wide", name, size, i, w)
				}
			}
		}
	}
}

func TestHelpHintSurvivesNarrowTerminals(t *testing.T) {
	m := sized(loaded(t, threeRows()), 60, 14)
	if !strings.Contains(m.View(), "<?> Help") {
		t.Fatal("help hint must always be visible")
	}
	d, _ := send(m, key("d"))
	if !strings.Contains(d.View(), "<?> Help") {
		t.Fatal("help hint must be visible on the dashboard too")
	}
}

func TestHeaderListsOnlyActionShortcuts(t *testing.T) {
	m := sized(loaded(t, filterable(&recorder{})), 220, 30)
	v := m.View()
	for _, want := range []string{"SSH session", "Update filters", "Open search", "Refresh data", "Reset defaults", "Next page",
		"Switch view", "Dashboard", "Help", "Quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("wide header missing %q", want)
		}
	}
	for _, gone := range []string{"Sort column", "Move", "Page up", "Top / bottom", "Scroll columns"} {
		if strings.Contains(v, gone) {
			t.Errorf("header should not list %q (it is in ? help)", gone)
		}
	}
	// a narrow terminal drops trailing columns instead of overflowing
	n := sized(m, 70, 30)
	for _, l := range strings.Split(n.View(), "\n") {
		if lipgloss.Width(l) > 70 {
			t.Fatalf("overflow: %q", l)
		}
	}
}

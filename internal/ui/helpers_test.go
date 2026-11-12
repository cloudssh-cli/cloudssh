package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/sshx"
)

type recorder struct {
	mu      sync.Mutex
	queries []provider.Query
}

func (r *recorder) add(q provider.Query) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
}

func (r *recorder) last() provider.Query {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queries) == 0 {
		return provider.Query{}
	}
	return r.queries[len(r.queries)-1]
}

type fakeResource struct {
	key      string
	pages    map[string]provider.Page
	err      error
	filters  []provider.FilterDef
	defaults []provider.Filter
	cols     []provider.Column
	rec      *recorder
}

func (f fakeResource) Key() string {
	if f.key == "" {
		return "fake"
	}
	return f.key
}
func (f fakeResource) Label() string { return "Fake " + f.Key() }
func (f fakeResource) Name() string  { return "Fake" }
func (f fakeResource) Crumb() string { return "fake_" + f.Key() }
func (f fakeResource) Columns() []provider.Column {
	if f.cols != nil {
		return f.cols
	}
	return []provider.Column{{Title: "ID"}, {Title: "Name"}}
}
func (f fakeResource) Filters() []provider.FilterDef     { return f.filters }
func (f fakeResource) DefaultFilters() []provider.Filter { return f.defaults }
func (f fakeResource) List(_ context.Context, q provider.Query) (provider.Page, error) {
	if f.rec != nil {
		f.rec.add(q)
	}
	return f.pages[q.Token], f.err
}

// drillRes is a fakeResource whose rows open a child resource.
type drillRes struct {
	fakeResource
	child provider.Resource
	err   error
}

func (drillRes) DrillHint() string { return "Child View" }
func (d drillRes) Drill(context.Context, provider.Row) (provider.Drill, error) {
	return provider.Drill{Resource: d.child, Filters: []provider.Filter{{Key: "parent", Value: "p1"}}}, d.err
}

type fakeProvider struct{ res []provider.Resource }

func (fakeProvider) Name() string                     { return "fake" }
func (p fakeProvider) Resources() []provider.Resource { return p.res }

func rows(names ...string) []provider.Row {
	var out []provider.Row
	for _, n := range names {
		out = append(out, provider.Row{
			Cells: []string{"id-" + n, n},
			Ref:   provider.Host{ID: "id-" + n, Name: n, PrivateIP: "10.0.0.1"},
		})
	}
	return out
}

func testCfg() config.Config {
	c := config.Default()
	c.SkipSplash = true
	return c
}

func sized(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func newModelWith(cfg config.Config, res ...provider.Resource) Model {
	m := New(cfg, fakeProvider{res})
	m.detectTunnels = func(context.Context) []sshx.LocalTunnel { return nil } // never scan real processes in tests
	return sized(m, 120, 30)
}

func newModel(res fakeResource) Model { return newModelWith(testCfg(), res) }

func send(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeKeys(m Model, s string) Model {
	for _, r := range s {
		m, _ = send(m, key(string(r)))
	}
	return m
}

// run executes cmd, flattening batches. Sub-commands slower than a few ms
// (the status-clear and splash ticks) are skipped.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, run(c)...)
			}
			return out
		}
		return []tea.Msg{msg}
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

// settle runs cmd and feeds every resulting message back into the model,
// following follow-up commands too.
func settle(m Model, cmd tea.Cmd) Model {
	queue := []tea.Cmd{cmd}
	for i := 0; i < 20 && len(queue) > 0; i++ {
		c := queue[0]
		queue = queue[1:]
		for _, msg := range run(c) {
			var next tea.Cmd
			m, next = send(m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

func loaded(t *testing.T, res fakeResource) Model {
	t.Helper()
	m := newModel(res)
	return settle(m, m.Init())
}

func threeRows() fakeResource {
	return fakeResource{pages: map[string]provider.Page{"": {Rows: rows("alpha", "beta", "gamma"), NextToken: "t1"}}}
}

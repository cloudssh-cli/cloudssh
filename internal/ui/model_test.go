package ui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

type fakeResource struct {
	pages map[string]provider.Page
	err   error
}

func (fakeResource) Name() string { return "Fake" }
func (fakeResource) Columns() []provider.Column {
	return []provider.Column{{Title: "ID"}, {Title: "Name"}}
}
func (f fakeResource) List(_ context.Context, token string) (provider.Page, error) {
	return f.pages[token], f.err
}

type fakeProvider struct{ res fakeResource }

func (fakeProvider) Name() string                     { return "fake" }
func (p fakeProvider) Resources() []provider.Resource { return []provider.Resource{p.res} }

func rows(names ...string) []provider.Row {
	var out []provider.Row
	for _, n := range names {
		out = append(out, provider.Row{
			Cells: []string{"id-" + n, n},
			Ref:   provider.Host{ID: "id-" + n, PrivateIP: "10.0.0.1"},
		})
	}
	return out
}

func newModel(res fakeResource) Model {
	m := New(config.Default(), fakeProvider{res})
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m2.(Model)
}

func send(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestInitLoadsFirstPage(t *testing.T) {
	m := newModel(fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a", "b")}}})
	msg := m.Init()()
	m, _ = send(m, msg)
	if len(m.rows) != 2 || m.loading {
		t.Fatalf("rows=%d loading=%v", len(m.rows), m.loading)
	}
}

func TestLoadErrorShownInStatus(t *testing.T) {
	m := newModel(fakeResource{err: errors.New("boom")})
	m, _ = send(m, m.Init()())
	if m.status != "error: boom" {
		t.Fatalf("status=%q", m.status)
	}
}

func TestNextPageAppends(t *testing.T) {
	m := newModel(fakeResource{pages: map[string]provider.Page{
		"":   {Rows: rows("a"), NextToken: "t1"},
		"t1": {Rows: rows("b")},
	}})
	m, _ = send(m, m.Init()())
	m, cmd := send(m, key("n"))
	if cmd == nil {
		t.Fatal("expected load command")
	}
	m, _ = send(m, cmd())
	if len(m.rows) != 2 || m.nextToken != "" {
		t.Fatalf("rows=%d token=%q", len(m.rows), m.nextToken)
	}
	m, _ = send(m, key("n"))
	if m.status != "no more pages" {
		t.Fatalf("status=%q", m.status)
	}
}

func TestFilterNarrowsRowsAndEscClears(t *testing.T) {
	m := newModel(fakeResource{pages: map[string]provider.Page{"": {Rows: rows("alpha", "beta")}}})
	m, _ = send(m, m.Init()())
	m, _ = send(m, key("/"))
	for _, r := range "alp" {
		m, _ = send(m, key(string(r)))
	}
	if len(m.view) != 1 || m.view[0].Cells[1] != "alpha" {
		t.Fatalf("view=%v", m.view)
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.view) != 2 || m.filtering {
		t.Fatalf("after esc: view=%d filtering=%v", len(m.view), m.filtering)
	}
}

func TestEnterOnEmptyTableDoesNothing(t *testing.T) {
	m := newModel(fakeResource{pages: map[string]provider.Page{"": {}}})
	m, _ = send(m, m.Init()())
	_, cmd := send(m, key("enter"))
	if cmd != nil {
		t.Fatal("expected no command")
	}
}

func TestEnterResolvesHost(t *testing.T) {
	m := newModel(fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a")}}})
	m, _ = send(m, m.Init()())
	_, cmd := send(m, key("enter"))
	if cmd == nil {
		t.Fatal("expected resolve command")
	}
	got, ok := cmd().(resolvedMsg)
	if !ok || got.host.ID != "id-a" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolvedMsgStartsSSH(t *testing.T) {
	m := newModel(fakeResource{})
	_, cmd := send(m, resolvedMsg{host: provider.Host{PrivateIP: "10.0.0.1", DefaultUser: "ubuntu"}})
	if cmd == nil {
		t.Fatal("expected ExecProcess command")
	}
}

func TestResolvedMsgWithoutIPReportsError(t *testing.T) {
	m := newModel(fakeResource{})
	m, cmd := send(m, resolvedMsg{host: provider.Host{}})
	if cmd != nil || m.status == "" {
		t.Fatalf("cmd=%v status=%q", cmd, m.status)
	}
}

func TestQuit(t *testing.T) {
	m := newModel(fakeResource{})
	_, cmd := send(m, key("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestViewMentionsProviderAndResource(t *testing.T) {
	v := newModel(fakeResource{}).View()
	if !contains(v, "fake") || !contains(v, "Fake") {
		t.Fatalf("view missing header: %q", v)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

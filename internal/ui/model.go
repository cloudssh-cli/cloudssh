// Package ui is the Bubble Tea front end. It only knows provider interfaces.
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/sshx"
)

type loadedMsg struct {
	page   provider.Page
	append bool
	err    error
}

type sshDoneMsg struct{ err error }

type resolvedMsg struct {
	host provider.Host
	err  error
}

type Model struct {
	cfg       config.Config
	prov      provider.Provider
	resources []provider.Resource
	current   int

	table     table.Model
	filter    textinput.Model
	filtering bool

	rows      []provider.Row // all loaded rows
	view      []provider.Row // rows matching filter, aligned with table rows
	nextToken string
	loading   bool
	status    string
	width     int
	height    int
}

func New(cfg config.Config, p provider.Provider) Model {
	f := textinput.New()
	f.Prompt = "/"
	f.Placeholder = "filter"
	m := Model{cfg: cfg, prov: p, resources: p.Resources(), filter: f, loading: true}
	m.table = table.New(table.WithFocused(true))
	m.applyColumns()
	return m
}

func (m Model) Init() tea.Cmd { return m.load("", false) }

func (m Model) res() provider.Resource { return m.resources[m.current] }

func (m *Model) applyColumns() {
	cols := m.res().Columns()
	tc := make([]table.Column, len(cols))
	for i, c := range cols {
		tc[i] = table.Column{Title: c.Title, Width: len(c.Title)}
	}
	m.table.SetRows(nil)
	m.table.SetColumns(tc)
}

func (m Model) load(token string, appendRows bool) tea.Cmd {
	res := m.res()
	return func() tea.Msg {
		page, err := res.List(context.Background(), token)
		return loadedMsg{page: page, append: appendRows, err: err}
	}
}

func (m *Model) refreshTable() {
	needle := strings.ToLower(m.filter.Value())
	m.view = m.view[:0]
	var trows []table.Row
	widths := make([]int, len(m.table.Columns()))
	for i, c := range m.table.Columns() {
		widths[i] = len(c.Title)
	}
	for _, r := range m.rows {
		if needle != "" && !strings.Contains(strings.ToLower(strings.Join(r.Cells, "\x00")), needle) {
			continue
		}
		m.view = append(m.view, r)
		trows = append(trows, table.Row(r.Cells))
		for i, c := range r.Cells {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	cols := m.table.Columns()
	for i := range cols {
		cols[i].Width = widths[i]
	}
	prev := m.table.Cursor()
	m.table.SetRows(nil) // avoid index panic while columns change
	m.table.SetColumns(cols)
	m.table.SetRows(trows)
	// bubbles/table leaves the cursor at -1 after rows go empty -> non-empty.
	m.table.SetCursor(min(max(prev, 0), max(len(trows)-1, 0)))
	m.table.SetHeight(max(m.height-5, 3))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetHeight(max(m.height-5, 3))
		return m, nil

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		if msg.append {
			m.rows = append(m.rows, msg.page.Rows...)
		} else {
			m.rows = msg.page.Rows
		}
		m.nextToken = msg.page.NextToken
		m.status = fmt.Sprintf("%d loaded", len(m.rows))
		m.refreshTable()
		return m, nil

	case resolvedMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		t, err := sshx.Resolve(msg.host, "", m.cfg.DefaultIP, config.ExpandHome(m.cfg.PemFilesPath))
		if err != nil {
			m.status = "error: " + err.Error()
			return m, nil
		}
		// ExecProcess suspends the TUI and gives ssh the real terminal.
		return m, tea.ExecProcess(t.Command(), func(err error) tea.Msg { return sshDoneMsg{err} })

	case sshDoneMsg:
		if msg.err != nil {
			m.status = "ssh: " + msg.err.Error()
		} else {
			m.status = "ssh session ended"
		}
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
				m.filter.Blur()
				if msg.String() == "esc" {
					m.filter.SetValue("")
					m.refreshTable()
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.refreshTable()
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.filtering = true
			return m, m.filter.Focus()
		case "r":
			m.loading = true
			return m, m.load("", false)
		case "n":
			if m.nextToken == "" {
				m.status = "no more pages"
				return m, nil
			}
			m.loading = true
			return m, m.load(m.nextToken, true)
		case "enter":
			return m.connect()
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) connect() (tea.Model, tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.view) {
		return m, nil
	}
	host, ok := m.view[idx].Ref.(provider.Host)
	if !ok {
		m.status = "this resource cannot be connected to"
		return m, nil
	}
	m.status = "resolving ssh user..."
	resolver, _ := m.prov.(provider.HostResolver)
	return m, func() tea.Msg {
		if resolver == nil {
			return resolvedMsg{host: host}
		}
		h, err := resolver.ResolveHost(context.Background(), host)
		return resolvedMsg{host: h, err: err}
	}
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m Model) View() string {
	head := titleStyle.Render(fmt.Sprintf("cloudssh · %s · %s", m.prov.Name(), m.res().Name()))
	if m.loading {
		head += statusStyle.Render("  loading...")
	}
	foot := statusStyle.Render(m.status + "   / filter · n next page · r refresh · enter ssh · q quit")
	if m.filtering || m.filter.Value() != "" {
		foot = m.filter.View() + "\n" + foot
	}
	return head + "\n" + m.table.View() + "\n" + foot
}

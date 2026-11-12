// Package ui is the Bubble Tea front end. It only knows provider interfaces.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/sshx"
)

const (
	headerHeight = 5
	footerHeight = 2
	filterHeight = 3
	minTableH    = 3
	statusTTL    = 4 * time.Second
	historyMax   = 20
	splashHold   = 500 * time.Millisecond
	splashStep   = 100 * time.Millisecond
)

// loadedMsg carries a page back to the screen (sid) that asked for it.
type loadedMsg struct {
	sid    int
	gen    int
	page   provider.Page
	append bool
	err    error
}

type drilledMsg struct {
	from  int // screen the drill started on
	drill provider.Drill
	err   error
}

type resolvedMsg struct {
	host provider.Host
	err  error
}

// eicMsg is the result of pushing a temporary key (EC2 Instance Connect):
// on success the model runs ssh with that key and deletes it afterwards.
type eicMsg struct {
	target sshx.Target
	key    sshx.TempKey
	err    error
}

type (
	sshDoneMsg     struct{ err error }
	clearStatusMsg struct{ seq int }
	splashMsg      struct{ step int }
)

// Model is the root Bubble Tea model. Each mode (0 = dashboard, 1..n = the
// provider's resources) keeps its own stack of screens, so switching views
// preserves where you were, like the Python app's Textual modes.
type Model struct {
	cfg       config.Config
	prov      provider.Provider
	resources []provider.Resource
	stacks    [][]*screen
	mode      int
	nextID    int

	filter    textinput.Model // shared search input
	filtering bool
	history   []string
	histPos   int

	modal     modal
	splashing bool
	splashAt  int

	status    string
	statusSeq int
	width     int
	height    int
}

func New(cfg config.Config, p provider.Provider) Model {
	f := textinput.New()
	f.Prompt = "🔍 "
	f.Placeholder = "Enter your search string here..."
	m := Model{
		cfg: cfg, prov: p, resources: p.Resources(), filter: f,
		splashing: !cfg.SkipSplash, nextID: 1,
	}
	m.stacks = make([][]*screen, len(m.resources)+1)
	m.mode = m.startMode()
	if m.mode > 0 {
		m.stacks[m.mode] = []*screen{m.newScreen(m.resources[m.mode-1], nil)}
	}
	return m
}

func (m Model) startMode() int {
	if m.cfg.View == "dashboard" || len(m.resources) == 0 {
		return 0
	}
	for i, r := range m.resources {
		if r.Key() == m.cfg.View {
			return i + 1
		}
	}
	return 1
}

// newScreen creates a screen; filters==nil means the resource's defaults.
func (m *Model) newScreen(res provider.Resource, filters []provider.Filter) *screen {
	if len(filters) == 0 {
		filters = res.DefaultFilters()
	}
	s := newScreen(m.nextID, res, filters, m.width, m.tableH())
	m.nextID++
	return s
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if s := m.cur(); s != nil {
		cmds = append(cmds, s.load("", false))
	}
	if m.splashing {
		cmds = append(cmds, tea.Tick(splashHold, func(time.Time) tea.Msg { return splashMsg{1} }))
	}
	return tea.Batch(cmds...)
}

// cur is the visible screen, nil on the dashboard.
func (m Model) cur() *screen {
	if m.mode == 0 || len(m.stacks[m.mode]) == 0 {
		return nil
	}
	st := m.stacks[m.mode]
	return st[len(st)-1]
}

func (m Model) find(id int) *screen {
	for _, st := range m.stacks {
		for _, s := range st {
			if s.id == id {
				return s
			}
		}
	}
	return nil
}

func (m *Model) tableH() int {
	fh := 0
	if m.filtering {
		fh = filterHeight
	}
	return max(m.height-headerHeight-fh-footerHeight, minTableH)
}

func (m *Model) layout() {
	for _, st := range m.stacks {
		for _, s := range st {
			s.tbl.resize(m.width, m.tableH())
		}
	}
	m.filter.Width = max(m.width-8, 10)
}

// setStatus shows a notification that clears itself after statusTTL.
func (m *Model) setStatus(s string) tea.Cmd {
	m.status = s
	m.statusSeq++
	if s == "" {
		return nil
	}
	seq := m.statusSeq
	return tea.Tick(statusTTL, func(time.Time) tea.Msg { return clearStatusMsg{seq} })
}

func (m *Model) pushHistory(s string) {
	if s == "" {
		return
	}
	for i, h := range m.history {
		if h == s {
			m.history = append(m.history[:i], m.history[i+1:]...)
			break
		}
	}
	m.history = append(m.history, s)
	if len(m.history) > historyMax {
		m.history = m.history[len(m.history)-historyMax:]
	}
}

func (m *Model) switchMode(i int) tea.Cmd {
	if i < 0 || i >= len(m.stacks) || i == m.mode {
		return nil
	}
	m.mode = i
	m.filtering = false
	m.layout()
	if i > 0 && len(m.stacks[i]) == 0 {
		s := m.newScreen(m.resources[i-1], nil)
		m.stacks[i] = []*screen{s}
		return s.load("", false)
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case splashMsg:
		if !m.splashing {
			return m, nil
		}
		if msg.step > splashFadeSteps {
			m.splashing = false
			return m, nil
		}
		m.splashAt = msg.step
		return m, tea.Tick(splashStep, func(time.Time) tea.Msg { return splashMsg{msg.step + 1} })

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case loadedMsg:
		s := m.find(msg.sid)
		if s == nil || msg.gen != s.gen {
			return m, nil // screen gone, or superseded by a refresh
		}
		s.loading = false
		if msg.err != nil {
			return m, m.setStatus("error: " + msg.err.Error())
		}
		if msg.append {
			s.rows = append(s.rows, msg.page.Rows...)
			s.pages++
		} else {
			s.rows = msg.page.Rows
			s.pages = 1
		}
		s.nextToken = msg.page.NextToken
		s.refreshView()
		if m.cur() != s {
			return m, nil
		}
		if s.nextToken == "" {
			return m, m.setStatus(fmt.Sprintf("%d loaded. No more pages left!", len(s.rows)))
		}
		return m, m.setStatus(fmt.Sprintf("%d loaded. Press n for the next page", len(s.rows)))

	case drilledMsg:
		if msg.err != nil {
			return m, m.setStatus("error: " + msg.err.Error())
		}
		for mode, st := range m.stacks {
			if n := len(st); n > 0 && st[n-1].id == msg.from { // only if still on top
				s := m.newScreen(msg.drill.Resource, msg.drill.Filters)
				m.stacks[mode] = append(st, s)
				return m, s.load("", false)
			}
		}
		return m, nil

	case resolvedMsg:
		if msg.err != nil {
			return m, m.setStatus("error: " + msg.err.Error())
		}
		if msg.host.PrivateIP == "" && msg.host.PublicIP == "" {
			return m, m.setStatus("error: host has no reachable IP address")
		}
		auth, _ := m.prov.(provider.Authorizer)
		sm, cmd := newSSHModal(msg.host, m.cfg, auth)
		m.modal = sm
		return m, cmd

	case eicMsg:
		if msg.err != nil {
			msg.key.Cleanup()
			return m, m.setStatus("error: " + msg.err.Error())
		}
		key := msg.key
		return m, tea.ExecProcess(msg.target.Command(), func(err error) tea.Msg {
			key.Cleanup() // the temporary key never outlives the session
			return sshDoneMsg{err}
		})

	case sshDoneMsg:
		if msg.err != nil {
			return m, m.setStatus("error: ssh: " + msg.err.Error())
		}
		return m, m.setStatus("ssh session ended")

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.splashing: // any key skips the splash
		m.splashing = false
		return m, nil
	case m.modal != nil:
		done, cmd := m.modal.key(&m, msg)
		if done {
			m.modal = nil
		}
		return m, cmd
	case m.filtering:
		return m.updateSearch(msg)
	}
	return m.updateKey(msg)
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.cur()
	switch msg.String() {
	case "enter":
		m.pushHistory(m.filter.Value())
		m.filtering = false
		m.filter.Blur()
		m.layout()
		return m, nil
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		s.search = ""
		s.refreshView()
		m.layout()
		return m, nil
	case "up", "down": // browse previous searches
		if len(m.history) == 0 {
			return m, nil
		}
		if msg.String() == "up" {
			m.histPos = max(m.histPos-1, 0)
		} else {
			m.histPos = min(m.histPos+1, len(m.history))
		}
		v := ""
		if m.histPos < len(m.history) {
			v = m.history[m.histPos]
		}
		m.filter.SetValue(v)
		m.filter.CursorEnd()
		s.search = v
		s.refreshView()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	s.search = m.filter.Value()
	s.refreshView()
	return m, cmd
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// keys that work on every screen, dashboard included
	switch k {
	case "q":
		return m, tea.Quit
	case "?":
		m.modal = newHelp(&m)
		return m, nil
	case "d", "0":
		return m, m.switchMode(0)
	}
	if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
		return m, m.switchMode(int(k[0] - '0')) // modes are 1-based like the shortcuts
	}

	s := m.cur()
	if s == nil {
		return m, nil
	}
	switch k {
	case "/", "f", "s":
		m.filtering = true
		m.histPos = len(m.history)
		m.filter.SetValue(s.search)
		m.filter.CursorEnd()
		m.layout()
		return m, m.filter.Focus()
	case "esc":
		switch {
		case s.search != "":
			s.search = ""
			m.filter.SetValue("")
			s.refreshView()
		case len(m.stacks[m.mode]) > 1:
			m.stacks[m.mode] = m.stacks[m.mode][:len(m.stacks[m.mode])-1]
		}
		return m, nil
	case "r":
		return m, tea.Batch(s.reload(), m.setStatus("refreshing..."))
	case "R": // back to the screen's defaults
		s.filters = cloneFilters(s.defaults)
		s.search = ""
		m.filter.SetValue("")
		s.applyDefaultSort()
		s.refreshView()
		return m, tea.Batch(s.reload(), m.setStatus("reset to defaults"))
	case "F":
		if len(s.res.Filters()) == 0 {
			return m, m.setStatus("no filters available for " + s.res.Name())
		}
		fm, cmd := newFiltersModal(s)
		m.modal = fm
		return m, cmd
	case "n", "shift+right":
		if s.nextToken == "" {
			return m, m.setStatus("no more pages")
		}
		s.loading = true
		return m, tea.Batch(s.load(s.nextToken, true), m.setStatus("fetching next page..."))
	case "enter":
		return m.open(s)
	case "S":
		return m.connect(s)
	case "up", "k":
		s.tbl.move(-1)
	case "down", "j":
		s.tbl.move(1)
	case "pgup", "ctrl+b":
		s.tbl.pageUp()
	case "pgdown", "ctrl+f":
		s.tbl.pageDown()
	case "home", "g":
		s.tbl.home()
	case "end", "G":
		s.tbl.end()
	case "left", "h":
		s.tbl.scrollLeft()
	case "right", "l":
		s.tbl.scrollRight()
	default:
		// k9s-style sorting: shift+letter sorts by that column, again reverses
		for i, sk := range s.sortKeys {
			if sk != "" && sk == k {
				s.sortBy(i)
				break
			}
		}
	}
	return m, nil
}

// open is enter: drill into the row if the resource supports it, else ssh.
func (m Model) open(s *screen) (tea.Model, tea.Cmd) {
	row, ok := s.selected()
	if !ok {
		if s.loading {
			return m, m.setStatus("Please wait! Data fetch is in progress!")
		}
		return m, nil
	}
	dr, ok := s.drillable()
	if !ok {
		return m.connect(s)
	}
	from := s.id
	return m, tea.Batch(m.setStatus("Opening "+strings.ToLower(dr.DrillHint())+"..."), func() tea.Msg {
		d, err := dr.Drill(context.Background(), row)
		return drilledMsg{from: from, drill: d, err: err}
	})
}

func (m Model) connect(s *screen) (tea.Model, tea.Cmd) {
	row, ok := s.selected()
	if !ok {
		if s.loading {
			return m, m.setStatus("Please wait! Data fetch is in progress!")
		}
		return m, m.setStatus("Instance not selected!")
	}
	host, ok := row.Ref.(provider.Host)
	if !ok {
		return m, m.setStatus("this resource cannot be connected to")
	}
	statusCmd := m.setStatus("resolving ssh user...")
	resolver, _ := m.prov.(provider.HostResolver)
	return m, tea.Batch(statusCmd, func() tea.Msg {
		if resolver == nil {
			return resolvedMsg{host: host}
		}
		h, err := resolver.ResolveHost(context.Background(), host)
		return resolvedMsg{host: h, err: err}
	})
}

// ---- view ----

func (m Model) infos() []info {
	out := []info{{"Cloud", m.prov.Name()}}
	if m.cfg.Profile != "" {
		out = append(out, info{"Profile", m.cfg.Profile})
	}
	if m.cfg.Region != "" {
		out = append(out, info{"Region", m.cfg.Region})
	}
	if s := m.cur(); s != nil {
		sum := s.filterSummary()
		out = append(out,
			info{"Filters", ansiTrunc(sum, min(50, max(m.width/3, 20)))},
			info{"Loaded", fmt.Sprintf("%d (%d shown)", len(s.rows), len(s.view))})
	}
	return out
}

func ansiTrunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return fit(s, w)
}

// hints lists every shortcut worth showing, most important first. The header
// flows them into as many columns as the terminal is wide, so nothing is
// capped here; narrow terminals simply drop the trailing columns.
func (m Model) hints() []hint {
	var h []hint
	if s := m.cur(); s != nil {
		if dr, ok := s.drillable(); ok {
			h = append(h, hint{"enter", dr.DrillHint()})
		}
		if s.sshable() {
			if _, drill := s.drillable(); drill {
				h = append(h, hint{"S", "SSH session"})
			} else {
				h = append(h, hint{"enter/S", "SSH session"})
			}
		}
		if len(s.res.Filters()) > 0 {
			h = append(h, hint{"F", "Update filters"})
		}
		h = append(h,
			hint{"/ f s", "Open search"},
			hint{"r", "Refresh data"},
			hint{"R", "Reset defaults"},
			hint{"n", "Next page"},
			hint{"shift+key", "Sort column (? lists keys)"},
			hint{"↑↓ j k", "Move"},
			hint{"pgup pgdn", "Page up / down"},
			hint{"g G", "Top / bottom"})
		if len(m.stacks[m.mode]) > 1 {
			h = append(h, hint{"esc", "Go back"})
		}
	}
	if len(m.resources) > 0 {
		h = append(h, hint{fmt.Sprintf("1-%d", min(len(m.resources), 9)), "Switch view"})
	}
	return append(h, hint{"d", "Dashboard"}, hint{"?", "Help"}, hint{"q", "Quit"})
}

func (m Model) crumbs() []string {
	if m.mode == 0 {
		return []string{"dashboard"}
	}
	var out []string
	for _, s := range m.stacks[m.mode] {
		out = append(out, s.res.Crumb())
	}
	return out
}

func (m Model) tableTitle(s *screen) string {
	search := "<all>"
	if s.search != "" {
		search = "</" + s.search + ">"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render(s.res.Name()) +
		lipgloss.NewStyle().Foreground(colAccent).Render(fmt.Sprintf("[%d]", len(s.view))) +
		lipgloss.NewStyle().Foreground(colSecondary).Render(search)
}

func emptyMessage(s *screen) string {
	if s.loading {
		return "Loading..."
	}
	return "No " + strings.ToLower(s.res.Name()) + " found!"
}

func (m Model) footer(s *screen) string {
	crumbs := m.crumbs()
	left := make([]string, len(crumbs))
	for i, c := range crumbs {
		st := crumbStyle
		if i == len(crumbs)-1 {
			st = crumbActive
		}
		left[i] = st.Render("<" + c + ">")
	}
	l := strings.Join(left, " ")

	r := hintKeyStyle.Render("<?>") + hintDescStyle.Render(" Help")
	if s != nil {
		pages := hintKeyStyle.Render("...")
		if !s.loading {
			n := fmt.Sprintf("%d", s.pages)
			if s.nextToken != "" {
				n += "+"
			}
			pages = hintKeyStyle.Render(n)
		}
		r = "Pages: " + pages + "   " + r
	}
	gap := max(m.width-lipgloss.Width(l)-lipgloss.Width(r), 1)
	line1 := fit(l+strings.Repeat(" ", gap)+r, m.width)

	st := statusStyle
	if strings.HasPrefix(m.status, "error") {
		st = errorStyle
	}
	return line1 + "\n" + st.Render(center(m.status, m.width))
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.splashing {
		return splashView(m.width, m.height, m.splashAt)
	}
	s := m.cur()
	parts := []string{renderHeader(m.infos(), m.hints(), m.width)}
	if m.filtering {
		parts = append(parts, panel("", []string{" " + m.filter.View()}, m.width, filterHeight))
	}
	th := m.tableH()
	switch {
	case m.modal != nil:
		parts = append(parts, m.modal.view(m.width, th))
	case s == nil:
		title := lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render("Dashboard")
		parts = append(parts, panel(title, dashboardLines(m.resources, m.width-2, th-2), m.width, th))
	default:
		parts = append(parts, s.tbl.render(m.tableTitle(s), emptyMessage(s)))
	}
	parts = append(parts, m.footer(s))
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	if len(lines) > m.height { // tiny terminal: sacrifice the header, keep table and footer
		lines = lines[len(lines)-m.height:]
	}
	return strings.Join(lines, "\n")
}

// Package ui is the Bubble Tea front end. It only knows provider interfaces.
package ui

import (
	"context"
	"fmt"
	"os/exec"
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
	host    provider.Host
	tunnels []sshx.LocalTunnel // already-running tunnels that lead to host
	err     error
}

// tempKeyMsg is the result of pushing a temporary key through the cloud:
// on success the model runs ssh with that key and deletes it afterwards.
type tempKeyMsg struct {
	target sshx.Target
	key    sshx.TempKey
	err    error
}

// authDoneMsg is the outcome of switching (or retrying) the sign-in profile.
type authDoneMsg struct {
	profile string
	err     error
}

// enrichedMsg is the result of fetching one partial row's details.
type enrichedMsg struct {
	sid, gen int
	id       string
	row      provider.Row
	err      error
}

// prefetchMsg asks a screen to load its next page in the background.
type prefetchMsg struct{ sid, gen int }

type (
	sshDoneMsg     struct{ err error }
	clearStatusMsg struct{ seq int }
	splashMsg      struct{ step int }
)

// Model is the root Bubble Tea model. Each mode (0 = dashboard, 1..n = the
// provider's resources) keeps its own stack of screens, so switching views
// preserves where you were.
type Model struct {
	cfg       config.Config
	prov      provider.Provider
	resources []provider.Resource
	stacks    [][]*screen
	forward   [][]*screen // per mode: screens left with [ that ] can bring back
	views     []int       // views visited, oldest first; { and } walk it like browser back/forward
	viewPos   int
	mode      int
	nextID    int

	filter    textinput.Model // shared search input
	filtering bool
	history   []string
	histPos   int

	modal modal
	// detectTunnels lists tunnels the user already runs (replaceable in tests).
	detectTunnels func(context.Context) []sshx.LocalTunnel
	splashing     bool
	splashAt      int

	// Sign-in. auth is nil for providers without profiles. While authErr is
	// set the app shows the profiles screen instead of the resources.
	auth        provider.Authenticator
	authErr     error
	prevMode    int  // where to return after the profiles screen
	hasProfiles bool // profiles exist on this machine (shows the p shortcut)
	demo        bool // sample data: mark the header, never start a real ssh

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
		splashing: !cfg.SkipSplash, nextID: 1, detectTunnels: sshx.DetectTunnels,
	}
	m.stacks = make([][]*screen, len(m.resources)+2) // 0 dashboard, 1..n resources, n+1 profiles
	m.forward = make([][]*screen, len(m.resources)+2)
	m.mode = m.startMode()
	if d, ok := p.(provider.Demoer); ok && d.Demo() {
		m.demo, m.detectTunnels = true, nil // nothing real to tunnel to
	}
	if a, ok := p.(provider.Authenticator); ok {
		m.auth, m.authErr = a, a.AuthError()
		m.hasProfiles = len(a.Profiles()) > 0
	}
	if m.authErr != nil {
		// Start on the profiles screen: say what is missing and offer the way forward.
		m.prevMode, m.mode = m.mode, m.profilesMode()
		m.stacks[m.mode] = []*screen{m.newScreen(&profilesResource{auth: m.auth}, nil)}
		m.stacks[m.mode][0].focusActive = true
	} else if m.mode > 0 {
		m.stacks[m.mode] = []*screen{m.newScreen(m.resources[m.mode-1], nil)}
	}
	return m
}

// profilesMode is the extra mode holding the profiles screen.
func (m Model) profilesMode() int { return len(m.resources) + 1 }

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
	if s := m.cur(); s != nil { // the profiles screen when signed out, else the start view
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
	if i < 0 || i > len(m.resources) || i == m.mode || m.authErr != nil {
		return nil // the profiles screen has its own key (p); nothing opens while signed out
	}
	m.noteView(i)
	return m.enterMode(i)
}

// noteView records a deliberate move to view i. Anything "forward" of the
// current position is dropped, as in a browser.
func (m *Model) noteView(i int) {
	if len(m.views) == 0 || m.views[m.viewPos] != m.mode {
		m.views = append(m.views[:min(m.viewPos+1, len(m.views))], m.mode)
		m.viewPos = len(m.views) - 1
	}
	m.views = append(m.views[:m.viewPos+1], i)
	m.viewPos++
}

// stepView walks the visited views: d=-1 back, +1 forward.
func (m *Model) stepView(d int) tea.Cmd {
	if m.authErr != nil || m.mode > len(m.resources) {
		return nil
	}
	if len(m.views) == 0 || m.views[m.viewPos] != m.mode { // not where the list thinks: start it here
		m.views, m.viewPos = []int{m.mode}, 0
	}
	np := m.viewPos + d
	if np < 0 || np >= len(m.views) {
		if d < 0 {
			return m.setStatus("no earlier view")
		}
		return m.setStatus("no later view")
	}
	m.viewPos = np
	return m.enterMode(m.views[np])
}

func (m *Model) enterMode(i int) tea.Cmd {
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

// openMode shows mode i, creating and loading its first screen if needed.
func (m *Model) openMode(i int) tea.Cmd {
	m.mode = i
	m.filtering = false
	m.layout()
	if i > 0 && i <= len(m.resources) && len(m.stacks[i]) == 0 {
		s := m.newScreen(m.resources[i-1], nil)
		m.stacks[i] = []*screen{s}
		return s.load("", false)
	}
	return nil
}

// showProfiles opens the profiles screen, remembering where to come back to.
func (m *Model) showProfiles() tea.Cmd {
	if m.mode != m.profilesMode() {
		m.prevMode = m.mode
	}
	m.mode, m.filtering = m.profilesMode(), false
	if len(m.stacks[m.mode]) == 0 {
		m.stacks[m.mode] = []*screen{m.newScreen(&profilesResource{auth: m.auth}, nil)}
	}
	m.layout()
	s := m.cur()
	s.focusActive = true
	return s.reload() // pick up profiles added since the last visit
}

// leaveProfiles returns to the screen the user came from.
func (m *Model) leaveProfiles() tea.Cmd {
	back := m.prevMode
	if back < 0 || back > len(m.resources) {
		back = m.startMode()
	}
	return m.openMode(back)
}

// useProfile switches the whole app to name ("" = default credential chain).
func (m *Model) useProfile(name string) tea.Cmd {
	auth := m.auth
	label := name
	if label == "" {
		label = "the default credentials"
	}
	return tea.Batch(m.setStatus("Signing in with "+label+"..."), func() tea.Msg {
		return authDoneMsg{profile: name, err: auth.UseProfile(context.Background(), name)}
	})
}

// command is the process to run for a connection: real ssh, or in demo mode a
// harmless local shell.
func (m *Model) command(t sshx.Target) *exec.Cmd {
	if m.demo {
		return t.DemoCommand()
	}
	return t.Command()
}

// prefetchTick schedules the next background page, if the screen may load more.
func (m Model) prefetchTick(s *screen) tea.Cmd {
	if s.budget <= 0 || s.nextToken == "" {
		return nil
	}
	sid, gen := s.id, s.gen
	return tea.Tick(time.Duration(m.cfg.PrefetchDelayMs)*time.Millisecond, func(time.Time) tea.Msg { return prefetchMsg{sid, gen} })
}

// maxEnrichInFlight bounds concurrent detail requests, so scrolling a long
// list cannot flood the cloud API.
const maxEnrichInFlight = 4

// enrichVisible starts detail requests for the partial rows now on screen.
func (m Model) enrichVisible() tea.Cmd {
	s := m.cur()
	if s == nil || isProfiles(s) {
		return nil
	}
	en, ok := s.res.(provider.Enricher)
	if !ok {
		return nil
	}
	var cmds []tea.Cmd
	for i := s.tbl.top; i < min(s.tbl.top+s.tbl.rowsH(), len(s.view)); i++ {
		r := s.view[i]
		if !r.Partial || r.ID == "" || s.enriching[r.ID] || len(s.enriching) >= maxEnrichInFlight {
			continue
		}
		if s.enriching == nil {
			s.enriching = map[string]bool{}
		}
		s.enriching[r.ID] = true
		sid, gen, row := s.id, s.gen, r
		cmds = append(cmds, func() tea.Msg {
			out, err := en.Enrich(context.Background(), row)
			return enrichedMsg{sid: sid, gen: gen, id: row.ID, row: out, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// Update runs the message through the model, then starts any detail requests
// the new screen state needs (rows scrolled into view, new pages, ...).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm := next.(Model)
	if e := nm.enrichVisible(); e != nil {
		return nm, tea.Batch(cmd, e)
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if isProfiles(s) {
			m.hasProfiles = len(s.rows) > 0
			if s.focusActive {
				s.focusActive = false
				for i, r := range s.view {
					if len(r.Cells) > 0 && r.Cells[0] == "●" {
						s.tbl.cursor = i
						s.tbl.ensureVisible()
						break
					}
				}
			}
			return m, nil
		}
		if !msg.append {
			s.budget = m.cfg.Prefetch // a fresh list: reset the background budget
		}
		next := m.prefetchTick(s)
		if m.cur() != s {
			return m, next
		}
		if next != nil { // more pages are coming on their own: no status chatter
			return m, next
		}
		if s.nextToken == "" {
			return m, m.setStatus(fmt.Sprintf("All %d loaded", len(s.rows)))
		}
		return m, m.setStatus(fmt.Sprintf("%d loaded. Press n for the next page", len(s.rows)))

	case enrichedMsg:
		s := m.find(msg.sid)
		if s == nil || msg.gen != s.gen {
			return m, nil // refreshed or gone
		}
		delete(s.enriching, msg.id)
		var cmd tea.Cmd
		for i := range s.rows {
			if s.rows[i].ID != msg.id {
				continue
			}
			if msg.err != nil { // show that it failed, once, instead of retrying forever
				for k := 1; k < len(s.rows[i].Cells); k++ {
					s.rows[i].Cells[k] = "?"
				}
				s.rows[i].Partial = false
				cmd = m.setStatus("error: details for " + msg.id + ": " + msg.err.Error())
			} else {
				s.rows[i] = msg.row
			}
			break
		}
		s.refreshView()
		return m, cmd

	case prefetchMsg:
		s := m.find(msg.sid)
		if s == nil || msg.gen != s.gen || s.loading || s.budget <= 0 || s.nextToken == "" {
			return m, nil // refreshed, gone, or something else is already loading
		}
		s.budget--
		s.loading = true
		return m, s.load(s.nextToken, true)

	case authDoneMsg:
		m.authErr = msg.err
		if msg.err != nil {
			// Stay on the profiles screen; the footer explains, the list can be tried again.
			var cmd tea.Cmd
			if s := m.cur(); s != nil && isProfiles(s) {
				cmd = s.reload()
			}
			return m, tea.Batch(m.setStatus(""), cmd)
		}
		// Signed in (possibly as someone else): drop everything loaded for the old account.
		for i := 1; i <= len(m.resources); i++ {
			m.stacks[i], m.forward[i] = nil, nil
		}
		m.hasProfiles = len(m.auth.Profiles()) > 0
		back := m.prevMode
		if back < 0 || back > len(m.resources) {
			back = m.startMode()
		}
		cmds := []tea.Cmd{m.openMode(back), m.setStatus("Signed in: " + m.auth.Access())}
		if s := m.stacks[m.profilesMode()]; len(s) > 0 {
			s[0].focusActive = true
			cmds = append(cmds, s[0].reload()) // refresh the ● marker for next time
		}
		return m, tea.Batch(cmds...)

	case drilledMsg:
		if msg.err != nil {
			return m, m.setStatus("error: " + msg.err.Error())
		}
		for mode, st := range m.stacks {
			if n := len(st); n > 0 && st[n-1].id == msg.from { // only if still on top
				s := m.newScreen(msg.drill.Resource, msg.drill.Filters)
				m.stacks[mode] = append(st, s)
				m.forward[mode] = nil // a new drill replaces whatever ] would have returned to
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
		sm, cmd := newSSHModal(msg.host, m.cfg, auth, msg.tunnels)
		m.modal = sm
		return m, cmd

	case tempKeyMsg:
		if msg.err != nil {
			msg.key.Cleanup()
			return m, m.setStatus("error: " + msg.err.Error())
		}
		key := msg.key
		return m, execSession(m.command(msg.target), func(err error) tea.Msg {
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

// goBack leaves the top screen of this view, remembering it for goForward.
func (m *Model) goBack() {
	st := m.stacks[m.mode]
	m.forward[m.mode] = append(m.forward[m.mode], st[len(st)-1])
	m.stacks[m.mode] = st[:len(st)-1]
}

// goForward returns to the screen goBack left, reloading it if its data was
// still on its way when it was left.
func (m *Model) goForward() (tea.Cmd, bool) {
	fw := m.forward[m.mode]
	if len(fw) == 0 {
		return nil, false
	}
	s := fw[len(fw)-1]
	m.forward[m.mode] = fw[:len(fw)-1]
	m.stacks[m.mode] = append(m.stacks[m.mode], s)
	if s.loading {
		return s.reload(), true
	}
	return nil, true
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
	case "p":
		return m.toggleProfiles()
	}
	if k == "d" || k == "0" || (len(k) == 1 && k[0] >= '1' && k[0] <= '9') {
		if m.authErr != nil {
			cmd := m.setStatus("Choose a profile first (enter), or press r to try again")
			return m, cmd
		}
		mode := 0
		if k != "d" { // modes are 1-based like the shortcuts
			mode = int(k[0] - '0')
		}
		cmd := m.switchMode(mode)
		return m, cmd
	}

	switch k {
	case "{", "}": // back / forward through the views visited
		cmd := m.stepView(map[string]int{"{": -1, "}": 1}[k])
		return m, cmd
	case "[":
		if len(m.stacks[m.mode]) < 2 {
			return m, m.setStatus("nothing to go back to")
		}
		m.goBack()
		return m, nil
	case "]":
		cmd, ok := m.goForward()
		if !ok {
			return m, m.setStatus("nothing to go forward to")
		}
		return m, cmd
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
		focus := m.filter.Focus()
		if s.nextToken != "" && m.cfg.SearchPrefetch > 0 { // opening search loads more pages so it sees more
			s.budget = max(s.budget, m.cfg.SearchPrefetch)
			if !s.loading {
				return m, tea.Batch(focus, m.prefetchTick(s))
			}
		}
		return m, focus
	case "esc":
		switch {
		case s.search != "":
			s.search = ""
			m.filter.SetValue("")
			s.refreshView()
		case len(m.stacks[m.mode]) > 1:
			m.goBack()
		case m.mode == m.profilesMode() && m.authErr == nil:
			cmd := m.leaveProfiles()
			return m, cmd
		}
		return m, nil
	case "r":
		if isProfiles(s) && m.authErr != nil {
			// e.g. the user logged in from another terminal: try again
			retry := m.useProfile(m.auth.Profile())
			return m, tea.Batch(s.reload(), retry)
		}
		reload, status := s.reload(), m.setStatus("refreshing...")
		return m, tea.Batch(reload, status)
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
	if isProfiles(s) {
		return m.chooseProfile(s)
	}
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

// toggleProfiles is the p key: open the profiles screen, or go back from it.
func (m Model) toggleProfiles() (tea.Model, tea.Cmd) {
	switch {
	case m.auth == nil:
		cmd := m.setStatus("This cloud has no profiles to switch between")
		return m, cmd
	case m.mode != m.profilesMode():
		cmd := m.showProfiles()
		return m, cmd
	case m.authErr != nil:
		cmd := m.setStatus("Pick a profile to continue (enter), or press r to try again")
		return m, cmd
	}
	cmd := m.leaveProfiles()
	return m, cmd
}

// chooseProfile is enter on the profiles screen.
func (m Model) chooseProfile(s *screen) (tea.Model, tea.Cmd) {
	row, ok := s.selected()
	if !ok {
		return m, nil
	}
	ref, ok := row.Ref.(profileRef)
	if !ok {
		return m, nil
	}
	if m.authErr == nil && ref.Name == m.auth.Profile() { // already using it
		cmd := m.leaveProfiles()
		return m, cmd
	}
	cmd := m.useProfile(ref.Name)
	return m, cmd
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
	detect := m.detectTunnels
	return m, tea.Batch(statusCmd, func() tea.Msg {
		ctx := context.Background()
		h := host
		if resolver != nil {
			var err error
			if h, err = resolver.ResolveHost(ctx, host); err != nil {
				return resolvedMsg{host: h, err: err}
			}
		}
		var tunnels []sshx.LocalTunnel
		if detect != nil {
			tunnels = sshx.TunnelsFor(detect(ctx), h)
		}
		return resolvedMsg{host: h, tunnels: tunnels}
	})
}

// ---- view ----

// infos is the header's left block: where we are connected and how, plus any
// active filter. Row counts are in the table title, so they are not repeated.
func (m Model) infos() []info {
	cloudName := m.prov.Name()
	region := m.cfg.Region
	if m.auth != nil && m.authErr == nil {
		region = m.auth.Region()
	}
	if region != "" {
		cloudName += " · " + region
	}
	if m.demo {
		cloudName += " · demo" // plain: the user asked for it
	}
	out := []info{{"Cloud", cloudName}}
	switch {
	case m.auth != nil && m.authErr != nil:
		out = append(out, info{"Auth", "not signed in"})
	case m.auth != nil:
		id := m.auth.Identity()
		if id.Account != "" { // which account we are acting in, by its friendly name when it has one
			acct := id.Account
			if id.Alias != "" {
				acct = id.Alias + " (" + id.Account + ")"
			}
			out = append(out, info{"Account", acct})
		}
		access := m.auth.Access()
		if id.Principal != "" {
			access += " · " + id.Principal
		}
		out = append(out, info{"Auth", access})
	case m.cfg.Profile != "":
		out = append(out, info{"Profile", m.cfg.Profile})
	}
	if s := m.cur(); s != nil && !isProfiles(s) {
		if chips := s.filterChips(min(48, max(m.width/3, 16))); chips != "" {
			out = append(out, info{"Filter", chips})
		}
	}
	return out
}

// hints lists the action shortcuts shown in the header. Navigation, sorting
// and drill-down keys are deliberately left out: they are discoverable from
// the help screen (?), which keeps the header short and relevant. The header
// flows hints into as many columns as the terminal fits.
func (m Model) hints() []hint {
	help := hint{"?", "Help"} // first, so it is always in the first column
	if s := m.cur(); s != nil && isProfiles(s) {
		h := []hint{help, {"enter", "Use profile"}, {"/ f s", "Open search"}}
		if m.authErr != nil {
			h = append(h, hint{"r", "Try again"})
		} else {
			h = append(h, hint{"r", "Refresh"}, hint{"esc  p", "Back"})
		}
		return append(h, hint{"q", "Quit"})
	}
	h := []hint{help}
	if s := m.cur(); s != nil {
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
			hint{"n", "Next page"})
		if len(m.stacks[m.mode]) > 1 {
			h = append(h, hint{"esc", "Go back"})
		}
		if len(m.forward[m.mode]) > 0 {
			h = append(h, hint{"]", "Go forward"})
		}
	}
	if len(m.resources) > 1 {
		h = append(h, hint{fmt.Sprintf("1-%d", min(len(m.resources), 9)), "Switch view"})
	}
	if m.auth != nil && m.hasProfiles {
		h = append(h, hint{"p", "Profiles"})
	}
	return append(h, hint{"d", "Dashboard"}, hint{"q", "Quit"})
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
	count := fmt.Sprintf("[%d]", len(s.view))
	if s.nextToken != "" {
		count = fmt.Sprintf("[%d+]", len(s.view)) // a lower bound: more pages exist
	}
	search := "<all>"
	if s.search != "" {
		search = "</" + s.search + ">"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render(s.res.Name()) +
		lipgloss.NewStyle().Foreground(colAccent).Render(count) +
		lipgloss.NewStyle().Foreground(colSecondary).Render(search)
}

func (m Model) emptyMessage(s *screen) string {
	if s.loading {
		return "Loading..."
	}
	if isProfiles(s) {
		return m.profilesEmpty()
	}
	return "No " + strings.ToLower(s.res.Name()) + " found!"
}

// profilesEmpty is the friendly empty state of the profiles screen: short,
// wrapped to the screen, and without the raw SDK error.
func (m Model) profilesEmpty() string {
	width := max(min(m.width-8, 72), 20)
	var out []string
	if m.authErr != nil {
		for _, l := range strings.Split(m.authErr.Error(), "\n") {
			if !strings.HasPrefix(l, "details:") {
				out = append(out, wrap(l, width)...)
			}
		}
		out = append(out, "", "No profiles here yet, which is normal. Set one up, then press r.")
	} else {
		out = append(out, "No profiles are configured, and none are needed:", "you are signed in with "+m.auth.Access()+".")
	}
	var wrapped []string
	for _, l := range out {
		wrapped = append(wrapped, wrap(l, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func (m Model) footer(s *screen) string {
	if s != nil && isProfiles(s) {
		s = nil // no page counter on the profiles screen
	}
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

	r := ""
	if s != nil {
		// "..." only while the first page is on its way; later background
		// pages just raise the count
		pages := hintKeyStyle.Render("...")
		if s.pages > 0 || !s.loading {
			n := fmt.Sprintf("%d", s.pages)
			if s.nextToken != "" {
				n += "+"
			}
			pages = hintKeyStyle.Render(n)
		}
		r = "Pages: " + pages
		if s.pages > 0 && s.nextToken == "" && !s.loading {
			r += " " + lipgloss.NewStyle().Foreground(colOK).Render("· all loaded")
		}
	}
	gap := max(m.width-lipgloss.Width(l)-lipgloss.Width(r), 1)
	line1 := fit(l+strings.Repeat(" ", gap)+r, m.width)

	st, msg := statusStyle, m.status
	switch {
	case strings.HasPrefix(msg, "error"):
		st = errorStyle
	case s != nil && s.search != "" && s.nextToken != "": // outranks transient load messages, never errors
		// search only looks at rows already loaded; say so instead of leaving a "missing" match unexplained
		if s.loading || s.budget > 0 {
			st, msg = warnStyle, fmt.Sprintf("Searching %d rows, loading more pages...", len(s.rows))
		} else {
			st, msg = warnStyle, fmt.Sprintf("Searched %d rows. Press n to load more.", len(s.rows))
		}
	case msg == "" && m.authErr != nil:
		st, msg = warnStyle, authHeadline(m.authErr) // stays until signed in
		if m.hasProfiles {
			msg += "  Pick a profile (enter), or press r to try again."
		}
	}
	return line1 + "\n" + st.Render(center(msg, m.width))
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
		parts = append(parts, panel(title, dashboardLines(m.resources, m.auth != nil && m.hasProfiles, m.width-2, th-2), m.width, th))
	default:
		parts = append(parts, s.tbl.render(m.tableTitle(s), m.emptyMessage(s)))
	}
	parts = append(parts, m.footer(s))
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	if len(lines) > m.height { // tiny terminal: sacrifice the header, keep table and footer
		lines = lines[len(lines)-m.height:]
	}
	return strings.Join(lines, "\n")
}

package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// bootDoneMsg carries the opened provider (or why it could not be opened).
type bootDoneMsg struct {
	prov provider.Provider
	err  error
}

// bootHeldMsg ends the minimum time the logo stays up.
type bootHeldMsg struct{}

// Boot is what the program runs first. Opening a cloud can take seconds (token
// refresh, SSO, identity lookups), so it happens in the background while the
// splash is already on screen; when both the provider and the minimum hold are
// done, Boot becomes the real Model and the logo fades out into it.
type Boot struct {
	cfg  config.Config
	open func() (provider.Provider, error)

	w, h  int
	ready *bootDoneMsg
	held  bool
	inner tea.Model // the real Model, once started
	err   error
}

// NewBoot returns the starting model. open is called once, off the UI thread.
func NewBoot(cfg config.Config, open func() (provider.Provider, error)) Boot {
	return Boot{cfg: cfg, open: open, held: cfg.SkipSplash}
}

// Err is why the provider could not be opened, nil otherwise.
func (b Boot) Err() error { return b.err }

func (b Boot) Init() tea.Cmd {
	cmds := []tea.Cmd{func() tea.Msg {
		p, err := b.open()
		return bootDoneMsg{p, err}
	}}
	if !b.held {
		cmds = append(cmds, tea.Tick(splashHold, func(time.Time) tea.Msg { return bootHeldMsg{} }))
	}
	return tea.Batch(cmds...)
}

func (b Boot) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if b.inner != nil {
		var cmd tea.Cmd
		b.inner, cmd = b.inner.Update(msg)
		return b, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.w, b.h = msg.Width, msg.Height
	case bootDoneMsg:
		b.ready = &msg
	case bootHeldMsg:
		b.held = true
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}
		b.held = true // any key skips the wait for the logo
	}
	return b.maybeStart()
}

// maybeStart swaps in the real Model once the provider is open and the logo
// has been shown long enough.
func (b Boot) maybeStart() (tea.Model, tea.Cmd) {
	if b.ready == nil || !b.held {
		return b, nil
	}
	if b.ready.err != nil {
		b.err = b.ready.err
		return b, tea.Quit
	}
	m := New(b.cfg, b.ready.prov)
	m.splashHeld = true // the hold already happened: go straight to the fade
	var sized tea.Model = m
	var cmd tea.Cmd
	if b.w > 0 {
		sized, cmd = m.Update(tea.WindowSizeMsg{Width: b.w, Height: b.h})
	}
	b.inner = sized
	return b, tea.Batch(cmd, sized.Init())
}

func (b Boot) View() string {
	if b.inner != nil {
		return b.inner.View()
	}
	if b.cfg.SkipSplash || b.w == 0 {
		return ""
	}
	lines := strings.Split(splashView(b.w, b.h, 0), "\n")
	if n := len(lines); n > 0 && b.held && b.ready == nil { // the minimum hold is over and we are still waiting
		lines[n-1] = lipgloss.PlaceHorizontal(b.w, lipgloss.Center, hintDescStyle.Render("signing in..."))
	}
	return strings.Join(lines, "\n")
}

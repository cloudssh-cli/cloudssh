package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudssh-cli/cloudssh/internal/config"
	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// bootDoneMsg carries the opened provider (or why it could not be opened).
type bootDoneMsg struct {
	prov provider.Provider
	err  error
}

// bootHeldMsg ends the minimum time the logo stays up with its "loading" caption.
type bootHeldMsg struct{}

// bootShownMsg ends the short time the "signed in" caption stays readable.
type bootShownMsg struct{}

const bootShown = 200 * time.Millisecond

// Boot is what the program runs first. Opening a cloud can take seconds (token
// refresh, SSO, identity lookups), so it happens in the background while the
// splash is already on screen; when both the provider and the minimum hold are
// done, Boot becomes the real Model and the logo fades out into it.
type Boot struct {
	cfg  config.Config
	open func() (provider.Provider, error)
	demo bool

	w, h  int
	ready *bootDoneMsg
	held  bool      // the logo has had its minimum time
	shown bool      // the "signed in" caption has been readable for a moment
	armed bool      // the timer for shown is running
	inner tea.Model // the real Model, once started
	err   error
}

// NewBoot returns the starting model. open is called once, off the UI thread.
// demo only changes the wording of the caption.
func NewBoot(cfg config.Config, open func() (provider.Provider, error), demo bool) Boot {
	return Boot{cfg: cfg, open: open, demo: demo, held: cfg.SkipSplash, shown: cfg.SkipSplash}
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
	case bootShownMsg:
		b.shown = true
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}
		b.held, b.shown = true, true // any key skips the waits for the logo
	}
	if b.ready != nil && b.ready.err == nil && b.held && !b.shown && !b.armed {
		b.armed = true // let "signed in" be read before the fade
		return b, tea.Tick(bootShown, func(time.Time) tea.Msg { return bootShownMsg{} })
	}
	return b.maybeStart()
}

// caption is the line under the logo: what is happening, then how it went.
func (b Boot) caption() string {
	switch {
	case b.ready == nil || !b.held:
		if b.demo {
			return "loading demo data..."
		}
		return "signing in..."
	case b.ready.err != nil:
		return ""
	}
	if b.demo {
		return "✓ demo data loaded"
	}
	if a, ok := b.ready.prov.(provider.Authenticator); ok {
		if a.AuthError() != nil {
			return "not signed in"
		}
		if acc := a.Access(); acc != "" {
			return "✓ signed in · " + acc
		}
	}
	return "✓ ready"
}

// maybeStart swaps in the real Model once the provider is open and the logo
// has been shown long enough.
func (b Boot) maybeStart() (tea.Model, tea.Cmd) {
	if b.ready != nil && b.ready.err != nil { // nothing to show: report it now
		b.err = b.ready.err
		return b, tea.Quit
	}
	if b.ready == nil || !b.held || !b.shown {
		return b, nil
	}
	m := New(b.cfg, b.ready.prov)
	m.splashHeld = true // the hold already happened: go straight to the fade
	m.splashNote = b.caption()
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
	return splashView(b.w, b.h, 0, b.caption())
}

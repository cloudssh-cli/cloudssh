package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func bootWith(open func() (provider.Provider, error)) Boot {
	cfg := testCfg()
	cfg.SkipSplash = false
	b := NewBoot(cfg, open, false)
	next, _ := b.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return next.(Boot)
}

func bootSend(b Boot, msg tea.Msg) (Boot, tea.Cmd) {
	next, cmd := b.Update(msg)
	return next.(Boot), cmd
}

func fakeOpen() (provider.Provider, error) {
	return fakeProvider{[]provider.Resource{threeRows()}}, nil
}

// The logo is on screen while the provider is still opening, and only
// becomes the real UI when the provider, the hold and the "signed in" moment are done.
func TestBootShowsLogoWhileSigningIn(t *testing.T) {
	b := bootWith(fakeOpen)
	if v := b.View(); !strings.Contains(v, "/ ____/") || !strings.Contains(v, "signing in...") {
		t.Fatalf("logo and caption must show before the provider is open:\n%s", v)
	}
	b, _ = bootSend(b, bootHeldMsg{}) // hold over, provider still opening
	if b.inner != nil || !strings.Contains(b.View(), "signing in...") {
		t.Fatalf("must keep waiting and say so:\n%s", b.View())
	}
	b, cmd := bootSend(b, bootDoneMsg{prov: fakeProvider{[]provider.Resource{threeRows()}}})
	if b.inner != nil || cmd == nil || !strings.Contains(b.View(), "✓ ready") {
		t.Fatalf("signed in: say so for a moment before the fade:\n%s", b.View())
	}
	b, cmd = bootSend(b, bootShownMsg{})
	if b.inner == nil || cmd == nil {
		t.Fatal("then the real UI must start")
	}
	m := b.inner.(Model)
	if !m.splashing || !m.splashHeld || m.width != 120 || m.splashNote != "✓ ready" {
		t.Fatalf("the fade must follow at once, sized, keeping the caption: %+v", m.splashNote)
	}
}

func TestBootCaptionsDemoAndSignedIn(t *testing.T) {
	d := NewBoot(testCfg(), fakeOpen, true)
	if d.caption() != "loading demo data..." {
		t.Fatalf("demo caption: %q", d.caption())
	}
	d.held, d.ready = true, &bootDoneMsg{prov: fakeProvider{}}
	if d.caption() != "✓ demo data loaded" {
		t.Fatalf("demo done: %q", d.caption())
	}
	s := NewBoot(testCfg(), fakeOpen, false)
	s.held, s.ready = true, &bootDoneMsg{prov: authedProvider{access: "SSO (prod)"}}
	if s.caption() != "✓ signed in · SSO (prod)" {
		t.Fatalf("signed in: %q", s.caption())
	}
	s.ready = &bootDoneMsg{prov: authedProvider{err: errors.New("no creds")}}
	if s.caption() != "not signed in" {
		t.Fatalf("signed out: %q", s.caption())
	}
}

func TestBootWaitsForTheHoldEvenWhenTheProviderIsFast(t *testing.T) {
	b := bootWith(fakeOpen)
	b, _ = bootSend(b, bootDoneMsg{prov: fakeProvider{}})
	if b.inner != nil || !strings.Contains(b.View(), "signing in...") {
		t.Fatal("the logo keeps its loading caption for its minimum time")
	}
	b, _ = bootSend(b, bootHeldMsg{})
	if b, _ = bootSend(b, bootShownMsg{}); b.inner == nil {
		t.Fatal("then it starts")
	}
}

func TestBootKeySkipsTheHoldButNotTheSignIn(t *testing.T) {
	b := bootWith(fakeOpen)
	b, _ = bootSend(b, key("x"))
	if b.inner != nil {
		t.Fatal("a key cannot skip the sign-in")
	}
	if b, _ = bootSend(b, bootDoneMsg{prov: fakeProvider{}}); b.inner == nil {
		t.Fatal("provider ready after the key: start")
	}
}

func TestBootOpenErrorEndsTheProgram(t *testing.T) {
	b := bootWith(fakeOpen)
	b, _ = bootSend(b, bootHeldMsg{})
	b, cmd := bootSend(b, bootDoneMsg{err: errors.New("cloud nope is not supported")})
	if b.Err() == nil || cmd == nil {
		t.Fatal("an open error must stop the program and be reported")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected quit")
	}
}

func TestBootCtrlCQuitsWhileWaiting(t *testing.T) {
	_, cmd := bootSend(bootWith(fakeOpen), key("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected quit")
	}
}

func TestBootInitOpensInTheBackground(t *testing.T) {
	called := make(chan struct{}, 1)
	b := bootWith(func() (provider.Provider, error) { called <- struct{}{}; return fakeProvider{}, nil })
	msgs := run(b.Init())
	var done bool
	for _, m := range msgs {
		_, ok := m.(bootDoneMsg)
		done = done || ok
	}
	if !done {
		t.Fatalf("Init must open the provider and report back: %v", msgs)
	}
	<-called
}

func TestBootSkipSplashShowsNothingAndStartsAtOnce(t *testing.T) {
	cfg := testCfg() // SkipSplash
	b := NewBoot(cfg, fakeOpen, false)
	b, _ = bootSend(b, tea.WindowSizeMsg{Width: 100, Height: 30})
	if b.View() != "" {
		t.Fatal("no logo when the splash is skipped")
	}
	if b, _ = bootSend(b, bootDoneMsg{prov: fakeProvider{}}); b.inner == nil {
		t.Fatal("no hold to wait for")
	}
}

func TestLoadingIndicatorAnimatesOnlyDuringTheFirstLoad(t *testing.T) {
	m := New(testCfg(), fakeProvider{[]provider.Resource{threeRows()}})
	m, cmd := send(sized(m, 120, 30), spinMsg{})
	if cmd == nil || !m.spinning {
		t.Fatal("a screen waiting for its first page must keep animating")
	}
	first := m.View()
	m, _ = send(m, spinMsg{})
	if m.View() == first || !strings.Contains(m.View(), "Loading") {
		t.Fatalf("the frame must advance:\n%s", m.View())
	}
	m, _ = send(m, loadedMsg{sid: m.cur().id, gen: m.cur().gen, page: provider.Page{Rows: rows("a")}})
	if strings.Contains(m.View(), "Loading") {
		t.Fatal("the indicator must go once data arrives")
	}
	if m, _ = send(m, spinMsg{}); m.spinning {
		t.Fatal("no more ticks once loaded")
	}
}

// authedProvider is a provider that can sign in (or not).
type authedProvider struct {
	fakeProvider
	access string
	err    error
}

func (a authedProvider) AuthError() error                         { return a.err }
func (a authedProvider) Access() string                           { return a.access }
func (a authedProvider) Region() string                           { return "" }
func (a authedProvider) Identity() provider.Identity              { return provider.Identity{} }
func (a authedProvider) Profile() string                          { return "" }
func (a authedProvider) Profiles() []provider.ProfileInfo         { return nil }
func (a authedProvider) UseProfile(context.Context, string) error { return a.err }

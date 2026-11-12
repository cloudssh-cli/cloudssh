package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func bootWith(open func() (provider.Provider, error)) Boot {
	cfg := testCfg()
	cfg.SkipSplash = false
	b := NewBoot(cfg, open)
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
// becomes the real UI when both the provider and the minimum hold are done.
func TestBootShowsLogoWhileSigningIn(t *testing.T) {
	b := bootWith(fakeOpen)
	if !strings.Contains(b.View(), "/ ____/") {
		t.Fatal("the logo must show before the provider is open")
	}
	b, _ = bootSend(b, bootHeldMsg{}) // hold over, provider still opening
	if b.inner != nil || !strings.Contains(b.View(), "signing in...") {
		t.Fatalf("must keep waiting and say so:\n%s", b.View())
	}
	b, cmd := bootSend(b, bootDoneMsg{prov: fakeProvider{[]provider.Resource{threeRows()}}})
	if b.inner == nil || cmd == nil {
		t.Fatal("provider ready and hold over: the real UI must start")
	}
	m := b.inner.(Model)
	if !m.splashing || !m.splashHeld || m.width != 120 {
		t.Fatalf("the fade must follow at once, sized: splashing=%v held=%v w=%d", m.splashing, m.splashHeld, m.width)
	}
}

func TestBootWaitsForTheHoldEvenWhenTheProviderIsFast(t *testing.T) {
	b := bootWith(fakeOpen)
	b, _ = bootSend(b, bootDoneMsg{prov: fakeProvider{}})
	if b.inner != nil {
		t.Fatal("the logo must stay for its minimum time")
	}
	if b, _ = bootSend(b, bootHeldMsg{}); b.inner == nil {
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
	b := NewBoot(cfg, fakeOpen)
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

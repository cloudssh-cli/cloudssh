package ui

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// fakeAuthn is a provider with profiles. A profile listed in fail makes
// UseProfile return that friendly error and leaves the provider signed out.
type fakeAuthn struct {
	fakeProvider
	mu       sync.Mutex
	err      error
	profile  string
	access   string
	region   string
	profiles []provider.ProfileInfo
	fail     map[string]error
	used     []string
}

func (f *fakeAuthn) AuthError() error { f.mu.Lock(); defer f.mu.Unlock(); return f.err }
func (f *fakeAuthn) Access() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "not signed in"
	}
	return f.access
}
func (f *fakeAuthn) Region() string                   { return f.region }
func (f *fakeAuthn) Profile() string                  { f.mu.Lock(); defer f.mu.Unlock(); return f.profile }
func (f *fakeAuthn) Profiles() []provider.ProfileInfo { return f.profiles }
func (f *fakeAuthn) UseProfile(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.used = append(f.used, name)
	if e, ok := f.fail[name]; ok {
		f.err, f.profile = e, name
		return e
	}
	f.err, f.profile = nil, name
	if name == "" {
		f.access = "environment keys"
	} else {
		f.access = "profile " + name + " · SSO"
	}
	return nil
}

// friendly is a headline-first message like the real provider produces (capitalised, ends in a period).
type friendly string

func (f friendly) Error() string { return string(f) }

var (
	errNoCreds = friendly("No AWS credentials found yet.\ncloudssh uses the standard AWS credential chain. Any of these works:\n  - a profile:      aws configure sso   (then pick it here)\ndetails: no EC2 IMDS role found")
	errSSO     = friendly("Your AWS single sign-on session needs a refresh.\nRun:  aws sso login --profile prod\nthen press r here to try again.")
	twoProfs   = []provider.ProfileInfo{
		{Name: "dev", Kind: "SSO", Region: "eu-west-1", Detail: "1111 Admin"},
		{Name: "prod", Kind: "access keys", Region: "us-east-1"},
	}
)

func signIn(t *testing.T, f *fakeAuthn, rec *recorder) Model {
	t.Helper()
	res := fakeResource{key: "ec2", rec: rec, pages: map[string]provider.Page{"": {Rows: rows("a", "b")}}}
	f.fakeProvider = fakeProvider{res: []provider.Resource{res}}
	m := New(testCfg(), f)
	m.detectTunnels = nil
	m = sized(m, 120, 30)
	return settle(m, m.Init())
}

func TestSignedOutStartsOnTheProfilesScreenWithAFriendlyMessage(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{err: errNoCreds, profiles: twoProfs}
	m := signIn(t, f, rec)
	if m.mode != m.profilesMode() || len(rec.queries) != 0 {
		t.Fatalf("mode=%d resource loads=%d: the app must wait for a profile", m.mode, len(rec.queries))
	}
	v := m.View()
	for _, want := range []string{"Profiles[2]", "dev", "prod", "SSO", "access keys", "eu-west-1", "Access: not signed in",
		"No AWS credentials found yet.", "Use profile", "Try again", "<profiles>"} {
		if !strings.Contains(v, want) {
			t.Errorf("signed-out screen missing %q:\n%s", want, v)
		}
	}
	for _, blame := range []string{"error", "failed", "invalid"} {
		if strings.Contains(strings.ToLower(v), blame) {
			t.Errorf("the screen should not sound like a failure (%q)", blame)
		}
	}
}

func TestChoosingAProfileSignsInAndLoadsTheApp(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{err: errNoCreds, profiles: twoProfs, region: "eu-west-1"}
	m := signIn(t, f, rec)
	m, cmd := send(m, key("enter")) // first row is dev
	m = settle(m, cmd)
	if len(f.used) != 1 || f.used[0] != "dev" {
		t.Fatalf("UseProfile calls: %v", f.used)
	}
	if m.authErr != nil || m.mode != 1 || m.cur() == nil || len(m.cur().rows) != 2 {
		t.Fatalf("authErr=%v mode=%d: choosing a profile should open the app", m.authErr, m.mode)
	}
	v := m.View()
	for _, want := range []string{"Access: profile dev · SSO", "Region: eu-west-1", "<p>"} {
		if !strings.Contains(v, want) {
			t.Errorf("after sign-in missing %q", want)
		}
	}
}

func TestAProfileThatDoesNotWorkKeepsYouOnTheListWithANewExplanation(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{err: errNoCreds, profiles: twoProfs, fail: map[string]error{"prod": errSSO}}
	m := signIn(t, f, rec)
	m, _ = send(m, key("j")) // prod
	m, cmd := send(m, key("enter"))
	m = settle(m, cmd)
	if m.mode != m.profilesMode() || m.authErr == nil || len(rec.queries) != 0 {
		t.Fatalf("mode=%d authErr=%v loads=%d", m.mode, m.authErr, len(rec.queries))
	}
	m, _ = send(m, clearStatusMsg{seq: m.statusSeq})
	if v := m.View(); !strings.Contains(v, "single sign-on session needs a refresh") || !strings.Contains(v, "dev") {
		t.Fatalf("the new reason should show, and the list stays usable:\n%s", v)
	}
	// after `aws sso login` elsewhere, r tries the same profile again
	delete(f.fail, "prod")
	m, cmd = send(m, key("r"))
	m = settle(m, cmd)
	if m.authErr != nil || m.mode != 1 || f.used[len(f.used)-1] != "prod" {
		t.Fatalf("retry: authErr=%v mode=%d used=%v", m.authErr, m.mode, f.used)
	}
}

func TestNoProfilesYetIsExplainedGentlyAndRetryWorks(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{err: errNoCreds} // nothing configured at all
	m := signIn(t, f, rec)
	v := m.View()
	for _, want := range []string{"No AWS credentials found yet.", "completely normal", "aws configure sso", "press r to try again"} {
		if !strings.Contains(v, want) {
			t.Errorf("empty state missing %q:\n%s", want, v)
		}
	}
	// the user exports keys / finishes setup, then presses r: the app opens
	f.err, f.access = nil, "environment keys"
	m, cmd := send(m, key("r"))
	m = settle(m, cmd)
	if m.authErr != nil || m.mode != 1 || len(m.cur().rows) != 2 {
		t.Fatalf("retry should sign in: authErr=%v mode=%d", m.authErr, m.mode)
	}
}

func TestSwitchProfilesAtAnyTime(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{profile: "dev", access: "profile dev · SSO", profiles: twoProfs, region: "eu-west-1"}
	m := signIn(t, f, rec)
	if v := m.View(); !strings.Contains(v, "Access: profile dev · SSO") || !strings.Contains(v, "<p>") {
		t.Fatalf("header should show the access and the p shortcut:\n%s", v)
	}
	loads := len(rec.queries)
	oldScreen := m.cur().id

	m, cmd := send(m, key("p"))
	m = settle(m, cmd)
	if m.mode != m.profilesMode() {
		t.Fatalf("p should open the profiles screen, mode=%d", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "●") {
		t.Fatalf("the active profile should be marked:\n%s", v)
	}
	m, _ = send(m, key("esc")) // back without switching
	if m.mode != 1 || len(f.used) != 0 || len(rec.queries) != loads || m.cur().id != oldScreen {
		t.Fatalf("esc must go back untouched: mode=%d used=%v", m.mode, f.used)
	}

	m, cmd = send(m, key("p"))
	m = settle(m, cmd)
	m, cmd = send(m, key("enter")) // dev again: already active
	m = settle(m, cmd)
	if len(f.used) != 0 || m.mode != 1 {
		t.Fatalf("choosing the active profile should just go back: used=%v mode=%d", f.used, m.mode)
	}

	m, cmd = send(m, key("p"))
	m = settle(m, cmd)
	m, _ = send(m, key("j")) // prod
	m, cmd = send(m, key("enter"))
	m = settle(m, cmd)
	if len(f.used) != 1 || f.used[0] != "prod" || m.mode != 1 {
		t.Fatalf("used=%v mode=%d", f.used, m.mode)
	}
	if m.cur().id == oldScreen || len(rec.queries) <= loads {
		t.Fatal("switching profile must drop what was loaded for the old account and reload")
	}
	if v := m.View(); !strings.Contains(v, "Access: profile prod · SSO") {
		t.Fatalf("header should follow the switch:\n%s", v)
	}
}

func TestEnvironmentKeysNeedNoProfileSetup(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{access: "environment keys", region: "us-east-1"} // no profiles on this machine
	m := signIn(t, f, rec)
	v := m.View()
	if !strings.Contains(v, "Access: environment keys") {
		t.Fatalf("header should say how we are signed in:\n%s", v)
	}
	if strings.Contains(v, "<p>") {
		t.Fatal("no profile shortcut is advertised when there are no profiles")
	}
	m, cmd := send(m, key("p")) // still reachable, and kind about it
	m = settle(m, cmd)
	if v := m.View(); !strings.Contains(v, "signed in with environment keys") || !strings.Contains(v, "none are needed") {
		t.Fatalf("profiles screen with nothing configured:\n%s", v)
	}
	m, _ = send(m, key("esc"))
	if m.mode != 1 {
		t.Fatalf("esc should return, mode=%d", m.mode)
	}
}

func TestResourceKeysWaitUntilSignedIn(t *testing.T) {
	rec := &recorder{}
	f := &fakeAuthn{err: errNoCreds, profiles: twoProfs}
	m := signIn(t, f, rec)
	for _, k := range []string{"1", "2", "d", "0"} {
		var cmd = func() Model { mm, _ := send(m, key(k)); return mm }
		if got := cmd(); got.mode != m.profilesMode() {
			t.Fatalf("%q left the profiles screen (mode %d)", k, got.mode)
		}
	}
	m, _ = send(m, key("3")) // not a resource view either
	if m.mode != m.profilesMode() || !strings.Contains(m.status, "Choose a profile first") {
		t.Fatalf("mode=%d status=%q", m.mode, m.status)
	}
	m, _ = send(m, key("p")) // p cannot leave while signed out
	if m.mode != m.profilesMode() {
		t.Fatal("p must not leave the profiles screen while signed out")
	}
}

func TestPWithoutProfilesSupportSaysSo(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("p"))
	if !strings.Contains(m.status, "no profiles") && !strings.Contains(m.status, "No profiles") && !strings.Contains(strings.ToLower(m.status), "no profiles") {
		t.Fatalf("status=%q", m.status)
	}
	if m.mode != 1 {
		t.Fatal("must stay on the current screen")
	}
}

func TestProfilesScreenListsInHelpAndDashboard(t *testing.T) {
	f := &fakeAuthn{profile: "dev", access: "profile dev · SSO", profiles: twoProfs}
	m := signIn(t, f, &recorder{})
	h, _ := send(m, key("?"))
	if !strings.Contains(h.View(), "Profiles: switch the sign-in profile") {
		t.Error("help should mention p")
	}
	h, _ = send(h, key("esc"))
	d, _ := send(h, key("d"))
	if !strings.Contains(d.View(), "Switch the sign-in profile") {
		t.Error("dashboard should list Profiles")
	}
}

func TestProfileStatesFitTheScreen(t *testing.T) {
	states := map[string]func() Model{
		"signed out with profiles": func() Model { return signIn(t, &fakeAuthn{err: errNoCreds, profiles: twoProfs}, &recorder{}) },
		"signed out, none yet":     func() Model { return signIn(t, &fakeAuthn{err: errNoCreds}, &recorder{}) },
		"signed in, none needed": func() Model {
			m := signIn(t, &fakeAuthn{access: "environment keys"}, &recorder{})
			m, cmd := send(m, key("p"))
			return settle(m, cmd)
		},
		"profiles open": func() Model {
			m := signIn(t, &fakeAuthn{profile: "dev", access: "profile dev · SSO", profiles: twoProfs}, &recorder{})
			m, cmd := send(m, key("p"))
			return settle(m, cmd)
		},
	}
	for name, mk := range states {
		for _, size := range [][2]int{{120, 30}, {80, 24}, {60, 14}, {200, 50}} {
			m := sized(mk(), size[0], size[1])
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s %v: %d lines, want %d", name, size, len(lines), size[1])
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%s %v: line %d is %d wide", name, size, i, w)
				}
			}
		}
	}
}

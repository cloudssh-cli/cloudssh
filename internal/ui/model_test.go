package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudssh-cli/cloudssh/internal/config"
	"github.com/cloudssh-cli/cloudssh/internal/provider"
	"github.com/cloudssh-cli/cloudssh/internal/sshx"
)

func TestInitLoadsFirstPage(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a", "b")}}})
	s := m.cur()
	if len(s.rows) != 2 || s.loading || s.pages != 1 {
		t.Fatalf("rows=%d loading=%v pages=%d", len(s.rows), s.loading, s.pages)
	}
}

func TestLoadErrorShownInStatus(t *testing.T) {
	m := loaded(t, fakeResource{err: errors.New("boom")})
	if m.status != "error: boom" {
		t.Fatalf("status=%q", m.status)
	}
}

func TestNextPageAppends(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{
		"":   {Rows: rows("a"), NextToken: "t1"},
		"t1": {Rows: rows("b")},
	}})
	m, cmd := send(m, key("n"))
	m = settle(m, cmd)
	s := m.cur()
	if len(s.rows) != 2 || s.nextToken != "" || s.pages != 2 {
		t.Fatalf("rows=%d token=%q pages=%d", len(s.rows), s.nextToken, s.pages)
	}
	m, _ = send(m, key("n"))
	if m.status != "no more pages" {
		t.Fatalf("status=%q", m.status)
	}
}

func TestSearchNarrowsRowsAndEscClears(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("/"))
	m = typeKeys(m, "alp")
	if s := m.cur(); len(s.view) != 1 || s.view[0].Cells[1] != "alpha" {
		t.Fatalf("view=%v", m.cur().view)
	}
	m, _ = send(m, key("esc"))
	if len(m.cur().view) != 3 || m.filtering {
		t.Fatalf("after esc: view=%d filtering=%v", len(m.cur().view), m.filtering)
	}
}

func TestSearchKeysDoNotLeakToGlobalBindings(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("/"))
	for _, k := range []string{"d", "1", "q", "?", "F", "S"} {
		var cmd tea.Cmd
		m, cmd = send(m, key(k))
		for _, msg := range run(cmd) {
			if _, quit := msg.(tea.QuitMsg); quit {
				t.Fatalf("%q quit while searching", k)
			}
		}
	}
	if m.mode != 1 || m.modal != nil || !m.filtering {
		t.Fatalf("mode=%d modal=%v filtering=%v", m.mode, m.modal, m.filtering)
	}
	if m.cur().search != "d1q?FS" {
		t.Fatalf("search=%q", m.cur().search)
	}
}

func TestSearchHistoryRecall(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("/"))
	m = typeKeys(m, "alp")
	m, _ = send(m, key("enter"))
	m, _ = send(m, key("esc")) // clear active search
	m, _ = send(m, key("/"))
	m, _ = send(m, key("up"))
	if m.filter.Value() != "alp" || m.cur().search != "alp" {
		t.Fatalf("history value=%q search=%q", m.filter.Value(), m.cur().search)
	}
}

func TestEnterOnEmptyTableDoesNothing(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {}}})
	_, cmd := send(m, key("enter"))
	if cmd != nil {
		t.Fatal("expected no command")
	}
}

func TestEnterResolvesHostThenSSHDialog(t *testing.T) {
	m := loaded(t, threeRows())
	m, cmd := send(m, key("enter"))
	var got resolvedMsg
	for _, msg := range run(cmd) {
		if r, ok := msg.(resolvedMsg); ok {
			got = r
		}
	}
	if got.host.Name != "alpha" {
		t.Fatalf("got %#v", got)
	}
	m, _ = send(m, got)
	if _, ok := m.modal.(*sshModal); !ok {
		t.Fatalf("modal=%T", m.modal)
	}
	if !strings.Contains(m.View(), "ssh -- 10.0.0.1") {
		t.Fatalf("preview missing:\n%s", m.View())
	}
	// typing sets the user and the preview follows
	m = typeKeys(m, "ops")
	if !strings.Contains(m.View(), "ssh ops@10.0.0.1") {
		t.Fatal("preview did not follow username")
	}
	m, cmd = send(m, key("enter"))
	if cmd == nil || m.modal != nil {
		t.Fatalf("enter should close the dialog and exec ssh (cmd=%v modal=%v)", cmd, m.modal)
	}
}

func TestSSHDialogSwitchesIPAndCancels(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, resolvedMsg{host: provider.Host{PrivateIP: "10.0.0.1", PublicIP: "3.3.3.3", DefaultUser: "ubuntu"}})
	if !strings.Contains(m.View(), "ssh ubuntu@10.0.0.1") {
		t.Fatal("default should be the private IP")
	}
	m, _ = send(m, key("tab")) // to the Address field
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	if !strings.Contains(m.View(), "ssh ubuntu@3.3.3.3") {
		t.Fatal("tab then right should switch to the public IP")
	}
	m, _ = send(m, key("esc"))
	if m.modal != nil {
		t.Fatal("esc should cancel")
	}
}

func TestResolvedMsgWithoutIPReportsError(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, resolvedMsg{host: provider.Host{}})
	if !strings.Contains(m.status, "error") || m.modal != nil {
		t.Fatalf("status=%q modal=%v", m.status, m.modal)
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

func TestStatusClears(t *testing.T) {
	m := loaded(t, threeRows())
	if m.status == "" {
		t.Fatal("expected status")
	}
	m, _ = send(m, clearStatusMsg{seq: m.statusSeq})
	if m.status != "" {
		t.Fatalf("status=%q", m.status)
	}
}

func TestStaleLoadIgnoredAfterRefresh(t *testing.T) {
	m := newModel(threeRows())
	stale := run(m.Init())[0]
	m, _ = send(m, key("r")) // bumps gen
	m, _ = send(m, stale)
	if len(m.cur().rows) != 0 {
		t.Fatal("stale load applied")
	}
}

func TestCursorMovesAndSelectionDrivesSSH(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, key("j"))
	m, _ = send(m, key("j"))
	m, _ = send(m, key("k"))
	_, cmd := send(m, key("S"))
	for _, msg := range run(cmd) {
		if r, ok := msg.(resolvedMsg); ok && r.host.Name != "beta" {
			t.Fatalf("selected %q, want beta", r.host.Name)
		}
	}
}

func TestSortKeysAreUniqueAndAvoidReserved(t *testing.T) {
	cols := []provider.Column{{Title: "ID"}, {Title: "Name"}, {Title: "Type"}, {Title: "AZ"}, {Title: "Private IP"},
		{Title: "Public IP"}, {Title: "ASG Name"}, {Title: "State"}, {Title: "Group Name"}}
	keys := assignSortKeys(cols)
	seen := map[string]bool{}
	for i, k := range keys {
		if k == "" {
			t.Errorf("%s has no sort key", cols[i].Title)
		}
		if reservedKeys[k] {
			t.Errorf("%s got reserved key %s", cols[i].Title, k)
		}
		if seen[k] {
			t.Errorf("duplicate key %s", k)
		}
		seen[k] = true
	}
	if keys[0] != "I" || keys[1] != "N" || keys[3] != "A" {
		t.Errorf("initials should win: %v", keys)
	}
}

func TestShiftLetterSortsAndRepeatReverses(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: rows("b", "a", "c")}}})
	m, _ = send(m, key("N")) // Name column
	if m.cur().view[0].Cells[1] != "a" {
		t.Fatalf("asc: %v", m.cur().view[0].Cells)
	}
	m, _ = send(m, key("N"))
	if m.cur().view[0].Cells[1] != "c" || !strings.Contains(m.View(), "Name ▼") {
		t.Fatalf("desc: %v", m.cur().view[0].Cells)
	}
	m, _ = send(m, key("R")) // reset clears the sort
	if m.cur().tbl.sortCol != -1 {
		t.Fatal("reset should clear sort")
	}
}

func TestDefaultSortFromColumn(t *testing.T) {
	m := loaded(t, fakeResource{
		cols:  []provider.Column{{Title: "ID"}, {Title: "Name", DefaultSort: "desc"}},
		pages: map[string]provider.Page{"": {Rows: rows("b", "a", "c")}},
	})
	if m.cur().view[0].Cells[1] != "c" {
		t.Fatalf("default desc sort not applied: %v", m.cur().view)
	}
}

func TestNumericColumnSortsByValue(t *testing.T) {
	rs := []provider.Row{{Cells: []string{"x", "10"}}, {Cells: []string{"y", "9"}}, {Cells: []string{"z", "100"}}}
	m := loaded(t, fakeResource{
		cols:  []provider.Column{{Title: "Name"}, {Title: "Count", Numeric: true}},
		pages: map[string]provider.Page{"": {Rows: rs}},
	})
	m, _ = send(m, key("C"))
	if got := m.cur().view[0].Cells[1] + m.cur().view[2].Cells[1]; got != "9100" {
		t.Fatalf("numeric sort wrong: %s", got)
	}
}

func TestColumnScrollOnNarrowTerminal(t *testing.T) {
	tb := newTable()
	tb.setCols([]provider.Column{{Title: "AAAAAAAAAA"}, {Title: "BBBBBBBBBB"}, {Title: "CCCCCCCCCC"}})
	tb.setRows([]provider.Row{{Cells: []string{"1", "2", "3"}}})
	tb.resize(30, 8)
	for i := 0; i < 4; i++ {
		tb.scrollRight()
	}
	if tb.colStart > 2 {
		t.Fatalf("colStart=%d", tb.colStart)
	}
	for i := 0; i < 4; i++ {
		tb.scrollLeft()
	}
	if tb.colStart != 0 {
		t.Fatalf("colStart=%d", tb.colStart)
	}
}

func TestSortRowsNumeric(t *testing.T) {
	r := []provider.Row{{Cells: []string{"10"}}, {Cells: []string{"9"}}, {Cells: []string{"x"}}}
	got := sortRows(r, 0, false, true)
	if got[0].Cells[0] != "9" || got[2].Cells[0] != "x" {
		t.Fatalf("%v", got)
	}
}

// fakeAuthProvider is a provider that can push temporary keys.
type fakeAuthProvider struct {
	fakeProvider
	got  provider.Host
	user string
	key  string
	err  error
}

func (*fakeAuthProvider) AuthLabel() string { return "Fake Connect" }
func (*fakeAuthProvider) AuthName() string  { return "fake-connect" }
func (f *fakeAuthProvider) AuthorizeSSH(_ context.Context, h provider.Host, user, pub string) error {
	f.got, f.user, f.key = h, user, pub
	return f.err
}

func authModel(t *testing.T, cfg config.Config, auth *fakeAuthProvider) Model {
	t.Helper()
	auth.fakeProvider = fakeProvider{res: []provider.Resource{threeRows()}}
	m := sized(New(cfg, auth), 120, 30)
	m = settle(m, m.Init())
	m, _ = send(m, resolvedMsg{host: provider.Host{ID: "i-1", AZ: "us-east-1a", PrivateIP: "10.0.0.1", DefaultUser: "ubuntu"}})
	return m
}

func TestSSHDialogOffersMethodOnlyWhenProviderSupportsIt(t *testing.T) {
	plain := loaded(t, threeRows())
	plain, _ = send(plain, resolvedMsg{host: provider.Host{PrivateIP: "10.0.0.1"}})
	if strings.Contains(plain.View(), "Instance Connect") || strings.Contains(plain.View(), "Method") {
		t.Fatal("no method choice without an Authorizer")
	}
	plain, _ = send(plain, key("tab")) // no extra fields: these must be inert
	plain, _ = send(plain, tea.KeyMsg{Type: tea.KeyRight})
	if plain.modal == nil {
		t.Fatal("tab and arrows must not close the dialog")
	}

	m := authModel(t, testCfg(), &fakeAuthProvider{})
	v := m.View()
	for _, want := range []string{"Method", "Fake Connect", "tab next", "ssh ubuntu@10.0.0.1"} {
		if !strings.Contains(v, want) {
			t.Errorf("dialog missing %q", want)
		}
	}
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	if v := m.View(); !strings.Contains(v, "-i <Fake Connect>") || !strings.Contains(v, "IdentitiesOnly=yes") {
		t.Fatalf("preview should show the temporary key command:\n%s", v)
	}
}

func TestConfigCanDefaultToTheTemporaryKey(t *testing.T) {
	cfg := testCfg()
	for _, name := range []string{"fake-connect", config.LegacySSHMethodTemporaryKey} {
		cfg.SSHMethod = name
		m := authModel(t, cfg, &fakeAuthProvider{})
		if !m.modal.(*sshModal).viaAuth || !strings.Contains(m.View(), "-i <Fake Connect>") {
			t.Fatalf("ssh_method: %s should preselect the temporary key", name)
		}
	}
	cfg.SSHMethod = "ssh"
	if authModel(t, cfg, &fakeAuthProvider{}).modal.(*sshModal).viaAuth {
		t.Fatal("ssh_method: ssh must not preselect it")
	}
	// but a provider without the capability falls back to plain ssh
	plain := loaded(t, threeRows())
	plain.cfg.SSHMethod = "fake-connect"
	plain, _ = send(plain, resolvedMsg{host: provider.Host{PrivateIP: "10.0.0.1"}})
	if plain.modal.(*sshModal).viaAuth {
		t.Fatal("must not select a method the provider lacks")
	}
}

func TestInstanceConnectPushesKeyThenRunsSSHAndCleansUp(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	auth := &fakeAuthProvider{}
	m := authModel(t, testCfg(), auth)
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	m, cmd := send(m, key("enter"))
	if m.modal != nil || !strings.Contains(m.status, "temporary key") {
		t.Fatalf("modal=%v status=%q", m.modal, m.status)
	}
	var ready tempKeyMsg
	for _, msg := range run(cmd) {
		if e, ok := msg.(tempKeyMsg); ok {
			ready = e
		}
	}
	defer ready.key.Cleanup()
	if ready.err != nil {
		t.Fatal(ready.err)
	}
	if auth.got.ID != "i-1" || auth.got.AZ != "us-east-1a" || auth.user != "ubuntu" || !strings.HasPrefix(auth.key, "ssh-ed25519 ") {
		t.Fatalf("authorizer got host=%+v user=%q key=%q", auth.got, auth.user, auth.key)
	}
	args := strings.Join(ready.target.Args(), " ")
	if !strings.Contains(args, "-i "+ready.key.Path) || !strings.Contains(args, "IdentitiesOnly=yes") || !strings.HasSuffix(args, "ubuntu@10.0.0.1") {
		t.Fatalf("ssh args: %s", args)
	}
	if _, cmd = send(m, ready); cmd == nil {
		t.Fatal("tempKeyMsg should start ssh")
	}
}

func TestInstanceConnectFailureCleansUpAndReports(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	auth := &fakeAuthProvider{err: errors.New("AccessDenied")}
	m := authModel(t, testCfg(), auth)
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	m, cmd := send(m, key("enter"))
	var res tempKeyMsg
	for _, msg := range run(cmd) {
		if e, ok := msg.(tempKeyMsg); ok {
			res = e
		}
	}
	if res.err == nil || res.key.Dir == "" {
		t.Fatalf("expected an error carrying the key to clean up: %+v", res)
	}
	m, _ = send(m, res)
	if !strings.Contains(m.status, "AccessDenied") {
		t.Fatalf("status=%q", m.status)
	}
	if _, err := os.Stat(res.key.Dir); !os.IsNotExist(err) {
		t.Fatal("temporary key must be deleted when authorisation fails")
	}
}

func tunnelHost() provider.Host {
	return provider.Host{ID: "i-1", Name: "web", PrivateIP: "10.0.0.1", DefaultUser: "ubuntu"}
}

func TestConnectOffersOnlyTunnelsThatLeadToTheHost(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: []provider.Row{
		{Cells: []string{"i-1", "web"}, Ref: tunnelHost()},
	}}}})
	m.detectTunnels = func(context.Context) []sshx.LocalTunnel {
		return []sshx.LocalTunnel{
			{Port: 2222, RemotePort: 22, InstanceID: "i-1"},
			{Port: 2223, RemotePort: 22, InstanceID: "i-other"},
			{Port: 3306, RemotePort: 3306, InstanceID: "i-1"},
		}
	}
	_, cmd := send(m, key("S"))
	var got resolvedMsg
	for _, msg := range run(cmd) {
		if r, ok := msg.(resolvedMsg); ok {
			got = r
		}
	}
	if len(got.tunnels) != 1 || got.tunnels[0].Port != 2222 {
		t.Fatalf("tunnels: %+v", got.tunnels)
	}
}

func TestSSHDialogOffersRunningTunnel(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, resolvedMsg{host: tunnelHost(), tunnels: []sshx.LocalTunnel{{Port: 2222, RemotePort: 22, InstanceID: "i-1"}}})
	v := m.View()
	for _, want := range []string{"Route", "Running tunnel", "127.0.0.1:2222", "tab next", "ssh ubuntu@10.0.0.1"} {
		if !strings.Contains(v, want) {
			t.Errorf("dialog missing %q", want)
		}
	}
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	v = m.View()
	for _, want := range []string{"-p 2222", "HostKeyAlias=i-1", "ServerAliveInterval=5", "ubuntu@127.0.0.1"} {
		if !strings.Contains(v, want) {
			t.Errorf("tunnel preview missing %q:\n%s", want, v)
		}
	}
	m, cmd := send(m, key("enter"))
	if cmd == nil || m.modal != nil {
		t.Fatalf("enter should start ssh through the tunnel (cmd=%v modal=%v)", cmd, m.modal)
	}
}

func TestTunnelOptionHiddenAndInertWithoutAMatchingTunnel(t *testing.T) {
	m := loaded(t, threeRows())
	m, _ = send(m, resolvedMsg{host: tunnelHost()})
	if strings.Contains(m.View(), "Route") || strings.Contains(m.View(), "Running tunnel") {
		t.Fatal("no tunnel option without a running tunnel")
	}
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	if m.modal == nil || m.modal.(*sshModal).viaTunnel {
		t.Fatal("arrows must be inert without a tunnel")
	}
}

func TestTunnelCombinesWithTemporaryKey(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	auth := &fakeAuthProvider{}
	m := authModel(t, testCfg(), auth)
	m, _ = send(m, resolvedMsg{
		host:    provider.Host{ID: "i-1", AZ: "us-east-1a", PrivateIP: "10.0.0.1", DefaultUser: "ubuntu"},
		tunnels: []sshx.LocalTunnel{{Port: 2222, RemotePort: 22, InstanceID: "i-1"}},
	})
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	m, _ = send(m, key("tab"))
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := send(m, key("enter"))
	var ready tempKeyMsg
	for _, msg := range run(cmd) {
		if e, ok := msg.(tempKeyMsg); ok {
			ready = e
		}
	}
	defer ready.key.Cleanup()
	if ready.err != nil {
		t.Fatal(ready.err)
	}
	args := strings.Join(ready.target.Args(), " ")
	for _, want := range []string{"-p 2222", "HostKeyAlias=i-1", "IdentitiesOnly=yes", "ubuntu@127.0.0.1"} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q missing %q", args, want)
		}
	}
	if auth.got.ID != "i-1" {
		t.Fatal("the temporary key must still be pushed to the instance")
	}
}

func TestGAndShiftGJumpToTopAndBottomOfTheTable(t *testing.T) {
	m := loaded(t, fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a", "b", "c", "d", "e")}}})
	m, _ = send(m, key("G"))
	if m.cur().tbl.cursor != 4 {
		t.Fatalf("G: cursor=%d", m.cur().tbl.cursor)
	}
	m, _ = send(m, key("g"))
	if m.cur().tbl.cursor != 0 {
		t.Fatalf("g: cursor=%d", m.cur().tbl.cursor)
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.cur().tbl.cursor != 4 {
		t.Fatalf("end: cursor=%d", m.cur().tbl.cursor)
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyHome})
	if m.cur().tbl.cursor != 0 {
		t.Fatalf("home: cursor=%d", m.cur().tbl.cursor)
	}
}

// demoProvider is a provider running on sample data.
type demoProvider struct{ fakeProvider }

func (demoProvider) Demo() bool { return true }

func TestDemoModeIsMarkedAndNeverStartsRealSSH(t *testing.T) {
	res := fakeResource{pages: map[string]provider.Page{"": {Rows: rows("a")}}}
	m := sized(New(testCfg(), demoProvider{fakeProvider{res: []provider.Resource{res}}}), 120, 30)
	m = settle(m, m.Init())
	if v := m.View(); !strings.Contains(v, "· demo") {
		t.Fatalf("the header must say this is a demo:\n%s", v)
	}
	if m.detectTunnels != nil {
		t.Error("demo mode has no real tunnels to look for")
	}
	target := sshx.Target{User: "ubuntu", Address: "10.0.0.1"}
	if got := m.command(target); filepath.Base(got.Path) != "sh" || !strings.Contains(strings.Join(got.Args, " "), "[demo]") {
		t.Fatalf("demo connection must be the local shell, got %v", got.Args)
	}
	real := newModel(fakeResource{})
	if got := real.command(target); got.Args[0] != "ssh" {
		t.Fatalf("a normal session runs ssh, got %v", got.Args)
	}
	if strings.Contains(real.View(), "· demo") {
		t.Error("only demo sessions are marked")
	}
}

func keyModel(t *testing.T, pemDir string, demo bool) Model {
	t.Helper()
	cfg := testCfg()
	cfg.PemFilesPath = pemDir
	var p provider.Provider = &fakeAuthProvider{fakeProvider: fakeProvider{res: []provider.Resource{threeRows()}}}
	if demo {
		p = demoProvider{fakeProvider{res: []provider.Resource{threeRows()}}}
	}
	m := sized(New(cfg, p), 120, 30)
	m = settle(m, m.Init())
	m, _ = send(m, resolvedMsg{host: provider.Host{ID: "i-1", PrivateIP: "10.0.0.1", DefaultUser: "ubuntu", KeyName: "prod"}})
	return m
}

func TestMissingPemOffersAKeyFileAndOtherwiseUsesTheAgent(t *testing.T) {
	dir := t.TempDir()
	m := keyModel(t, dir, false)
	if v := m.View(); !strings.Contains(v, "Key file (optional)") || !strings.Contains(v, "prod.pem not found") || !strings.Contains(v, "agent") {
		t.Fatalf("a missing pem must be explained with an optional Key file field:\n%s", v)
	}
	tgt, err := m.modal.(*sshModal).target()
	if err != nil || tgt.KeyFile != "" || strings.Contains(tgt.CommandLine(), "-i") {
		t.Fatalf("with no key chosen ssh uses the agent keys: %+v %v", tgt, err)
	}

	m, _ = send(m, key("tab"))                      // User -> Method
	m, _ = send(m, key("tab"))                      // -> Key file
	m = typeKeys(m, filepath.Join(dir, "nope.pem")) // a path that does not exist is refused
	m, _ = send(m, key("enter"))
	if m.modal == nil || !strings.Contains(m.View(), "no such file") {
		t.Fatalf("a bad path must be refused, dialog open:\n%s", m.View())
	}

	pem := filepath.Join(dir, "mine.pem")
	if err := os.WriteFile(pem, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.modal.(*sshModal).keyFile.SetValue(pem)
	if tgt, err = m.modal.(*sshModal).target(); err != nil || tgt.KeyFile != pem || !tgt.OnlyKey {
		t.Fatalf("a real key file connects with that key: %+v %v", tgt, err)
	}
	m, cmd := send(m, key("enter"))
	if m.modal != nil || cmd == nil {
		t.Fatal("with a usable key, enter hands over to ssh")
	}

	m = keyModel(t, dir, false) // and with the field empty, enter goes straight to ssh
	if m, cmd = send(m, key("enter")); m.modal != nil || cmd == nil {
		t.Fatal("no key chosen: enter connects with the agent keys")
	}
}

func TestSSHDialogLinesNeverBreakTheFrame(t *testing.T) {
	dir := t.TempDir()
	m := keyModel(t, dir, false)
	m.modal.(*sshModal).keyFile.SetValue(filepath.Join(dir, strings.Repeat("deep/", 12)+"k.pem"))
	m, _ = send(m, key("enter"))
	for _, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > m.width {
			t.Fatalf("line wider than the screen (%d): %q", w, l)
		}
	}
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.HasPrefix(l, "│") != strings.HasSuffix(l, "│") {
			t.Fatalf("a panel line lost its frame: %q", l)
		}
	}
}

func TestPemPresentOrOtherMethodNeedsNoKeyField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prod.pem"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if keyModel(t, dir, false).modal.(*sshModal).needKey {
		t.Fatal("the pem exists: no Key field")
	}
	missing := keyModel(t, t.TempDir(), false)
	missing, _ = send(missing, key("tab")) // Method
	missing, _ = send(missing, tea.KeyMsg{Type: tea.KeyRight})
	if strings.Contains(missing.View(), "Key file") {
		t.Fatalf("a temporary key replaces the pem: no Key field:\n%s", missing.View())
	}
	if keyModel(t, t.TempDir(), true).modal.(*sshModal).needKey {
		t.Fatal("demo mode has no real key files to miss")
	}
}

func TestHeaderArrowsShowHiddenColumns(t *testing.T) {
	tb := newTable()
	tb.setCols([]provider.Column{{Title: "AAAAAAAAAA"}, {Title: "BBBBBBBBBB"}, {Title: "CCCCCCCCCC"}})
	tb.setRows([]provider.Row{{Cells: []string{"1", "2", "3"}}})
	tb.resize(30, 8)
	head := func() string { return strings.Split(tb.render("t", ""), "\n")[1] }
	if h := head(); !strings.Contains(h, "»") || strings.Contains(h, "«") {
		t.Fatalf("columns hidden on the right: only » expected: %q", h)
	}
	tb.scrollRight()
	if h := head(); !strings.Contains(h, "«") {
		t.Fatalf("scrolled right: « expected: %q", h)
	}
	tb.resize(120, 8)
	tb.colStart = 0
	if h := head(); strings.Contains(h, "»") || strings.Contains(h, "«") {
		t.Fatalf("everything fits: no arrows: %q", h)
	}
}

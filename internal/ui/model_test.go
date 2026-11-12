package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
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
	sm, ok := m.modal.(*sshModal)
	if !ok {
		t.Fatalf("modal=%T", m.modal)
	}
	if !strings.Contains(m.View(), "ssh ec2-user@10.0.0.1") {
		t.Fatalf("preview missing:\n%s", m.View())
	}
	// typing edits the user and the preview follows
	sm.user.SetValue("")
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
	m, _ = send(m, key("tab"))
	if !strings.Contains(m.View(), "ssh ubuntu@3.3.3.3") {
		t.Fatal("tab should switch to the public IP")
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

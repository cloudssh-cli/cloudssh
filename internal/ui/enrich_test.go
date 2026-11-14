package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// lazyResource lists cheap partial rows and enriches them on request.
type lazyResource struct {
	fakeResource
	mu     sync.Mutex
	calls  []string
	failOn string
}

func partialRows(n int) []provider.Row {
	var out []provider.Row
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("item-%02d", i)
		out = append(out, provider.Row{Cells: []string{id, "…"}, Ref: id, ID: id, Partial: true})
	}
	return out
}

func (l *lazyResource) Enrich(_ context.Context, r provider.Row) (provider.Row, error) {
	l.mu.Lock()
	l.calls = append(l.calls, r.ID)
	l.mu.Unlock()
	if r.ID == l.failOn {
		return r, errors.New("denied")
	}
	r.Cells, r.Partial = []string{r.ID, "details of " + r.ID}, false
	return r, nil
}

func lazyModel(t *testing.T, n, height int, failOn string) (Model, *lazyResource) {
	t.Helper()
	l := &lazyResource{fakeResource: fakeResource{pages: map[string]provider.Page{"": {Rows: partialRows(n)}}}, failOn: failOn}
	m := sized(New(testCfg(), fakeProvider{res: []provider.Resource{l}}), 100, height)
	return settle(m, m.Init()), l
}

func TestOnlyVisibleRowsAreEnrichedAndFilledInAsTheyArrive(t *testing.T) {
	m, l := lazyModel(t, 40, 14, "") // the table shows about 4 rows
	v := m.View()
	if !strings.Contains(v, "details of item-01") || strings.Contains(v, "details of item-30") {
		t.Fatalf("visible rows are completed, others are not:\n%s", v)
	}
	if len(l.calls) == 0 || len(l.calls) > 6 {
		t.Fatalf("only the visible rows should be requested, got %d: %v", len(l.calls), l.calls)
	}
	before := len(l.calls)
	m, cmd := send(m, key("G")) // jump to the bottom: those rows are enriched now
	m = settle(m, cmd)
	if !strings.Contains(m.View(), "details of item-40") || len(l.calls) <= before {
		t.Fatalf("scrolling brings new rows into view and enriches them:\n%s", m.View())
	}
	again := len(l.calls)
	m, cmd = send(m, key("k"))
	settle(m, cmd)
	if len(l.calls) != again {
		t.Fatal("a row that is already complete must not be requested again")
	}
}

func TestEnrichmentFailureShowsQuestionMarksOnceAndDoesNotLoop(t *testing.T) {
	m, l := lazyModel(t, 3, 14, "item-02")
	v := m.View()
	if !strings.Contains(v, "details of item-01") || !strings.Contains(v, "details of item-03") {
		t.Fatalf("the other rows still complete:\n%s", v)
	}
	if !strings.Contains(m.status, "details for item-02") {
		t.Fatalf("the failure is reported: %q", m.status)
	}
	for i := 0; i < 3; i++ { // moving around must not retry the failed row
		var cmd tea.Cmd
		m, cmd = send(m, key("j"))
		m = settle(m, cmd)
	}
	n := 0
	for _, c := range l.calls {
		if c == "item-02" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the failed row is attempted once, not %d times", n)
	}
}

func TestStaleDetailsFromAnOldListAreIgnored(t *testing.T) {
	m, _ := lazyModel(t, 3, 14, "")
	s := m.cur()
	old := s.gen
	s.gen++ // a refresh happened
	m, _ = send(m, enrichedMsg{sid: s.id, gen: old, id: "item-01", row: provider.Row{ID: "item-01", Cells: []string{"item-01", "STALE"}}})
	if strings.Contains(m.View(), "STALE") {
		t.Fatal("a late answer for the previous list must not be applied")
	}
}

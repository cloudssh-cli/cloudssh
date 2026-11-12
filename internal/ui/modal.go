package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// modal is an overlay that takes over the table area and swallows all keys
// (except ctrl+c, handled by the model) while open.
type modal interface {
	// key handles a key press; done=true closes the modal.
	key(m *Model, msg tea.KeyMsg) (done bool, cmd tea.Cmd)
	// view renders the modal inside a w x h area (exactly h lines, w wide).
	view(w, h int) string
}

// placeCenter centres block (multi-line) in a w x h blank area.
func placeCenter(block string, w, h int) string {
	lines := strings.Split(block, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	top := (h - len(lines)) / 2
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, fit("", w))
	}
	for _, l := range lines {
		out = append(out, center(l, w))
	}
	for len(out) < h {
		out = append(out, fit("", w))
	}
	return strings.Join(out, "\n")
}

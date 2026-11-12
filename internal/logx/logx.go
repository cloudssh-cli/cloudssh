// Package logx is cloudssh's debug log. It is off unless a file is given
// (--log-file, log_file in the config, or $CLOUDSSH_LOG): the TUI owns the
// terminal, so logs can only go to a file. Everything else in the program
// calls L() and never has to know whether logging is on.
package logx

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu  sync.RWMutex
	log = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// L is the process-wide logger; it discards everything until Init succeeds.
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return log
}

// Init starts logging to path (created 0600, appended to). An empty path
// keeps logging off. The returned func closes the file.
func Init(path string, debug bool) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return func() {}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // the user chose this path
	if err != nil {
		return func() {}, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	mu.Lock()
	log = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
	mu.Unlock()
	return func() { _ = f.Close() }, nil
}

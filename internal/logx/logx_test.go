package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOffByDefaultAndWritesWhenGivenAFile(t *testing.T) {
	L().Info("goes nowhere") // must not panic with logging off

	path := filepath.Join(t.TempDir(), "sub", "cloudssh.log")
	closeFn, err := Init(path, false)
	if err != nil {
		t.Fatal(err)
	}
	L().Info("hello", "k", "v")
	L().Debug("hidden at info level")
	closeFn()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); !strings.Contains(s, "hello") || !strings.Contains(s, "k=v") || strings.Contains(s, "hidden") {
		t.Fatalf("log: %q", s)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("log must be private, mode %v", st.Mode())
	}
}

func TestEmptyPathKeepsLoggingOff(t *testing.T) {
	closeFn, err := Init("", true)
	if err != nil {
		t.Fatal(err)
	}
	closeFn()
}

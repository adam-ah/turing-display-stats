//go:build windows

package displayapp

import (
	"path/filepath"
	"testing"
)

func TestConfigPathForExecutable(t *testing.T) {
	exePath := filepath.Join(`C:\build\dist`, "turing-display.exe")
	got := configPathForExecutable(exePath)
	want := filepath.Join(`C:\build\dist`, configFileName)
	if got != want {
		t.Fatalf("configPathForExecutable(%q) = %q, want %q", exePath, got, want)
	}
}

//go:build windows

package displayapp

import (
	"path/filepath"
	"testing"
)

func TestRepoRootFromFile(t *testing.T) {
	got := repoRootFromFile(filepath.Join("C:", "work", "go_display", "internal", "displayapp", "assets.go"))
	want := filepath.Clean(filepath.Join("C:", "work"))
	if got != want {
		t.Fatalf("repoRootFromFile() = %q, want %q", got, want)
	}
}

func TestRepoAssetPathUsesProjectRoot(t *testing.T) {
	got, err := repoAssetPath("res", "icons", "monitor-icon-17865", "icon.ico")
	if err != nil {
		t.Fatalf("repoAssetPath returned error: %v", err)
	}
	if filepath.Base(got) != "icon.ico" {
		t.Fatalf("repoAssetPath returned %q, want a path ending in icon.ico", got)
	}
}

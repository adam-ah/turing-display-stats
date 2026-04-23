//go:build windows

package displayapp

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

func repoRootFromFile(file string) string {
	// This package lives under go_display/internal/displayapp, while shared
	// assets live alongside go_display in ../res.
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func repoRootPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	return repoRootFromFile(file), nil
}

func repoAssetPath(parts ...string) (string, error) {
	root, err := repoRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{root}, parts...)...), nil
}

func loadPNGAsset(parts ...string) (image.Image, error) {
	path, err := repoAssetPath(parts...)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

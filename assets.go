//go:build windows

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

func repoAssetPath(parts ...string) (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	return filepath.Join(append([]string{filepath.Dir(root)}, parts...)...), nil
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

//go:build windows

package app

import (
	"os"
	"path/filepath"
)

const configFileName = "config.json"

func configPathForExecutable(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), configFileName)
}

func appConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return configFileName
	}
	return configPathForExecutable(exePath)
}

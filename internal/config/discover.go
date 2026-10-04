package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSearchDirs lists the directories searched for a config file when
// --config is not given, in order of precedence.
var DefaultSearchDirs = []string{".", "/etc/ts-restic-server"}

// configFileNames are tried in each search directory, in order of precedence.
var configFileNames = []string{"config.yaml", "config.yml"}

// FindConfigFile resolves the config file to load.
//
// If explicit is non-empty it must point to an existing regular file.
// Otherwise each directory in searchDirs is checked for config.yaml and then
// config.yml; the first regular file found wins. An empty path with a nil
// error means no config file exists and defaults plus environment variables
// apply.
func FindConfigFile(explicit string, searchDirs []string) (string, error) {
	if explicit != "" {
		info, err := os.Stat(explicit)
		if err != nil {
			return "", fmt.Errorf("config file %s: %w", explicit, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("config file %s is not a regular file", explicit)
		}
		return explicit, nil
	}

	for _, dir := range searchDirs {
		for _, name := range configFileNames {
			p := filepath.Join(dir, name)
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				return p, nil
			}
		}
	}
	return "", nil
}

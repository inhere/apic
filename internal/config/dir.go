package config

import (
	"os"
	"path/filepath"
)

// Dir returns the configured directory or the per-user default.
func Dir() (string, error) {
	if dir := os.Getenv("APIC_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "apic"), nil
}

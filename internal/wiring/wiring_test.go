package wiring

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const execSecretValue = "tok-from-config-dir"

// writeSecretScript writes a script that prints execSecretValue under root and
// returns its path relative to root.
func writeSecretScript(t *testing.T, root string) string {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	assert.NoErr(t, os.MkdirAll(binDir, 0o755))
	if runtime.GOOS == "windows" {
		body := "@echo off\r\necho " + execSecretValue + "\r\n"
		assert.NoErr(t, os.WriteFile(filepath.Join(binDir, "secret.cmd"), []byte(body), 0o600))
		return "bin/secret.cmd"
	}
	body := "#!/bin/sh\necho " + execSecretValue + "\n"
	assert.NoErr(t, os.WriteFile(filepath.Join(binDir, "secret.sh"), []byte(body), 0o755))
	return "bin/secret.sh"
}

// TestLoadExecUsesTheConfigDirectory checks that ${exec:} keeps the directory
// bound at Load time: a caller such as site add swaps Deps.Dir for a temporary
// directory and the relative command still resolves against the real one.
func TestLoadExecUsesTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	config := "version: 1\nsites:\n  demo:\n    envs:\n      main:\n        base_url: http://example.test\n"
	assert.NoErr(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o600))
	script := writeSecretScript(t, dir)
	t.Setenv("APIC_CONFIG_DIR", dir)

	d, err := Load()
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, dir, d.Dir)
	assert.True(t, d.Env.Exec != nil, "Load must inject config.Env.Exec")

	// The script does not exist under the new directory, so re-injection fails.
	d.Dir = t.TempDir()
	out, err := d.Env.Exec([]string{script})
	assert.NoErr(t, err)
	assert.Eq(t, execSecretValue, strings.TrimSpace(out))
}

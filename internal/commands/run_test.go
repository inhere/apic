package commands_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/commands"
)

func TestReservedSiteSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	def := filepath.Join(dir, "pet.yaml")
	assert.Require(t, assert.NoError(t, os.WriteFile(def, []byte("operations:\n  one: {get: /one}\n"), 0600)))
	config := fmt.Sprintf("version: 1\nsites:\n  skill: {source: {type: simple, file: missing.yaml}}\n  pet:\n    source: {type: simple, file: %q}\n    default_env: e\n    envs: {e: {base_url: http://example.test}}\n", def)
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600)))

	code, stdout, stderr := runIO(t, "ops", "pet")
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, stdout, "one")
	warning := `warning: site "skill" uses a reserved name and is skipped; rename it`
	assert.Eq(t, 1, strings.Count(stderr, warning))

	code, _, _ = runIO(t, "site", "add", "skill", "--def", filepath.Join(dir, "missing.yaml"))
	assert.True(t, code != 0)
}

func TestRun(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	t.Run("version", func(t *testing.T) {
		assert.Eq(t, 0, commands.Run(commands.BuildInfo{Version: "t"}, []string{"--version"}))
	})
	t.Run("unknown command", func(t *testing.T) {
		assert.True(t, commands.Run(commands.BuildInfo{Version: "t"}, []string{"no-such-cmd"}) != 0)
	})
}

package commands_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/config"
)

func TestLinkCLI(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", cfg)
	scripts := filepath.Join(t.TempDir(), "links")
	yaml := "version: 1\ndefaults:\n  link_dir: " + scripts + "\nsites:\n  pet:\n    source:\n      type: simple\n    envs:\n      prod:\n        base_url: http://example.test\n    auths:\n      ro:\n        type: bearer\n        token: dummy\n"
	// Quote Windows paths in YAML so backslashes are not interpreted as escapes.
	if runtime.GOOS == "windows" {
		yaml = strings.Replace(yaml, "link_dir: "+scripts, "link_dir: '"+scripts+"'", 1)
	}
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte(yaml), 0600)))
	t.Run("generate list remove", func(t *testing.T) {
		code, out := runCommand(t, "link", "pet", "--name", "petctl", "--env", "prod", "--auth", "ro")
		assert.Eq(t, 0, code)
		path := filepath.Join(scripts, "petctl")
		assert.Contains(t, out, path)
		if runtime.GOOS == "windows" {
			assert.Contains(t, out, ".cmd 版传 JSON 参数请用 -d @file")
		}
		data, err := os.ReadFile(path)
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), "--api pet --env prod --auth ro")
		st, err := config.LoadState(cfg)
		assert.NoError(t, err)
		assert.Eq(t, "pet", st.Links["petctl"].Site)
		code, out = runCommand(t, "link", "ls")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "petctl\tpet\t"+path)
		code, _ = runCommand(t, "link", "rm", "petctl")
		assert.Eq(t, 0, code)
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
		st, err = config.LoadState(cfg)
		assert.NoError(t, err)
		assert.Len(t, st.Links, 0)
	})
	t.Run("validation", func(t *testing.T) {
		for _, args := range [][]string{
			{"link", "pet", "--name", "../x"},
			{"link", "missing", "--name", "x"},
			{"link", "pet", "--name", "x", "--env", "missing"},
			{"link", "pet", "--name", "x", "--auth", "missing"},
			{"--yes", "link", "pet", "--name", "x"},
			{"link", "pet", "--name", "x", "-y"},
		} {
			code, _ := runCommand(t, args...)
			assert.True(t, code != 0)
		}
	})
}

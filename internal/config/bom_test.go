package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
)

const bom = "\xEF\xBB\xBF"

// Files saved by Windows PowerShell 5 or older Notepad start with a UTF-8 BOM.
func TestLoadStripsBOM(t *testing.T) {
	dir := testDir(t)
	putFile(t, filepath.Join(dir, "config.yaml"), bom+"sites:\n  one:\n    default_env: main\n    envs:\n      main: {base_url: https://one.example}\n")
	putFile(t, filepath.Join(dir, "sites", "two.yaml"), bom+"envs:\n  main: {base_url: https://two.example}\n")
	putFile(t, filepath.Join(dir, ".env"), bom+"TOKEN_A=abc\n")
	putFile(t, filepath.Join(dir, "overlays", "one.yaml"), bom+"operations: {}\n")
	putFile(t, filepath.Join(dir, "state.yaml"), bom+"selections:\n  one:\n    env: main\n")

	cfg, err := config.Load(dir)
	assert.Require(t, assert.NoError(t, err))
	assert.Require(t, assert.NotNil(t, cfg.Sites["one"]))
	assert.Require(t, assert.NotNil(t, cfg.Sites["two"]))
	assert.Eq(t, "https://one.example", cfg.Sites["one"].Envs["main"].BaseURL)
	assert.Eq(t, "https://two.example", cfg.Sites["two"].Envs["main"].BaseURL)

	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoError(t, err))
	v, ok := env.Values["TOKEN_A"]
	assert.True(t, ok)
	assert.Eq(t, "abc", v)

	_, err = config.LoadOverlay(dir, "one")
	assert.NoError(t, err)

	st, err := config.LoadState(dir)
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, "main", st.Selections["one"].Env)
}

// The shipped GitLab example must keep loading as a config directory.
func TestGitLabExampleLoads(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "examples", "gitlab"))
	assert.Require(t, assert.NoError(t, err))
	site := cfg.Sites["gitlab"]
	assert.Require(t, assert.NotNil(t, site))
	assert.Eq(t, "https://gitlab.com/api/v4", site.Envs["main"].BaseURL)
	assert.Eq(t, "pat", site.DefaultAuth)
}

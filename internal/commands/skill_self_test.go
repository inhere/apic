package commands

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

// runSkill dispatches through Run while capturing stdout.
func runSkill(t *testing.T, info BuildInfo, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	assert.Require(t, assert.NoError(t, err))
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := Run(info, args)
	assert.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	assert.NoError(t, err)
	assert.NoError(t, r.Close())
	return code, string(out)
}

func TestSkillSelfCLI(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", cfg)

	t.Run("out option", func(t *testing.T) {
		out := t.TempDir()
		code, stdout := runSkill(t, BuildInfo{Version: "v9.9.9"}, "skill", "--out", out)
		assert.Eq(t, 0, code)
		path := filepath.Join(out, "apic", "SKILL.md")
		assert.Eq(t, path+"\n", stdout)
		data, err := os.ReadFile(path)
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), "Version: v9.9.9\n")
	})

	t.Run("default dir and name", func(t *testing.T) {
		code, stdout := runSkill(t, BuildInfo{}, "skill")
		assert.Eq(t, 0, code)
		path := filepath.Join(cfg, "skills", "apic", "SKILL.md")
		assert.Eq(t, path+"\n", stdout)
		data, err := os.ReadFile(path)
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), "Version: dev\n")
	})

	t.Run("name option", func(t *testing.T) {
		out := t.TempDir()
		code, stdout := runSkill(t, BuildInfo{}, "skill", "--out", out, "--name", "apic-guide")
		assert.Eq(t, 0, code)
		assert.Eq(t, filepath.Join(out, "apic-guide", "SKILL.md")+"\n", stdout)
	})

	t.Run("invalid name exits 2", func(t *testing.T) {
		out := t.TempDir()
		code, _ := runSkill(t, BuildInfo{}, "skill", "--out", out, "--name", "../x")
		assert.Eq(t, 2, code)
		_, err := os.Stat(filepath.Join(out, "..", "x"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("foreign file needs force", func(t *testing.T) {
		out := t.TempDir()
		file := filepath.Join(out, "apic", "SKILL.md")
		assert.Require(t, assert.NoError(t, os.MkdirAll(filepath.Dir(file), 0755)))
		assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte("hand written\n"), 0644)))

		code, _ := runSkill(t, BuildInfo{}, "skill", "--out", out)
		assert.Eq(t, 1, code)
		got, err := os.ReadFile(file)
		assert.Require(t, assert.NoError(t, err))
		assert.Eq(t, "hand written\n", string(got))

		code, _ = runSkill(t, BuildInfo{}, "skill", "--out", out, "--force")
		assert.Eq(t, 0, code)
		got, err = os.ReadFile(file)
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(got), "## Exit codes\n")
	})

	t.Run("missing config dir", func(t *testing.T) {
		t.Setenv("APIC_CONFIG_DIR", filepath.Join(t.TempDir(), "absent"))
		out := t.TempDir()
		code, stdout := runSkill(t, BuildInfo{}, "skill", "--out", out)
		assert.Eq(t, 0, code)
		assert.Eq(t, filepath.Join(out, "apic", "SKILL.md")+"\n", stdout)
	})

	t.Run("broken config file", func(t *testing.T) {
		dir := t.TempDir()
		assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("sites: [\n"), 0600)))
		t.Setenv("APIC_CONFIG_DIR", dir)
		out := t.TempDir()
		code, stdout := runSkill(t, BuildInfo{}, "skill", "--out", out)
		assert.Eq(t, 0, code)
		assert.Eq(t, filepath.Join(out, "apic", "SKILL.md")+"\n", stdout)
	})
}

// TestSkillSelfNoLeak generates the skill inside a config directory that holds
// a site, a base URL and a token; none of them may reach the output.
func TestSkillSelfNoLeak(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", cfg)
	assert.Require(t, assert.NoError(t, os.MkdirAll(filepath.Join(cfg, "defs"), 0755)))
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(cfg, "defs", "secretsite.yaml"),
		[]byte("operations:\n  ping:\n    get: /ping\n"), 0600)))
	conf := "version: 1\nsites:\n  secretsite:\n    source: {type: simple, file: defs/secretsite.yaml}\n" +
		"    default_env: prod\n    envs:\n      prod: {base_url: 'https://internal.example.test', protect: true}\n" +
		"    default_auth: ro\n    auths:\n      ro: {type: bearer, token: '${env:SECRETSITE_TOKEN}'}\n"
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte(conf), 0600)))
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(cfg, ".env"), []byte("SECRETSITE_TOKEN=token-value-secret\n"), 0600)))

	out := t.TempDir()
	code, stdout := runSkill(t, BuildInfo{}, "skill", "--out", out)
	assert.Eq(t, 0, code)
	data, err := os.ReadFile(strings.TrimSpace(stdout))
	assert.Require(t, assert.NoError(t, err))
	text := string(data)
	for _, s := range []string{"secretsite", "internal.example.test", "token-value-secret", "SECRETSITE_TOKEN", cfg, "ping"} {
		assert.NotContains(t, text, s)
	}
}

// TestSkillSelfDrift keeps the static template in step with the options and
// exit codes the binary actually implements.
func TestSkillSelfDrift(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	out := t.TempDir()
	code, stdout := runSkill(t, BuildInfo{}, "skill", "--out", out)
	assert.Eq(t, 0, code)
	data, err := os.ReadFile(strings.TrimSpace(stdout))
	assert.Require(t, assert.NoError(t, err))
	text := string(data)

	t.Run("global options", func(t *testing.T) {
		flags := globalSpec().Flags
		assert.Require(t, assert.Gte(t, len(flags), 10), "globalSpec returned too few options")
		for name := range flags {
			assert.Contains(t, text, name, "global option "+name+" is missing from the template")
		}
	})

	t.Run("exit codes", func(t *testing.T) {
		codes := []int{
			model.ExitOK, model.ExitLocal, model.ExitUsage, model.ExitNetwork,
			model.ExitHTTP4xx, model.ExitHTTP5xx, model.ExitProtected, model.ExitPostProcess,
		}
		for _, c := range codes {
			row := "| " + strconv.Itoa(c) + " |"
			assert.Contains(t, text, row, "exit code row "+row+" is missing from the table")
		}
	})
}

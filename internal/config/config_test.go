package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
)

func testDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	return dir
}

func putFile(t *testing.T, file, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func localError(t *testing.T, err error, parts ...string) {
	t.Helper()
	var coded *model.CodedError
	assert.Require(t, assert.True(t, errors.As(err, &coded)))
	assert.Eq(t, model.ExitLocal, coded.Code())
	for _, part := range parts {
		assert.Contains(t, err.Error(), part)
	}
}

func TestDir(t *testing.T) {
	dir := testDir(t)
	got, err := config.Dir()
	assert.NoErr(t, err)
	assert.Eq(t, dir, got)
}

func TestLoadDesignExample(t *testing.T) {
	dir := testDir(t)
	data, err := os.ReadFile(filepath.Join("testdata", "config.yaml"))
	assert.Require(t, assert.NoErr(t, err))
	putFile(t, filepath.Join(dir, "config.yaml"), string(data))
	cfg, err := config.Load(dir)
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, 1, cfg.Version)
	assert.Eq(t, "30s", cfg.Defaults.Timeout)
	assert.Eq(t, 24*time.Hour, cfg.Sites["pet"].Source.TTL)
	assert.Eq(t, "data.token", cfg.Sites["pet"].Auths["me"].Login.Token.Path)
	assert.Eq(t, "Authorization", cfg.Sites["pet"].Auths["me"].Login.Apply.Header)
	assert.Eq(t, filepath.Join(dir, "defs", "demo.yaml"), cfg.Sites["demo"].Source.File)
	assert.Eq(t, filepath.Join(dir, "config.yaml"), cfg.Sites["pet"].Origin)
	assert.Eq(t, 0, len(config.Check(cfg)))
}

func TestLoadSitesFilesAndErrors(t *testing.T) {
	for _, tc := range []struct{ name, main, siteFile, siteContent, match, site string }{
		{"site file", "", "sites/pet.yaml", "source: {type: simple, file: defs/pet.yaml}\n", "", "pet"},
		{"duplicate", "sites:\n  pet: {}\n", "sites/pet.yaml", "source: {type: simple}\n", "duplicate", "pet"},

		{"invalid", "", "sites/Bad_name.yaml", "source: {type: simple}\n", "invalid", "Bad_name"},
		{"invalid main", "sites:\n  bad_name: {}\n", "", "", "invalid", "bad_name"},
		{"bad version", "version: 2\n", "", "", "version", "<config>"},
		{"bad field", "sites:\n  pet:\n    envs:\n      dev: {protect: invalid}\n", "", "", "pet", "pet"},
		{"null site", "sites:\n  pet: null\n", "", "", "null", "pet"},
		{"bad YAML", "sites: [\n", "", "", "", "<config>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testDir(t)
			if tc.main != "" {
				putFile(t, filepath.Join(dir, "config.yaml"), tc.main)
			}
			if tc.siteFile != "" {
				putFile(t, filepath.Join(dir, tc.siteFile), tc.siteContent)
			}
			cfg, err := config.Load(dir)
			if tc.match != "" || tc.name == "bad YAML" {
				localError(t, err, tc.site, tc.match)
				return
			}
			assert.Require(t, assert.NoErr(t, err))
			assert.Eq(t, filepath.Join(dir, "sites", "pet.yaml"), cfg.Sites["pet"].Origin)
			assert.Eq(t, filepath.Join(dir, "defs", "pet.yaml"), cfg.Sites["pet"].Source.File)
		})
	}
}

func TestReservedSiteSkipped(t *testing.T) {
	for _, tc := range []struct {
		name, main, siteFile string
	}{
		{"config.yaml", "version: 1\nsites:\n  skill: {source: {type: simple, file: defs/skill.yaml}}\n  pet: {source: {type: simple, file: defs/pet.yaml}}\n", ""},
		{"sites/skill.yaml", "version: 1\nsites:\n  pet: {source: {type: simple, file: defs/pet.yaml}}\n", "source: {type: simple, file: defs/skill.yaml}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testDir(t)
			putFile(t, filepath.Join(dir, "config.yaml"), tc.main)
			if tc.siteFile != "" {
				putFile(t, filepath.Join(dir, tc.name), tc.siteFile)
			}
			cfg, err := config.Load(dir)
			assert.Require(t, assert.NoErr(t, err))
			assert.True(t, cfg.Sites["skill"] == nil)
			assert.True(t, cfg.Sites["pet"] != nil)
			assert.Eq(t, []string{`site "skill" uses a reserved name and is skipped; rename it`}, cfg.Warnings)
		})
	}
}

func TestLoadMissingConfig(t *testing.T) {
	for _, content := range []string{"<missing>", "", "{}"} {
		t.Run(content, func(t *testing.T) {
			dir := testDir(t)
			if content != "<missing>" {
				putFile(t, filepath.Join(dir, "config.yaml"), content)
			}
			cfg, err := config.Load(dir)
			assert.Require(t, assert.NoErr(t, err))
			assert.Eq(t, 1, cfg.Version)
			assert.Eq(t, 0, len(cfg.Sites))
		})
	}
}

func TestDotenvAndLookup(t *testing.T) {
	dir := testDir(t)
	_, wasExported := os.LookupEnv("APIC_T02_NOT_EXPORTED")
	putFile(t, filepath.Join(dir, ".env"), "# comment\nexport PET_USER='single quoted'\nPET_PASS=\"double quoted\" # comment\nMixed=lower # trailing\nMIXED=upper\nINLINE=part#value\nEMPTY= # comment\nAPIC_T02_NOT_EXPORTED=only-in-file\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	for _, tc := range []struct{ key, want string }{
		{"PET_USER", "single quoted"}, {"PET_PASS", "double quoted"},
		{"Mixed", "lower"}, {"MIXED", "upper"}, {"INLINE", "part#value"}, {"EMPTY", ""},
	} {
		t.Run(tc.key, func(t *testing.T) {
			value, ok := env.Lookup(tc.key)
			assert.True(t, ok)
			assert.Eq(t, tc.want, value)
		})
	}
	_, ok := env.Lookup("mixed")
	assert.False(t, ok)
	t.Setenv("PET_USER", "os override")
	value, ok := env.Lookup("PET_USER")
	assert.True(t, ok)
	assert.Eq(t, "os override", value)
	// The parser must not export a key that was absent before loading.
	_, exported := os.LookupEnv("APIC_T02_NOT_EXPORTED")
	assert.Eq(t, wasExported, exported)
}

func TestLoadEnvOnlyFromDirectory(t *testing.T) {
	dir := testDir(t)
	cwd := t.TempDir()
	t.Chdir(cwd)
	putFile(t, filepath.Join(cwd, ".env"), "APIC_T02_CWD_ONLY=wrong\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	_, ok := env.Lookup("APIC_T02_CWD_ONLY")
	assert.False(t, ok)
	_, err = config.LoadEnv("")
	localError(t, err, "config directory")
}

func TestExpandAndResolveAuth(t *testing.T) {
	dir := testDir(t)
	putFile(t, filepath.Join(dir, ".env"), "AUTH_TOKEN=secret value\nLOGIN_PASS=pass value\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	var sec model.Secrets
	for _, tc := range []struct{ name, input, want, missing string }{
		{"existing", "Bearer ${env:AUTH_TOKEN}", "Bearer secret value", ""},
		{"missing", "${env:NOT_HERE}", "", "NOT_HERE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := config.Expand(tc.input, env, &sec)
			if tc.missing != "" {
				localError(t, err, tc.missing)
				return
			}
			assert.NoErr(t, err)
			assert.Eq(t, tc.want, got)
		})
	}
	auth := model.AuthSpec{Type: "login", Login: model.LoginSpec{Body: map[string]any{"outer": map[string]any{"password": "${env:LOGIN_PASS}"}, "items": []any{"${env:AUTH_TOKEN}"}}}}
	site := &model.Site{Auths: map[string]model.AuthSpec{"me": auth, "other": {Token: "${env:NOT_HERE}"}}}
	resolved, err := config.ResolveAuth(site, "me", env, &sec)
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, "pass value", resolved.Login.Body["outer"].(map[string]any)["password"])
	assert.Eq(t, "${env:LOGIN_PASS}", site.Auths["me"].Login.Body["outer"].(map[string]any)["password"])
	assert.Eq(t, "***", (&model.Redactor{Secrets: &sec}).Redact("pass value"))
	_, err = config.ResolveAuth(site, "other", env, &sec)
	localError(t, err, "NOT_HERE")
}

func TestResolveAuthRegistersOnlyCredentials(t *testing.T) {
	dir := testDir(t)
	putFile(t, filepath.Join(dir, ".env"), "B_USER=demo\nB_PASS=pass-secret\n")
	env, err := config.LoadEnv(dir)
	assert.Require(t, assert.NoErr(t, err))
	var sec model.Secrets
	site := &model.Site{Auths: map[string]model.AuthSpec{
		"me": {Type: "basic", Username: "${env:B_USER}", Password: "${env:B_PASS}"},
	}}
	resolved, err := config.ResolveAuth(site, "me", env, &sec)
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, "demo", resolved.Username)
	red := &model.Redactor{Secrets: &sec}
	// The username is not a credential: a URL path containing it stays readable.
	assert.Eq(t, "/job/demo-job ***", red.Redact("/job/demo-job pass-secret"))
}

func TestStateRoundTrip(t *testing.T) {
	dir := testDir(t)
	empty, err := config.LoadState(dir)
	assert.NoErr(t, err)
	assert.Eq(t, 0, len(empty.Selections))
	st := &config.State{Selections: map[string]config.Selection{"pet": {Env: "dev", Auth: "me"}}, Links: map[string]config.LinkRec{"demo": {Path: "demo", Hash: "abc", Site: "pet"}}, ManagedSites: []string{"pet"}}
	assert.Require(t, assert.NoErr(t, config.SaveState(dir, st)))
	loaded, err := config.LoadState(dir)
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, st.Selections["pet"], loaded.Selections["pet"])
	assert.Eq(t, st.Links["demo"], loaded.Links["demo"])
	assert.Eq(t, st.ManagedSites, loaded.ManagedSites)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "state.yaml"))
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestCheckPlaintext(t *testing.T) {
	testDir(t)
	cfg := &model.Config{Sites: map[string]*model.Site{"pet": {Auths: map[string]model.AuthSpec{
		"me": {Token: "literal token", Password: "${env:SAFE}", Login: model.LoginSpec{Body: map[string]any{"payload": map[string]any{"secretKey": "plain", "accessToken": "${env:TOKEN}"}}}},
	}}}}
	warnings := config.Check(cfg)
	assert.Eq(t, 2, len(warnings))
	assert.True(t, strings.Contains(strings.Join(warnings, " "), "secretKey"))
	assert.False(t, strings.Contains(strings.Join(warnings, " "), "literal token"))
}

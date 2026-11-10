package commands_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/commands"
)

func runCommand(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	assert.Require(t, assert.NoError(t, err))
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := commands.Run(commands.BuildInfo{}, args)
	assert.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	assert.NoError(t, err)
	assert.NoError(t, r.Close())
	return code, string(out)
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "spec", "testdata", name))
	assert.Require(t, assert.NoError(t, err))
	return path
}

func TestSiteLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	path := fixture(t, "min-v2.json")
	code, out := runCommand(t, "site", "add", "demo", "--spec", path)
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "demo")
	assert.Contains(t, out, "demo.local")
	info, err := os.Stat(filepath.Join(dir, "sites", "demo.yaml"))
	assert.Require(t, assert.NoError(t, err))
	if runtime.GOOS != "windows" {
		assert.Eq(t, os.FileMode(0600), info.Mode().Perm())
	}
	code, _ = runCommand(t, "site", "add", "demo", "--spec", path)
	assert.Eq(t, 1, code)
	code, out = runCommand(t, "site", "ls")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "demo")
	assert.Contains(t, out, "openapi")
	code, out = runCommand(t, "site", "ls", "-o", "json")
	assert.Eq(t, 0, code)
	var sites []map[string]any
	assert.NoError(t, json.Unmarshal([]byte(out), &sites))
	assert.Len(t, sites, 1)
	code, out = runCommand(t, "ops", "demo", "-o", "json")
	assert.Eq(t, 0, code)
	var ops []map[string]any
	assert.NoError(t, json.Unmarshal([]byte(out), &ops))
	assert.Len(t, ops, 4)
	code, out = runCommand(t, "ops", "demo", "--match", "GET PET")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "get-pet")
	code, out = runCommand(t, "desc", "demo", "get-pet")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "/api/v1/pets/{id}")
	assert.Contains(t, out, "verbose")
	assert.Contains(t, out, "name")
	code, out = runCommand(t, "desc", "demo", "get-pet", "-o", "json")
	assert.Eq(t, 0, code)
	var op map[string]any
	assert.NoError(t, json.Unmarshal([]byte(out), &op))
	code, _ = runCommand(t, "desc", "demo", "missing")
	assert.Eq(t, 2, code)
	code, out = runCommand(t, "site", "refresh", "demo")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "4")
	tokenDir := filepath.Join(dir, "cache", "tokens")
	assert.NoError(t, os.MkdirAll(tokenDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(tokenDir, "demo.default.auth.json"), []byte("test"), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(tokenDir, "demo-other.default.auth.json"), []byte("test"), 0600))
	code, _ = runCommand(t, "site", "rm", "demo")
	assert.Eq(t, 0, code)
	_, err = os.Stat(filepath.Join(dir, "sites", "demo.yaml"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(tokenDir, "demo.default.auth.json"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(tokenDir, "demo-other.default.auth.json"))
	assert.NoError(t, err)
	code, _ = runCommand(t, "site", "rm", "demo")
	assert.Eq(t, 1, code)
}

func TestSiteSimpleAndMissingBase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	path := fixture(t, "jenkins.yaml")
	code, _ := runCommand(t, "site", "add", "demo", "--def", path)
	assert.Eq(t, 1, code)
	_, err := os.Stat(filepath.Join(dir, "sites", "demo.yaml"))
	assert.True(t, os.IsNotExist(err))
	code, _ = runCommand(t, "site", "add", "demo", "--def", path, "--base-url", "http://example.test")
	assert.Eq(t, 0, code)
	code, out := runCommand(t, "ops", "demo")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "job-info")
	code, _ = runCommand(t, "site", "add", "no-source")
	assert.Eq(t, 2, code)
	data, err := os.ReadFile(fixture(t, "min-v2.json"))
	assert.Require(t, assert.NoError(t, err))
	withoutHost := strings.Replace(string(data), `"host": "demo.local",`, "", 1)
	noHostFile := filepath.Join(dir, "no-host.json")
	assert.NoError(t, os.WriteFile(noHostFile, []byte(withoutHost), 0600))
	code, _ = runCommand(t, "site", "add", "no-host", "--spec", noHostFile)
	assert.Eq(t, 1, code)
	_, err = os.Stat(filepath.Join(dir, "sites", "no-host.yaml"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "cache", "specs", "no-host.raw"))
	assert.True(t, os.IsNotExist(err))
	code, _ = runCommand(t, "site", "add", "invalid", "--spec", filepath.Join(dir, "absent.json"))
	assert.Eq(t, 1, code)
	_, err = os.Stat(filepath.Join(dir, "sites", "invalid.yaml"))
	assert.True(t, os.IsNotExist(err))
}

func TestConfigShowCheckAndUserSite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	t.Setenv("APIX_TEST_SECRET", "")
	// Unset rather than set empty, so the .env value takes effect.
	assert.NoError(t, os.Unsetenv("APIX_TEST_SECRET"))
	config := "version: 1\nsites:\n  demo:\n    source: {type: openapi, url: 'https://example.test/api'}\n    envs:\n      default: {base_url: 'http://placeholder-secret-value'}\n    auths:\n      default: {type: bearer, token: '${env:APIX_TEST_SECRET}'}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APIX_TEST_SECRET=placeholder-secret-value\n"), 0600))
	for _, args := range [][]string{{"site", "show", "demo"}, {"config", "show"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			code, out := runCommand(t, args...)
			assert.Eq(t, 0, code)
			assert.False(t, strings.Contains(out, "placeholder-secret-value"))
			assert.Contains(t, out, "${env:APIX_TEST_SECRET}")
		})
	}
	code, _ := runCommand(t, "site", "rm", "demo")
	assert.Eq(t, 1, code)
	code, out := runCommand(t, "config", "path")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, dir)
	code, out = runCommand(t, "config", "check")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "ok")
}

func TestSiteFailureRecovery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	cacheDir := filepath.Join(dir, "cache", "specs")
	assert.NoError(t, os.MkdirAll(cacheDir, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(cacheDir, "demo.raw"), []byte("stale"), 0600))
	code, _ := runCommand(t, "site", "add", "demo", "--spec", fixture(t, "min-v2.json"))
	assert.Eq(t, 0, code)
	blocker := filepath.Join(dir, "cache", "tokens", "demo.blocked")
	assert.NoError(t, os.MkdirAll(blocker, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(blocker, "child"), []byte("x"), 0600))
	code, _ = runCommand(t, "site", "rm", "demo")
	assert.Eq(t, 1, code)
	_, err := os.Stat(filepath.Join(dir, "sites", "demo.yaml"))
	assert.NoError(t, err)
	assert.NoError(t, os.Remove(filepath.Join(blocker, "child")))
	code, _ = runCommand(t, "site", "rm", "demo")
	assert.Eq(t, 0, code)
}

func TestConfigPlainCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	cfg := "version: 1\nsites:\n  demo:\n    source: {type: openapi, url: 'https://example.test/sample-password/ab'}\n    auths:\n      demo: {type: basic, username: sample-user, password: sample-password, token: ab, value: sample-value, login: {body: {pin: 123, secret: sample-body}}}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	for _, args := range [][]string{{"site", "show", "demo"}, {"config", "show"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			code, out := runCommand(t, args...)
			assert.Eq(t, 0, code)
			for _, secret := range []string{"sample-user", "sample-password", "sample-value", "sample-body", "123", "token: ab", "/ab"} {
				assert.False(t, strings.Contains(out, secret))
			}
			assert.Contains(t, out, "***")
		})
	}
	code, out := runCommand(t, "config", "check")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "plaintext credential")
}

func TestConfigShortReferencedSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	t.Setenv("APIX_SHORT_SECRET", "xy")
	cfg := "version: 1\nsites:\n  demo:\n    source: {type: openapi, url: 'https://example.test/xy'}\n    auths:\n      demo: {type: bearer, token: '${env:APIX_SHORT_SECRET}'}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	for _, args := range [][]string{{"site", "show", "demo"}, {"config", "show"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			code, out := runCommand(t, args...)
			assert.Eq(t, 0, code)
			assert.False(t, strings.Contains(out, "/xy"))
			assert.Contains(t, out, "${env:APIX_SHORT_SECRET}")
		})
	}
}

func TestSiteRefreshStaticAuth(t *testing.T) {
	for _, tc := range []struct {
		name, auth, header, value string
	}{
		{"basic", "{type: basic, username: demo, password: sample-secret}", "Authorization", "Basic ZGVtbzpzYW1wbGUtc2VjcmV0"},
		{"apikey", "{type: apikey, in: header, name: X-Api-Key, value: sample-secret}", "X-Api-Key", "sample-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("APIX_CONFIG_DIR", dir)
			data, err := os.ReadFile(fixture(t, "min-v2.json"))
			assert.Require(t, assert.NoError(t, err))
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get(tc.header) != tc.value {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if _, err := w.Write(data); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			cfg := fmt.Sprintf("version: 1\nsites:\n  demo:\n    source: {type: openapi, url: %q, auth: example}\n    auths:\n      example: %s\n", server.URL, tc.auth)
			assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
			code, _ := runCommand(t, "site", "refresh", "demo")
			assert.Eq(t, 0, code)
			assert.Eq(t, 1, calls)
		})
	}
}

func TestSiteRefreshAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	data, err := os.ReadFile(fixture(t, "min-v2.json"))
	assert.Require(t, assert.NoError(t, err))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer sample-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	config := fmt.Sprintf("version: 1\nsites:\n  demo:\n    source: {type: openapi, url: %q, auth: example}\n    auths:\n      example: {type: bearer, token: '${env:APIX_SAMPLE_TOKEN}'}\n", server.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600))
	t.Setenv("APIX_SAMPLE_TOKEN", "sample-secret")
	code, _ := runCommand(t, "site", "refresh", "demo")
	assert.Eq(t, 0, code)
	assert.Eq(t, 1, requests)
	config = strings.Replace(config, "type: bearer", "type: login", 1)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0600))
	code, _ = runCommand(t, "site", "refresh", "demo")
	assert.Eq(t, 1, code)
	assert.Eq(t, 1, requests)
}

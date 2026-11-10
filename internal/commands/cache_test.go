package commands_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// openAPIServer serves min-v2.json until failing is set.
func openAPIServer(t *testing.T, failing *bool, hits *int) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(fixture(t, "min-v2.json"))
	assert.Require(t, assert.NoError(t, err))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if *failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSiteRefreshStaleCacheExit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	failing, hits := false, 0
	srv := openAPIServer(t, &failing, &hits)
	cfg := fmt.Sprintf("version: 1\nsites:\n  demo:\n    source: {type: openapi, url: %q, ttl: 1ns}\n    default_env: e\n    envs: {e: {base_url: 'http://x.test'}}\n", srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	code, _, errOut := runIO(t, "site", "refresh", "demo")
	assert.Eq(t, 0, code, errOut)
	failing = true
	// explicit refresh that falls back to the stale cache fails with exit 1.
	code, out, errOut := runIO(t, "site", "refresh", "demo")
	assert.Eq(t, 1, code)
	assert.Contains(t, errOut, "refresh")
	assert.Eq(t, "", out)
	// implicit use of the stale cache keeps exit 0 with a warning.
	code, out, errOut = runIO(t, "ops", "demo")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "get-pet")
	assert.Contains(t, errOut, "warning")
}

func TestSiteCacheReadIgnoresAuthAndTLS(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	t.Setenv("APIX_SPEC_TOKEN", "spec-secret-token")
	failing, hits := false, 0
	srv := openAPIServer(t, &failing, &hits)
	cfg := fmt.Sprintf("version: 1\nsites:\n  demo:\n    source: {type: openapi, url: %q, auth: spec}\n    default_env: e\n    envs: {e: {base_url: 'http://x.test'}}\n    auths:\n      spec: {type: bearer, token: '${env:APIX_SPEC_TOKEN}'}\n", srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	code, _, errOut := runIO(t, "site", "refresh", "demo")
	assert.Eq(t, 0, code, errOut)
	n := hits
	// the auth variable disappears and the CA file is missing: the valid cache still serves.
	assert.NoError(t, os.Unsetenv("APIX_SPEC_TOKEN"))
	cfg = fmt.Sprintf("version: 1\nsites:\n  demo:\n    source: {type: openapi, url: %q, auth: spec}\n    default_env: e\n    envs: {e: {base_url: 'http://x.test', ca_file: /no/such/ca.pem}}\n    auths:\n      spec: {type: bearer, token: '${env:APIX_SPEC_TOKEN}'}\n", srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	code, out, errOut := runIO(t, "ops", "demo")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "get-pet")
	code, _, errOut = runIO(t, "desc", "demo", "get-pet")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, n, hits)
	// an actual fetch still reports the problem.
	code, _, _ = runIO(t, "site", "refresh", "demo")
	assert.Eq(t, 1, code)
}

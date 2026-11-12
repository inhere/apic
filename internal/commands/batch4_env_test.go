package commands_test

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/commands"
	"github.com/inhere/apic/internal/link"
)

const batch4Def = `operations:
  list-users:
    get: /users
`

// batch4Setup writes config.yaml (a format string with one %[1]q for the
// definition path), the definition and an optional state.yaml into a fresh
// config directory.
func batch4Setup(t *testing.T, cfg, state string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	def := filepath.Join(dir, "demo-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(batch4Def), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(fmt.Sprintf(cfg, def)), 0600))
	if state != "" {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "state.yaml"), []byte(state), 0600))
	}
	return dir
}

// clearCfg declares the site "demo"; goneCfg is a configuration that no longer
// has it, for the residue case.
const (
	clearCfg = `version: 1
sites:
  demo:
    source: {type: simple, file: %[1]q}
    default_env: dev
    envs:
      dev: {base_url: http://example.test}
      prod: {base_url: http://example.test}
    auths:
      tok: {type: bearer, token: token-value}
`
	goneCfg = `version: 1
sites:
  pet:
    source: {type: simple, file: %[1]q}
    default_env: test
    envs:
      test: {base_url: http://example.test}
`
)

func TestUseClear(t *testing.T) {
	t.Run("clears the saved selection", func(t *testing.T) {
		dir := batch4Setup(t, clearCfg, "")
		code, _, errOut := runIO(t, "use", "demo", "--env", "prod", "--auth", "tok")
		assert.Eq(t, 0, code, errOut)
		code, out, errOut := runIO(t, "use", "demo")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "env: prod\nauth: tok (saved)\n")

		code, out, errOut = runIO(t, "use", "demo", "--clear")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "env: dev\nauth: (none)\n")
		code, bare, errOut := runIO(t, "use", "demo")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, bare, out)

		state, err := os.ReadFile(filepath.Join(dir, "state.yaml"))
		assert.Require(t, assert.NoError(t, err))
		assert.NotContains(t, string(state), "demo")
	})

	t.Run("no saved selection", func(t *testing.T) {
		batch4Setup(t, clearCfg, "")
		code, out, errOut := runIO(t, "use", "demo", "--clear")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "env: dev\nauth: (none)\n")
	})

	t.Run("with --env or --auth", func(t *testing.T) {
		batch4Setup(t, clearCfg, "")
		for name, extra := range map[string][]string{"env": {"--env", "dev"}, "auth": {"--auth", "tok"}} {
			t.Run(name, func(t *testing.T) {
				code, _, _ := runIO(t, append([]string{"use", "demo", "--clear"}, extra...)...)
				assert.Eq(t, 2, code)
			})
		}
	})

	t.Run("residue of a removed site", func(t *testing.T) {
		dir := batch4Setup(t, goneCfg, "selections:\n  demo: {env: prod, auth: tok}\nlinks:\n  petctl: {path: bin/petctl, hash: abc, site: pet}\n")
		code, out, errOut := runIO(t, "use", "demo", "--clear")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "cleared")
		state, err := os.ReadFile(filepath.Join(dir, "state.yaml"))
		assert.Require(t, assert.NoError(t, err))
		assert.NotContains(t, string(state), "demo")
		// state.links is not part of clearing.
		assert.Contains(t, string(state), "petctl")
	})

	t.Run("unknown site without residue", func(t *testing.T) {
		batch4Setup(t, goneCfg, "")
		code, _, errOut := runIO(t, "use", "demo", "--clear")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, `site "demo" does not exist`)
		// Other uses still require the site.
		code, _, errOut = runIO(t, "use", "demo")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, `site "demo" does not exist`)
	})
}

func TestPrescanEnv(t *testing.T) {
	spec := commands.GlobalSpec{Flags: map[string]bool{"--env": true, "--auth": true, "--yes": false, "-o": true}}
	sites := []string{"pet", "other"}
	cases := []struct {
		name string
		args []string
		site string
		env  string
	}{
		{"before the site", []string{"--env", "prod", "pet", "ls"}, "pet", "prod"},
		{"after the site", []string{"pet", "ls", "--env", "alt"}, "pet", "alt"},
		{"both, last wins", []string{"--env", "prod", "pet", "ls", "--env", "alt"}, "pet", "alt"},
		{"equals form", []string{"--env=prod", "pet"}, "pet", "prod"},
		{"equals form after the site", []string{"pet", "--env=alt", "ls"}, "pet", "alt"},
		{"api form", []string{"--api", "pet", "--env", "alt", "ls"}, "pet", "alt"},
		{"api form, env after the site", []string{"--api", "pet", "ls", "--env=alt"}, "pet", "alt"},
		{"after the terminator", []string{"pet", "--", "--env", "alt"}, "pet", ""},
		{"no site", []string{"site", "ls", "--env", "alt"}, "", ""},
		{"no env", []string{"pet", "ls"}, "pet", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site, _, env, err := commands.Prescan(tc.args, spec, sites)
			assert.Require(t, assert.NoError(t, err))
			assert.Eq(t, tc.site, site)
			assert.Eq(t, tc.env, env)
		})
	}

	// The prescan takes the last --env, so gcli must also keep the last value of
	// a repeated option. The base path of the dry-run URL shows the value gcli
	// ended up with.
	t.Run("gcli keeps the last --env", func(t *testing.T) {
		setup(t)
		t.Run("after the site", func(t *testing.T) {
			code, out, errOut := runIO(t, "pet", "list-users", "--env", "alt", "--env", "test", "--dry-run")
			assert.Eq(t, 0, code, errOut)
			assert.Contains(t, out, "/test/users")
			assert.NotContains(t, out, "/alt/users")
		})
		t.Run("across the site name", func(t *testing.T) {
			code, out, errOut := runIO(t, "--env", "alt", "pet", "list-users", "--env", "test", "--dry-run")
			assert.Eq(t, 0, code, errOut)
			assert.Contains(t, out, "/test/users")
			assert.NotContains(t, out, "/alt/users")
		})
	})
}

// batch4Spec is the OpenAPI document the TLS server serves.
const batch4Spec = `{
  "openapi": "3.0.3",
  "info": {"title": "Demo API", "version": "1.0"},
  "paths": {"/me": {"get": {"operationId": "me", "summary": "Me", "responses": {"200": {"description": "OK"}}}}}
}`

// batch4Server is one TLS server for both the definition and the API. It counts
// the definition requests, so a test can tell whether a fetch happened.
type batch4Server struct {
	srv *httptest.Server
	pem string
	mu  sync.Mutex
	hit int
}

func newBatch4Server(t *testing.T) *batch4Server {
	t.Helper()
	b := &batch4Server{}
	b.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/spec.json" {
			b.mu.Lock()
			b.hit++
			b.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, batch4Spec)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(b.srv.Close)
	b.pem = filepath.Join(t.TempDir(), "ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b.srv.Certificate().Raw})
	assert.NoError(t, os.WriteFile(b.pem, cert, 0600))
	return b
}

func (b *batch4Server) specHits() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hit
}

// config writes a fresh configuration directory for site "dh". Only the
// "secure" environment trusts the server certificate; "plain" does not, so a
// fetch with it fails. A new directory has no cache, so every run fetches.
func (b *batch4Server) config(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	cfg := fmt.Sprintf(`version: 1
sites:
  dh:
    source: {type: openapi, url: %q}
    default_env: plain
    envs:
      plain: {base_url: %q}
      secure: {base_url: %q, ca_file: %q}
`, b.srv.URL+"/spec.json", b.srv.URL, b.srv.URL, b.pem)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	return dir
}

func TestEnvFetch(t *testing.T) {
	b := newBatch4Server(t)

	t.Run("site call with the env after the site", func(t *testing.T) {
		b.config(t)
		before := b.specHits()
		// `dh --env secure ops` is not a valid call: ops is not a subcommand of a
		// site, so the site call itself lists the operations.
		code, out, errOut := runIO(t, "dh", "--env", "secure")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "me")
		assert.Eq(t, before+1, b.specHits())
	})

	for name, args := range map[string][]string{
		"env before the site":   {"--env", "secure", "dh", "me", "--dry-run"},
		"env after the site":    {"dh", "me", "--env", "secure", "--dry-run"},
		"api form":              {"--api", "dh", "--env", "secure", "me", "--dry-run"},
		"link script arguments": {"--api", "dh", "--env", "secure", "me", "--dry-run"},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "link script arguments" {
				// A generated shortcut runs `apic --api <site> --env <env> "$@"`.
				assert.Contains(t, link.Scripts("linux", "dh", "secure", "")[""], "--api dh --env secure")
			}
			b.config(t)
			before := b.specHits()
			code, out, errOut := runIO(t, args...)
			assert.Eq(t, 0, code, errOut)
			assert.Contains(t, out, b.srv.URL)
			assert.Eq(t, before+1, b.specHits())
		})
	}

	t.Run("ops after the site is not a subcommand", func(t *testing.T) {
		// The plan's `dh --env secure ops` form: ops is not a subcommand of a site,
		// so the site call itself is what lists the operations.
		b.config(t)
		code, _, errOut := runIO(t, "dh", "--env", "secure", "ops")
		assert.Eq(t, 2, code)
		assert.Contains(t, errOut, `unknown operation "ops"`)
	})

	t.Run("no env fails to fetch", func(t *testing.T) {
		b.config(t)
		before := b.specHits()
		code, _, errOut := runIO(t, "dh")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "fetch definition")
		// The TLS handshake fails before the handler runs.
		assert.Eq(t, before, b.specHits())
	})

	t.Run("the last env wins", func(t *testing.T) {
		b.config(t)
		before := b.specHits()
		code, out, errOut := runIO(t, "--env", "plain", "dh", "me", "--env", "secure", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, b.srv.URL)
		assert.Eq(t, before+1, b.specHits())

		b.config(t)
		before = b.specHits()
		code, _, errOut = runIO(t, "--env", "secure", "dh", "me", "--env", "plain", "--dry-run")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "fetch definition")
		assert.Eq(t, before, b.specHits())
	})

	for name, args := range map[string][]string{
		"listing": {"dh", "--env", "bogus"},
		"call":    {"dh", "me", "--env", "bogus"},
	} {
		t.Run("unknown env, "+name, func(t *testing.T) {
			b.config(t)
			before := b.specHits()
			code, _, errOut := runIO(t, args...)
			assert.Eq(t, 1, code)
			assert.Contains(t, errOut, `site "dh" has no environment "bogus"`)
			// The environment is checked before any fetch.
			assert.Eq(t, before, b.specHits())
		})
	}

	t.Run("saved env no longer declared", func(t *testing.T) {
		dir := b.config(t)
		code, _, errOut := runIO(t, "dh", "--env", "secure")
		assert.Eq(t, 0, code, errOut)
		state := "selections:\n  dh: {env: gone}\n"
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "state.yaml"), []byte(state), 0600))
		before := b.specHits()
		code, out, errOut := runIO(t, "ops", "dh")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "me")
		// The cached index is used, as before this change.
		assert.Eq(t, before, b.specHits())
	})
}

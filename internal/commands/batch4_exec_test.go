package commands_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// The two values the helper scripts print and the two marker files they append
// to. Both values are long enough to be registered as secrets: a value shorter
// than 4 characters is not redacted.
const (
	execRefToken  = "tok-exec-ref-8c1"
	execRefOther  = "tok-exec-other-4b7"
	execRefMarker = "marker-tok.txt"
	execRefUnused = "marker-other.txt"
)

// writeExecRefScript writes a sh script into dir that marks every run in marker
// and prints value on stdout. The scripts are run as `sh <name>` from the config
// directory, so the marker path is relative to it.
func writeExecRefScript(t *testing.T, dir, name, marker, value string) {
	t.Helper()
	body := "#!/bin/sh\necho run >> " + marker + "\nprintf '%s\\n' " + value + "\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

// setupExecRef writes a site "demo" whose selected auth takes its header value
// from a script, with two more auths that must not run unless they are selected.
func setupExecRef(t *testing.T) (*fakeAPI, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	f := newFakeAPI(t)
	def := filepath.Join(dir, "demo-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte("operations:\n  list-users:\n    get: /users\n"), 0o600))
	writeExecRefScript(t, dir, "tok.sh", execRefMarker, execRefToken)
	writeExecRefScript(t, dir, "other.sh", execRefUnused, execRefOther)
	cfg := fmt.Sprintf(`version: 1
sites:
  demo:
    source: {type: simple, file: %q}
    default_env: dev
    default_auth: pat
    envs:
      dev: {base_url: %q}
    auths:
      pat:
        type: apikey
        in: header
        name: X-Token
        value: '${exec:sh tok.sh}'
      mixed:
        type: apikey
        in: header
        name: X-Mixed
        value: 'Bearer ${exec:sh other.sh}'
      unused:
        type: apikey
        in: header
        name: X-Unused
        value: '${exec:sh other.sh}'
`, def, f.srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	return f, dir
}

// TestExecRefCall covers the ${exec:} behaviour that needs a real command: it
// runs sh scripts, so it is skipped on Windows (the B6 real-env check covers
// that platform on the host).
func TestExecRefCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ${exec:} command cases run sh scripts")
	}

	t.Run("the server receives the expanded value", func(t *testing.T) {
		f, dir := setupExecRef(t)
		code, _, errOut := runIO(t, "demo", "list-users")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, execRefToken, f.last(t).Header.Get("X-Token"))
		// The script ran once; an auth that is not selected never runs.
		assert.Eq(t, 1, countLines(t, filepath.Join(dir, execRefMarker)))
		assert.Eq(t, 0, countLines(t, filepath.Join(dir, execRefUnused)))
	})

	t.Run("verbose output masks the value", func(t *testing.T) {
		f, dir := setupExecRef(t)
		code, out, errOut := runIO(t, "demo", "list-users", "-v")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, execRefToken, f.last(t).Header.Get("X-Token"))
		assert.Contains(t, errOut, "X-Token: ")
		for _, s := range []string{out, errOut} {
			assert.NotContains(t, s, execRefToken)
			assert.NotContains(t, s, "tok.sh")
		}
		assert.Eq(t, 1, countLines(t, filepath.Join(dir, execRefMarker)))
	})

	t.Run("dry-run prints the placeholder and runs nothing", func(t *testing.T) {
		f, dir := setupExecRef(t)
		code, out, errOut := runIO(t, "demo", "list-users", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		// A custom header shows the placeholder instead of the command text.
		assert.Contains(t, out, "X-Token: <exec>")
		assert.NotContains(t, out, execRefToken)
		assert.NotContains(t, out, "tok.sh")
		assert.Eq(t, 0, len(f.requests()))
		assert.Eq(t, 0, countLines(t, filepath.Join(dir, execRefMarker)))
	})

	t.Run("a mixed value keeps the surrounding text", func(t *testing.T) {
		f, dir := setupExecRef(t)
		code, out, errOut := runIO(t, "demo", "list-users", "--auth", "mixed", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "X-Mixed: Bearer <exec>")
		assert.NotContains(t, out, "other.sh")
		assert.Eq(t, 0, countLines(t, filepath.Join(dir, execRefUnused)))

		code, _, errOut = runIO(t, "demo", "list-users", "--auth", "mixed")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer "+execRefOther, f.last(t).Header.Get("X-Mixed"))
		assert.Eq(t, 0, countLines(t, filepath.Join(dir, execRefMarker)))
	})
}

// execRefSpecServer serves the definition and records the headers it received.
type execRefSpecServer struct {
	srv *httptest.Server
	mu  sync.Mutex
	hdr http.Header
	n   int
}

func newExecRefSpecServer(t *testing.T) *execRefSpecServer {
	t.Helper()
	s := &execRefSpecServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hdr, s.n = r.Header.Clone(), s.n+1
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, batch4Spec)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *execRefSpecServer) lastHeader(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hdr.Get(name)
}

func (s *execRefSpecServer) hits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// TestExecRefSourceAuth covers the source.auth entry: fetching the definition
// itself resolves the ${exec:} of the identity it uses.
func TestExecRefSourceAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ${exec:} command cases run sh scripts")
	}
	s := newExecRefSpecServer(t)
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	writeExecRefScript(t, dir, "tok.sh", execRefMarker, execRefToken)
	cfg := fmt.Sprintf(`version: 1
sites:
  rem:
    source: {type: openapi, url: %q, auth: pat}
    default_env: dev
    envs: {dev: {base_url: %q}}
    auths:
      pat: {type: apikey, in: header, name: X-Token, value: '${exec:sh tok.sh}'}
`, s.srv.URL+"/spec.json", s.srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))

	code, _, errOut := runIO(t, "site", "refresh", "rem")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, 1, s.hits())
	assert.Eq(t, execRefToken, s.lastHeader("X-Token"))
	assert.Eq(t, 1, countLines(t, filepath.Join(dir, execRefMarker)))
	assert.NotContains(t, errOut, execRefToken)
}

// execRefCheckCfg declares one ${exec:} per case: a credential value, a value
// mixed with ${env:} and text, a misplaced reference, an unclosed one and an
// exec auth whose command[0] is itself a reference.
const execRefCheckCfg = `version: 1
sites:
  demo:
    source: {type: simple, file: %[1]q}
    default_env: dev
    envs: {dev: {base_url: http://example.test}}
    auths:
      pat:
        type: apikey
        in: header
        name: X-Token
        value: '${exec:secret-tool-cmd --flag}'
      mixed:
        type: bearer
        token: 'Bearer ${env:PLAIN_UNSET_VAR}-${exec:other-tool}'
      wrong:
        type: bearer
        token: plain-token-value
        username: '${exec:misplaced-tool}'
      broken:
        type: apikey
        in: header
        name: X-Token
        value: '${exec:unclosed-tool'
      ex:
        type: exec
        exec: {command: ['${exec:wrapped-tool arg}', 'plain-arg']}
`

// TestExecRefConfigCheck covers config check: it reports every reference, never
// prints the command text and never runs anything (so it works on every OS).
func TestExecRefConfigCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	def := filepath.Join(dir, "demo-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte("operations:\n  ping: {get: /ping}\n"), 0o600))
	cfg := fmt.Sprintf(execRefCheckCfg, def)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))

	code, out, errOut := runIO(t, "config", "check")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "site demo auth pat value: runs ${exec:}")
	// A value mixing ${env:}, text and ${exec:} is a reference, not a literal.
	assert.Contains(t, out, "site demo auth mixed token: runs ${exec:}")
	assert.NotContains(t, out, "site demo auth mixed token: plaintext credential")
	assert.Contains(t, out, "site demo auth wrong username: ${exec:} is only allowed in credential values")
	assert.Contains(t, out, "site demo auth broken value: malformed ${exec:} reference")
	assert.Contains(t, out, "site demo auth ex exec.command[0]: ${exec:} is only allowed in credential values")
	assert.Contains(t, out, "site demo auth ex: runs ***")
	for _, command := range []string{"secret-tool-cmd", "--flag", "other-tool", "misplaced-tool", "unclosed-tool", "wrapped-tool", "plain-arg"} {
		assert.NotContains(t, out+errOut, command)
	}
}

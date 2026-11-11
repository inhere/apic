package commands_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const (
	sentLit = "SENTINEL_LIT_9f3"
	sentEnv = "SENTINEL_ENV_7c1"
	cmd0Dir = "cmd0_marker_5d2"
)

// buildExecStub compiles the auth package's stub into a directory whose name
// is a marker, so any leak of command[0] is detectable.
func buildExecStub(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), cmd0Dir)
	assert.NoError(t, os.MkdirAll(dir, 0o755))
	name := "stubx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		goBin = "go"
	}
	cmd := exec.Command(goBin, "build", "-o", out, ".")
	cmd.Dir = filepath.Join("..", "auth", "testdata", "stub")
	cmd.Env = os.Environ()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, b)
	}
	return out
}

// setupExec writes site "pet" with several exec auths; count is the file the
// stub appends to on every run.
func setupExec(t *testing.T) (f *fakeAPI, dir, stub, count string) {
	t.Helper()
	f, dir = setup(t)
	stub = buildExecStub(t)
	count = filepath.Join(t.TempDir(), "count")
	def := filepath.Join(dir, "pet-def.yaml")
	q := func(s string) string { return fmt.Sprintf("%q", s) }
	argv := func(ops ...string) string {
		parts := []string{q(stub), q("count=" + count), q(sentLit), q("${env:SENT}")}
		for _, o := range ops {
			parts = append(parts, q(o))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	cfg := fmt.Sprintf(`version: 1
sites:
  pet:
    source: {type: simple, file: %s}
    default_env: test
    default_auth: ex
    envs:
      test: {base_url: %s, base_path: /test}
    auths:
      ex:
        type: exec
        exec: {command: %s, ttl: 1h, env: {STUB_FOO: '${env:SENT}', STUB_PLAIN: plain-literal-value}}
      exnc:
        type: exec
        exec: {command: %s}
      exfail:
        type: exec
        exec: {command: %s}
      exfailenv:
        type: exec
        exec: {command: %s}
      exinfo:
        type: exec
        exec: {command: %s}
      exslow:
        type: exec
        exec: {command: %s, timeout: 300ms}
`, q(def), q(f.srv.URL), argv("echoargs", "token=tok-cli-1"), argv("echoargs", "token=tok-cli-2"),
		argv("echoargs", "errline=boom", "exit=3"), argv("echoargs", "exit=4"), argv("info="+filepath.Join(dir, "info.txt"), "token=tok-info-1"), argv("echoargs", "sleep=10s"))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SENT="+sentEnv+"\n"), 0o600))
	return f, dir, stub, count
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func TestAuthExecCLI(t *testing.T) {
	f, dir, _, count := setupExec(t)
	t.Run("request injects the token", func(t *testing.T) {
		code, _, errOut := runIO(t, "pet", "list-users")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-cli-1", f.last(t).Header.Get("Authorization"))
		assert.Eq(t, 1, countLines(t, count))
		// cached (ttl): the next call does not run the command.
		code, _, _ = runIO(t, "pet", "list-users")
		assert.Eq(t, 0, code)
		assert.Eq(t, 1, countLines(t, count))
	})
	t.Run("login status logout with ttl", func(t *testing.T) {
		code, out, _ := runIO(t, "auth", "status", "pet")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "logged in, expires")
		code, out, _ = runIO(t, "auth", "login", "pet")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "logged in")
		assert.Eq(t, 2, countLines(t, count))
		code, out, _ = runIO(t, "auth", "logout", "pet")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "logged out")
		code, out, _ = runIO(t, "auth", "status", "pet")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, ": logged out")
	})
	t.Run("401 re-runs the command once", func(t *testing.T) {
		before := countLines(t, count)
		code, _, _ := runIO(t, "pet", "status", "--code", "401")
		assert.Eq(t, 4, code)
		assert.Eq(t, before+2, countLines(t, count))
	})
	t.Run("uncacheable exec", func(t *testing.T) {
		code, out, _ := runIO(t, "auth", "status", "pet", "--auth", "exnc")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "type=exec: not cached (no expiry)")
		before := countLines(t, count)
		code, out, _ = runIO(t, "auth", "login", "pet", "--auth", "exnc")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "token obtained; not cached, the command runs on every request")
		assert.NotContains(t, out, "tok-cli-2")
		assert.Eq(t, before+1, countLines(t, count))
		code, out, _ = runIO(t, "auth", "logout", "pet", "--auth", "exnc")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "logged out")
		// every request runs the command.
		before = countLines(t, count)
		for i := 0; i < 2; i++ {
			code, _, _ = runIO(t, "pet", "list-users", "--auth", "exnc")
			assert.Eq(t, 0, code)
		}
		assert.Eq(t, before+2, countLines(t, count))
		assert.Eq(t, "Bearer tok-cli-2", f.last(t).Header.Get("Authorization"))
	})
	t.Run("command runs in the config directory", func(t *testing.T) {
		code, _, errOut := runIO(t, "pet", "list-users", "--auth", "exinfo")
		assert.Eq(t, 0, code, errOut)
		b, err := os.ReadFile(filepath.Join(dir, "info.txt"))
		assert.NoError(t, err)
		want, _ := filepath.EvalSymlinks(dir)
		var cwd string
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "cwd="); ok {
				cwd, _ = filepath.EvalSymlinks(v)
			}
		}
		assert.Eq(t, want, cwd)
		assert.Contains(t, string(b), "site=pet\nenv=test\nauth=exinfo\n")
	})
	t.Run("dry run never executes", func(t *testing.T) {
		before := countLines(t, count)
		code, out, _ := runIO(t, "pet", "list-users", "--auth", "exnc", "--dry-run")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "Authorization: ***")
		assert.Eq(t, before, countLines(t, count))
	})
}

func TestShowMasksExec(t *testing.T) {
	_, _, stub, _ := setupExec(t)
	for _, args := range [][]string{{"site", "show", "pet"}, {"config", "show"}} {
		code, out, errOut := runIO(t, args...)
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, stub)
		assert.Contains(t, out, `"***"`+"\n") // command: [stub, "***"]
		for _, banned := range []string{sentLit, sentEnv, "count=", "echoargs", "token=tok-cli", "plain-literal-value"} {
			assert.NotContains(t, out, banned, strings.Join(args, " "))
		}
		assert.Contains(t, out, "${env:SENT}") // references stay verbatim
	}
}

func TestShowMasksExecEnvCommand(t *testing.T) {
	_, dir, _, _ := setupExec(t)
	cfg := `version: 1
sites:
  pet:
    source: {type: simple, file: x}
    default_env: test
    envs: {test: {base_url: "http://x.test"}}
    auths:
      ex:
        type: exec
        exec: {command: ["${env:SENT}", "arg-secret-literal"]}
`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	code, out, errOut := runIO(t, "config", "show")
	assert.Eq(t, 0, code, errOut)
	assert.NotContains(t, out, sentEnv)
	assert.NotContains(t, out, "arg-secret-literal")
	assert.Contains(t, out, `- "***"`)
	code, out, _ = runIO(t, "config", "check")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "runs ***")
	assert.NotContains(t, out, sentEnv)
}

// TestExecSentinelsNeverPrinted runs every output surface and checks that
// neither the literal nor the ${env:} argv sentinel appears, and that
// command[0] appears only where it is meant to (config show, site show,
// config check).
func TestExecSentinelsNeverPrinted(t *testing.T) {
	_, _, stub, _ := setupExec(t)
	type run struct {
		name string
		args []string
		want int
	}
	runs := []run{
		{"request -v", []string{"pet", "list-users", "-v"}, 0},
		{"request -v uncached", []string{"pet", "list-users", "-v", "--auth", "exnc"}, 0},
		{"dry-run", []string{"pet", "list-users", "--dry-run", "--auth", "exnc"}, 0},
		{"dry-run -v", []string{"pet", "list-users", "--dry-run", "-v", "--auth", "exnc"}, 0},
		{"nonzero exit", []string{"pet", "list-users", "--auth", "exfail"}, 1},
		{"nonzero exit -v", []string{"pet", "list-users", "-v", "--auth", "exfail"}, 1},
		{"auth login failing", []string{"auth", "login", "pet", "--auth", "exfail", "-v"}, 1},
		{"timeout", []string{"pet", "list-users", "--auth", "exslow"}, 1},
		{"auth login", []string{"auth", "login", "pet", "-v"}, 0},
		{"auth login uncached", []string{"auth", "login", "pet", "--auth", "exnc"}, 0},
		{"auth status", []string{"auth", "status", "pet"}, 0},
		{"auth status uncached", []string{"auth", "status", "pet", "--auth", "exnc"}, 0},
		{"auth logout", []string{"auth", "logout", "pet"}, 0},
		{"auth ls", []string{"auth", "ls", "pet"}, 0},
		{"config show", []string{"config", "show"}, 0},
		{"site show", []string{"site", "show", "pet"}, 0},
		{"config check", []string{"config", "check"}, 0},
	}
	for _, r := range runs {
		t.Run(r.name, func(t *testing.T) {
			code, out, errOut := runIO(t, r.args...)
			assert.Eq(t, r.want, code, errOut)
			for _, s := range []string{out, errOut} {
				assert.NotContains(t, s, sentLit)
				assert.NotContains(t, s, sentEnv)
				if r.name != "config show" && r.name != "site show" && r.name != "config check" {
					assert.NotContains(t, s, cmd0Dir)
					assert.NotContains(t, s, stub)
				}
			}
		})
	}
	t.Run("failure messages", func(t *testing.T) {
		code, _, errOut := runIO(t, "pet", "list-users", "--auth", "exfail")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "auth exfail: exec failed (exit 3): boom")
		code, _, errOut = runIO(t, "pet", "list-users", "--auth", "exslow")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "exec timed out after 300ms")
	})
	t.Run("last stderr line is the argv echo: env value is redacted", func(t *testing.T) {
		code, out, errOut := runIO(t, "pet", "list-users", "--auth", "exfailenv")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "exec failed (exit 4): ")
		assert.Contains(t, errOut, "***")
		assert.NotContains(t, out+errOut, sentEnv)
		assert.NotContains(t, out+errOut, cmd0Dir)
	})
}

// topKeys lists the top-level YAML keys of a document, sorted.
func topKeys(doc string) []string {
	var keys []string
	for _, l := range strings.Split(doc, "\n") {
		if l != "" && l[0] != ' ' && l[0] != '-' && l[0] != '#' {
			k, _, _ := strings.Cut(l, ":")
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func TestNoAuthInjection(t *testing.T) {
	t.Run("source.auth pointing at exec is refused and never runs", func(t *testing.T) {
		f, dir, stub, count := setupExec(t)
		cfg := fmt.Sprintf(`version: 1
sites:
  rem:
    source: {type: openapi, url: %q, auth: ex}
    default_env: test
    envs: {test: {base_url: %q}}
    auths:
      ex:
        type: exec
        exec: {command: [%q, "count=%s", "token=tok-src-1"]}
`, f.srv.URL+"/openapi.json", f.srv.URL, stub, count)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
		code, _, errOut := runIO(t, "site", "refresh", "rem")
		assert.Eq(t, 1, code, errOut)
		assert.Contains(t, errOut, "apply definition credentials failed")
		assert.Eq(t, 0, countLines(t, count))
		assert.Eq(t, 0, len(f.requests()))
	})
	t.Run("site add writes only source and envs", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("APIC_CONFIG_DIR", dir)
		spec, err := filepath.Abs(filepath.Join("..", "spec", "testdata", "min-v2.json"))
		assert.NoError(t, err)
		code, _, errOut := runIO(t, "site", "add", "demo", "--spec", spec)
		assert.Eq(t, 0, code, errOut)
		data, err := os.ReadFile(filepath.Join(dir, "sites", "demo.yaml"))
		assert.NoError(t, err)
		assert.Eq(t, []string{"default_env", "envs", "source"}, topKeys(string(data)))
	})
	t.Run("auths in a simple definition produce no authentication", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("APIC_CONFIG_DIR", dir)
		stub := buildExecStub(t)
		count := filepath.Join(t.TempDir(), "count")
		def := filepath.Join(dir, "def.yaml")
		body := fmt.Sprintf("operations:\n  ping:\n    get: /ping\nauths:\n  ex:\n    type: exec\n    exec: {command: [%q, \"count=%s\", \"token=tok-def-1\"]}\n", stub, count)
		assert.NoError(t, os.WriteFile(def, []byte(body), 0o600))
		code, _, errOut := runIO(t, "site", "add", "demo", "--def", def, "--base-url", "http://x.test")
		if code != 0 {
			t.Log("simple definition with auths: rejected outright")
			assert.Eq(t, 1, code, errOut)
			assert.Eq(t, 0, countLines(t, count))
			return
		}
		data, err := os.ReadFile(filepath.Join(dir, "sites", "demo.yaml"))
		assert.NoError(t, err)
		assert.Eq(t, []string{"default_env", "envs", "source"}, topKeys(string(data)))
		code, out, _ := runIO(t, "auth", "ls", "demo")
		assert.Eq(t, 0, code)
		assert.NotContains(t, out, "exec")
		assert.NotContains(t, out, "ex ")
		code, _, errOut = runIO(t, "demo", "ping", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 0, countLines(t, count))
	})
}

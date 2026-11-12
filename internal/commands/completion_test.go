package commands_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/link"
)

const completionSpec = `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{
"/users":{"get":{"operationId":"list-users","summary":"List users","parameters":[
 {"name":"status","in":"query","schema":{"type":"string","enum":["active","banned"]}},
 {"name":"page","in":"query","schema":{"type":"integer"}}]}},
"/users/{id}":{"get":{"operationId":"get-user","summary":"Get one","parameters":[
 {"name":"id","in":"path","required":true,"schema":{"type":"string"}}]}}}}`

const completionJkDef = `operations:
  job-info:
    get: /job/{name}/api/json
  job-build:
    post: /job/{name}/buildWithParameters
    query: [BRANCH, ENV]
`

// completionEnv is a config dir with an openapi site "pet" (cached), simple
// sites "jk" and "jkx", and an exec-auth stub that counts its runs.
type completionEnv struct {
	dir, count string
	hits       *atomic.Int32
}

func setupCompletion(t *testing.T, extraSites string) *completionEnv {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/openapi.json" {
			fmt.Fprint(w, completionSpec)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	stub := buildExecStub(t)
	count := filepath.Join(t.TempDir(), "count")
	def := filepath.Join(dir, "jk-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(completionJkDef), 0o600))
	cfg := fmt.Sprintf(`version: 1
sites:
  pet:
    source: {type: openapi, url: %q}
    default_env: test
    envs:
      test: {base_url: %q}
      prod: {base_url: %q, protect: true}
      alt: {base_url: %q}
    auths:
      tok: {type: bearer, token: literal}
      ex:
        type: exec
        exec: {command: [%q, %q, "token=tok-1"]}
  jk:
    source: {type: simple, file: %q}
    default_env: test
    envs: {test: {base_url: %q}}
  jkx:
    source: {type: simple, file: %q}
    default_env: test
    envs: {test: {base_url: %q}}
%s`, srv.URL+"/openapi.json", srv.URL, srv.URL, srv.URL, stub, "count="+count, def, srv.URL, def, srv.URL, extraSites)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	code, _, errOut := runIO(t, "site", "refresh", "pet")
	assert.Eq(t, 0, code, errOut)
	hits.Store(0)
	return &completionEnv{dir: dir, count: count, hits: &hits}
}

// complete runs `apic --in-completion <words>` and returns the sorted candidates.
func complete(t *testing.T, words ...string) []string {
	t.Helper()
	code, out, errOut := runIO(t, append([]string{"--in-completion"}, words...)...)
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "", errOut)
	assert.NotContains(t, out, "ERROR")
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	sort.Strings(lines)
	return lines
}

var managementNames = []string{"auth", "config", "desc", "help", "link", "ops", "overlay", "site", "skill", "use"}

func withNames(extra ...string) []string {
	out := append(append([]string{}, managementNames...), extra...)
	sort.Strings(out)
	return out
}

func TestCompletion(t *testing.T) {
	env := setupCompletion(t, "")
	globalLong := []string{"--all-pages", "--auth", "--dry-run", "--env", "--header", "--include", "--max-pages", "--output", "--pick", "--timeout", "--verbose", "--yes"}
	t.Run("top level lists management commands and sites", func(t *testing.T) {
		assert.Eq(t, withNames("jk", "jkx", "pet"), complete(t, ""))
	})
	t.Run("site level lists operations", func(t *testing.T) {
		assert.Eq(t, []string{"job-build", "job-info"}, complete(t, "jk", ""))
		assert.Eq(t, []string{"get-user", "list-users"}, complete(t, "pet", ""))
	})
	t.Run("operation options include parameters and global options", func(t *testing.T) {
		got := complete(t, "jk", "job-build", "--")
		for _, want := range []string{"--BRANCH", "--ENV", "--env", "--dry-run", "--pick", "--output", "--auth"} {
			assert.Contains(t, got, want)
		}
		got = complete(t, "pet", "list-users", "--")
		for _, want := range append([]string{"--status", "--page"}, globalLong...) {
			assert.Contains(t, got, want)
		}
		assert.Contains(t, complete(t, "pet", "list-users", "-"), "-o")
	})
	t.Run("option values", func(t *testing.T) {
		assert.Eq(t, []string{"alt", "prod", "test"}, complete(t, "pet", "list-users", "--env", ""))
		assert.Eq(t, []string{"ex", "tok"}, complete(t, "pet", "list-users", "--auth", ""))
		assert.Eq(t, []string{"alt"}, complete(t, "pet", "list-users", "--env", "a"))
		assert.Eq(t, []string{"active", "banned"}, complete(t, "pet", "list-users", "--status", ""))
		assert.Eq(t, []string{"auto", "json", "raw", "table", "yaml"}, complete(t, "pet", "list-users", "-o", ""))
		assert.Eq(t, []string{"auto", "json", "raw", "table", "yaml"}, complete(t, "pet", "-o", ""))
		assert.Eq(t, []string{"alt", "prod", "test"}, complete(t, "pet", "list-users", "--env", ""))
	})
	t.Run("equals form is split by bash", func(t *testing.T) {
		assert.Eq(t, []string{"alt"}, complete(t, "pet", "list-users", "--env", "=", "a"))
		assert.Eq(t, []string{"alt", "prod", "test"}, complete(t, "pet", "list-users", "--env", "="))
	})
	t.Run("static commands only offer json", func(t *testing.T) {
		assert.Eq(t, []string{"json"}, complete(t, "ops", "pet", "-o", ""))
		assert.Eq(t, []string{"json"}, complete(t, "desc", "pet", "list-users", "-o", ""))
	})
	t.Run("no request, login or exec", func(t *testing.T) {
		complete(t, "pet", "list-users", "--")
		complete(t, "pet", "")
		complete(t, "")
		assert.Eq(t, int32(0), env.hits.Load())
		_, err := os.Stat(env.count)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestPrescanCompletion(t *testing.T) {
	setupCompletion(t, "")
	top := []string{"--all-pages", "--api", "--auth", "--dry-run", "--env", "--gen-completion", "--header", "--help", "--include", "--max-pages", "--output", "--pick", "--timeout", "--verbose", "--version", "--yes", "-H", "-V", "-h", "-i", "-o", "-v"}
	t.Run("top level options", func(t *testing.T) {
		assert.Eq(t, top, complete(t, "-"))
		assert.Eq(t, []string{"--env"}, complete(t, "--e"))
		assert.Eq(t, []string{"--env"}, complete(t, "--env"))
		assert.Eq(t, []string{"--dry-run"}, complete(t, "--dr"))
	})
	t.Run("value options at top level", func(t *testing.T) {
		assert.Eq(t, []string{"jk", "jkx", "pet"}, complete(t, "--api", ""))
		assert.Eq(t, []string{"pet"}, complete(t, "--api", "pe"))
		assert.Eq(t, []string{"pet"}, complete(t, "--api", "=", "pe"))
		assert.Eq(t, []string{"jk", "jkx", "pet"}, complete(t, "--api", "="))
		assert.Eq(t, []string{"auto", "json", "raw", "table", "yaml"}, complete(t, "-o", ""))
		assert.Eq(t, []string{"json"}, complete(t, "--output", "j"))
		assert.Eq(t, []string{"yaml"}, complete(t, "--output", "=", "y"))
		assert.Eq(t, []string(nil), complete(t, "--env", ""))
		assert.Eq(t, []string(nil), complete(t, "--auth", ""))
		assert.Eq(t, []string(nil), complete(t, "--timeout", ""))
	})
	t.Run("the word being typed is not a site hit", func(t *testing.T) {
		assert.Eq(t, []string{"jk", "jkx"}, complete(t, "jk"))
		assert.Eq(t, []string{"jkx"}, complete(t, "jkx"))
		assert.Eq(t, []string{"job-build", "job-info"}, complete(t, "--env", "test", "jk", ""))
	})
	t.Run("api rewrites to the site", func(t *testing.T) {
		assert.Eq(t, []string{"get-user", "list-users"}, complete(t, "--api", "pet", ""))
		assert.Eq(t, []string{"alt", "prod", "test"}, complete(t, "--api=pet", "list-users", "--env", ""))
		assert.Contains(t, complete(t, "--api", "pet", "--"), "--env")
	})
	t.Run("errors are silent", func(t *testing.T) {
		for _, words := range [][]string{
			{"--bogus", "jk", ""}, {"--api", "nope", ""}, {"--api", "pet", "--api", "jk", ""}, {"-h", ""}, {"--version", ""},
		} {
			assert.Eq(t, []string(nil), complete(t, words...), words)
		}
	})
	t.Run("management commands still complete", func(t *testing.T) {
		assert.Contains(t, complete(t, "site", ""), "add")
	})
}

func TestCompletionOffline(t *testing.T) {
	t.Run("expired cache is used and nothing is fetched", func(t *testing.T) {
		env := setupCompletion(t, "")
		meta := filepath.Join(env.dir, "cache", "specs", "pet.meta.json")
		old := time.Now().Add(-1000 * time.Hour).UTC().Format(time.RFC3339)
		assert.NoError(t, os.WriteFile(meta, []byte(`{"etag":"x","fetched_at":"`+old+`"}`), 0o600))
		assert.Eq(t, []string{"get-user", "list-users"}, complete(t, "pet", ""))
		assert.Eq(t, []string{"active", "banned"}, complete(t, "pet", "list-users", "--status", ""))
		assert.Eq(t, int32(0), env.hits.Load())
		_, err := os.Stat(env.count)
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("missing cache registers no operations and is silent", func(t *testing.T) {
		env := setupCompletion(t, "")
		assert.NoError(t, os.RemoveAll(filepath.Join(env.dir, "cache")))
		assert.Eq(t, []string(nil), complete(t, "pet", ""))
		assert.Eq(t, int32(0), env.hits.Load())
	})
	t.Run("a site named skill keeps stderr empty", func(t *testing.T) {
		setupCompletion(t, "  skill:\n    source: {type: simple, file: jk-def.yaml}\n    default_env: test\n    envs: {test: {base_url: \"http://127.0.0.1:1\"}}\n")
		got := complete(t, "")
		assert.Contains(t, got, "jk")
		complete(t, "pet", "")
	})
	t.Run("a broken config is silent", func(t *testing.T) {
		env := setupCompletion(t, "")
		assert.NoError(t, os.WriteFile(filepath.Join(env.dir, "config.yaml"), []byte("version: [\n"), 0o600))
		assert.Eq(t, withNames(), complete(t, ""))
	})
}

func TestLinkCompletion(t *testing.T) {
	recs := link.Records{
		"petctl":          {Site: "pet", Path: "/x/petctl"},
		"petctl.cmd":      {Site: "pet", Path: "/x/petctl.cmd"},
		"my-jk":           {Site: "jk", Path: "/x/my-jk"},
		"bad;rm -rf /":    {Site: "pet"},
		"evil":            {Site: "pet;rm -rf /"},
		"sub":             {Site: "$(x)"},
		"$(x)":            {Site: "pet"},
		"has space":       {Site: "pet"},
		"quote'd":         {Site: "pet"},
		"sp":              {Site: "pe t"},
		"dq":              {Site: `pe"t`},
		"UPPER":           {Site: "pet"},
		"ok.cmd":          {Site: "pet"},
		"-lead":           {Site: "pet"},
		"":                {Site: "pet"},
		"tick":            {Site: "`id`"},
		"new\nline":       {Site: "pet"},
		"under_score":     {Site: "pet"},
		"also-ok":         {Site: ""},
		"dots.name.cmd.x": {Site: "pet"},
	}
	for _, shell := range []string{"bash", "zsh", "pwsh"} {
		t.Run(shell, func(t *testing.T) {
			script, err := link.CompletionScript(shell, recs)
			assert.NoError(t, err)
			for _, want := range []string{"petctl", "my-jk", "ok"} {
				assert.Contains(t, script, want)
			}
			assert.Contains(t, script, "--in-completion")
			assert.NotContains(t, script, "--api")
			assert.NotContains(t, script, "--env")
			for _, bad := range []string{"rm -rf", "$(x)", "has space", "quote'd", "pe t", `pe"t`, "UPPER", "`id`", "under_score", "evil", "also-ok", "dots", "tick"} {
				assert.NotContains(t, script, bad)
			}
		})
	}
	t.Run("one function per valid shortcut", func(t *testing.T) {
		script, err := link.CompletionScript("bash", recs)
		assert.NoError(t, err)
		assert.Eq(t, 3, strings.Count(script, "complete -F "))
		assert.Contains(t, script, "complete -F _apic_link_my_jk my-jk")
		assert.Contains(t, script, `apic --in-completion jk "${words[@]}"`)
		zsh, _ := link.CompletionScript("zsh", recs)
		assert.Contains(t, zsh, `apic --in-completion pet "${words[@]:1}"`)
		assert.Contains(t, zsh, "compdef _apic_link_petctl petctl")
		pwsh, _ := link.CompletionScript("pwsh", recs)
		assert.Contains(t, pwsh, "Register-ArgumentCompleter -Native -CommandName 'petctl'")
		assert.Contains(t, pwsh, "--in-completion 'pet' @words")
	})
	t.Run("unsupported shell", func(t *testing.T) {
		_, err := link.CompletionScript("fish", recs)
		assert.Err(t, err)
	})
	t.Run("link ls --completion", func(t *testing.T) {
		setupCompletion(t, "")
		code, out, errOut := runIO(t, "link", "pet", "--name", "petctl", "--dir", t.TempDir())
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, errOut, "link ls --completion bash")
		assert.Contains(t, out, "petctl")
		code, out, errOut = runIO(t, "link", "ls", "--completion", "bash")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "", errOut)
		assert.Contains(t, out, "complete -F _apic_link_petctl petctl")
		code, _, errOut = runIO(t, "link", "ls", "--completion", "fish")
		assert.Eq(t, 2, code)
		assert.Contains(t, errOut, "unsupported shell")
		code, _, errOut = runIO(t, "link", "rm", "petctl")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, errOut, "refresh shortcut completion")
	})
	// The rewritten call is `apic --in-completion <site> <words>`.
	t.Run("rewritten words complete like the site", func(t *testing.T) {
		setupCompletion(t, "")
		assert.Eq(t, []string{"get-user", "list-users"}, complete(t, "pet", ""))
		got := complete(t, "pet", "--")
		assert.Contains(t, got, "--env")
		assert.Eq(t, []string{"alt", "prod", "test"}, complete(t, "pet", "--env", ""))
		assert.Contains(t, complete(t, "pet", "list-users", "--"), "--status")
	})
}

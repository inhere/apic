package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const ovlDef = `operations:
  list-users:
    get: /users
    summary: List users
    query: [page, debug]
  get-user:
    get: '/users/{id}'
  create-user:
    post: /users
    body: {type: json}
  secret:
    get: /secret
    summary: Secret op
  auth-op:
    get: /auth
    params: [{name: Authorization, in: header}]
`

// ovlEnv is setup() with the overlay test definition on site "pet".
func ovlEnv(t *testing.T) (*fakeAPI, string) {
	t.Helper()
	f, dir := setup(t)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "pet-def.yaml"), []byte(ovlDef), 0600))
	return f, dir
}

func ovlWrite(t *testing.T, dir, content string) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "overlays"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "overlays", "pet.yaml"), []byte(content), 0600))
}

// ovlOpNames returns the NAME column of `ops pet`.
func ovlOpNames(t *testing.T) []string {
	t.Helper()
	code, out, errOut := runIO(t, "ops", "pet")
	assert.Eq(t, 0, code, errOut)
	var names []string
	for _, l := range strings.Split(out, "\n")[1:] {
		if f := strings.Fields(l); len(f) > 0 {
			names = append(names, f[0])
		}
	}
	return names
}

func TestOverlayRename(t *testing.T) {
	t.Run("swap names", func(t *testing.T) {
		f, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"GET /users\": {name: get-user}\n  \"GET /users/{id}\": {name: list-users}\n")
		code, out, errOut := runIO(t, "desc", "pet", "get-user")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "GET /test/users\n")
		assert.Contains(t, out, "OVERLAY: name")
		code, _, errOut = runIO(t, "pet", "list-users", "-p", "id=7")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "/test/users/7", f.last(t).Path)
		assert.Eq(t, "", errOut)
	})
	t.Run("generated name clash gets suffix and warning", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  secret: {name: get-user}\n")
		code, out, errOut := runIO(t, "ops", "pet")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, errOut, `warning: operation GET /users/{id} renamed "get-user" to "get-user-2"`)
		assert.Contains(t, out, "get-user-2")
		ovlMatch(t, `get-user\s+GET\s+/secret`, out)
	})
	t.Run("duplicate overlay names are an error", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  secret: {name: same}\n  create-user: {name: same}\n")
		code, _, errOut := runIO(t, "ops", "pet")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, `name "same" is also set by`)
	})
	t.Run("help is rejected", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  secret: {name: help}\n")
		code, _, errOut := runIO(t, "ops", "pet")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "name must not be help")
	})
}

func TestOverlayHideOperation(t *testing.T) {
	f, dir := ovlEnv(t)
	code, _, errOut := runIO(t, "site", "refresh", "pet")
	assert.Eq(t, 0, code, errOut)
	ovlWrite(t, dir, "operations:\n  \"GET /secret\": {hide: true}\n")
	t.Run("ops", func(t *testing.T) {
		assert.Eq(t, []string{"auth-op", "create-user", "get-user", "list-users"}, ovlOpNames(t))
	})
	t.Run("completion", func(t *testing.T) {
		got := complete(t, "pet", "")
		assert.Contains(t, got, "list-users")
		assert.NotContains(t, got, "secret")
	})
	t.Run("skill", func(t *testing.T) {
		out := t.TempDir()
		code, _, errOut := runIO(t, "skill", "pet", "--out", out)
		assert.Eq(t, 0, code, errOut)
		data, err := os.ReadFile(filepath.Join(out, "apic-pet", "SKILL.md"))
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), "list-users")
		assert.NotContains(t, string(data), "secret")
		assert.NotContains(t, string(data), "/secret")
	})
	t.Run("unknown by name", func(t *testing.T) {
		code, _, errOut := runIO(t, "pet", "secret")
		assert.Eq(t, 2, code)
		assert.Contains(t, errOut, "unknown operation")
		code, _, _ = runIO(t, "desc", "pet", "secret")
		assert.Eq(t, 2, code)
	})
	t.Run("raw path still sent", func(t *testing.T) {
		code, _, errOut := runIO(t, "pet", "/secret")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "/secret", f.last(t).Path) // raw paths carry no base path
	})
}

// ovlMatch asserts that out matches the regular expression pattern.
func ovlMatch(t *testing.T, pattern, out string) {
	t.Helper()
	assert.True(t, regexp.MustCompile(pattern).MatchString(out), "pattern %q not found in:\n%s", pattern, out)
}

func TestOverlayParams(t *testing.T) {
	t.Run("edit existing parameter", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, `operations:
  "GET /users":
    params:
      page: {required: true, type: integer, desc: Page number, enum: ["1", "2"]}
`)
		code, out, errOut := runIO(t, "desc", "pet", "list-users")
		assert.Eq(t, 0, code, errOut)
		ovlMatch(t, `page \(overlay\)\s+query\s+integer\s+true\s+1,2\s+Page number`, out)
		assert.Contains(t, out, "debug  ")
		assert.NotContains(t, out, "debug (overlay)")
	})
	t.Run("added query and header parameters are sent", func(t *testing.T) {
		f, dir := ovlEnv(t)
		ovlWrite(t, dir, `operations:
  "GET /users":
    params:
      limit: {in: query, type: integer, desc: Max rows}
      X-Trace: {in: header}
`)
		code, out, errOut := runIO(t, "desc", "pet", "list-users")
		assert.Eq(t, 0, code, errOut)
		ovlMatch(t, `limit \(overlay\)\s+query\s+integer`, out)
		ovlMatch(t, `X-Trace \(overlay\)\s+header\s+string`, out)
		code, _, errOut = runIO(t, "pet", "list-users", "--limit", "5", "--X-Trace", "abc")
		assert.Eq(t, 0, code, errOut)
		last := f.last(t)
		assert.Eq(t, "limit=5", last.Query)
		assert.Eq(t, "abc", last.Header.Get("X-Trace"))
	})
	t.Run("hidden parameter", func(t *testing.T) {
		f, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"GET /users\": {params: {debug: {hide: true}}}\n")
		code, out, errOut := runIO(t, "desc", "pet", "list-users")
		assert.Eq(t, 0, code, errOut)
		ovlMatch(t, `debug \(hidden\)\s+query`, out)
		assert.NotContains(t, out, "debug (overlay)")
		// No --debug option is generated, so it is not offered either.
		code, out, _ = runIO(t, "pet", "list-users", "-h")
		assert.Eq(t, 0, code)
		assert.NotContains(t, out, "--debug")
		assert.Contains(t, out, "--page")
		for _, arg := range []string{"debug=1", "query.debug=1"} {
			n := len(f.requests())
			code, _, errOut = runIO(t, "pet", "list-users", "-p", arg)
			assert.Eq(t, 2, code, arg)
			assert.Contains(t, errOut, "hidden by overlay")
			assert.Eq(t, n, len(f.requests()))
		}
		code, _, errOut = runIO(t, "pet", "list-users", "--page", "2")
		assert.Eq(t, 0, code, errOut)
	})
	t.Run("body type is annotated", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"POST /users\": {body: form, output: raw, summary: Make}\n")
		code, out, errOut := runIO(t, "desc", "pet", "create-user")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "BODY form (overlay)")
		assert.Contains(t, out, "OVERLAY: summary, output\n")
	})
	t.Run("spec Authorization parameter disambiguated by in", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  auth-op: {params: {Authorization: {in: header, desc: Token}}}\n")
		code, out, errOut := runIO(t, "desc", "pet", "auth-op")
		assert.Eq(t, 0, code, errOut)
		ovlMatch(t, `Authorization \(overlay\)\s+header`, out)
	})
}

func TestOverlayStructuralErrors(t *testing.T) {
	cases := []struct{ name, overlay, want string }{
		{"hidden path parameter", "operations:\n  \"GET /users/{id}\": {params: {id: {hide: true}}}\n", "path parameter cannot be hidden"},
		{"hidden required parameter", "operations:\n  \"GET /users\": {params: {page: {required: true, hide: true}}}\n", "hidden parameter is still required"},
		{"new path parameter", "operations:\n  \"GET /users\": {params: {extra: {in: path}}}\n", "new parameter needs in: query or header"},
		{"blacklisted header", "operations:\n  \"GET /users\": {params: {Content-Type: {in: header}}}\n", "cannot be added"},
		{"in mismatch", "operations:\n  \"GET /users\": {params: {page: {in: header}}}\n", "does not match the parameter's location"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, dir := ovlEnv(t)
			ovlWrite(t, dir, "")
			assert.NoError(t, os.Remove(filepath.Join(dir, "overlays", "pet.yaml")))
			code, _, errOut := runIO(t, "site", "refresh", "pet")
			assert.Eq(t, 0, code, errOut)
			ovlWrite(t, dir, tc.overlay)
			n := len(f.requests())
			for _, args := range [][]string{
				{"pet", "list-users"}, {"pet", "/users"}, {"ops", "pet"}, {"desc", "pet", "list-users"},
				{"skill", "pet", "--out", t.TempDir()},
			} {
				code, out, errOut := runIO(t, args...)
				assert.Eq(t, 1, code, strings.Join(args, " "))
				assert.Contains(t, errOut, tc.want, strings.Join(args, " "))
				assert.Eq(t, "", out)
			}
			assert.Eq(t, n, len(f.requests()))
			// Refresh reads the raw index and keeps working.
			code, _, errOut = runIO(t, "site", "refresh", "pet")
			assert.Eq(t, 0, code, errOut)
			// Completion prints nothing for the site and never the raw index.
			code, out, errOut := runIO(t, "--in-completion", "pet", "")
			assert.Eq(t, 0, code)
			assert.Eq(t, "", errOut)
			assert.NotContains(t, out, "list-users")
			assert.NotContains(t, out, "get-user")
		})
	}
	t.Run("invalid overlay file text", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  secret: {bogus: 1}\n")
		code, _, errOut := runIO(t, "ops", "pet")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "overlay ")
	})
}

func TestOverlayKeys(t *testing.T) {
	t.Run("unmatched key only warns", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"GET /nowhere\": {hide: true}\n")
		code, out, errOut := runIO(t, "ops", "pet")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, errOut, "warning: overlay ")
		assert.Contains(t, errOut, "GET /nowhere matches no operation")
		assert.Contains(t, out, "list-users")
	})
	t.Run("null entry equals empty entry", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"GET /users\":\n")
		code, out, errOut := runIO(t, "desc", "pet", "list-users")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "", errOut)
		assert.NotContains(t, out, "OVERLAY")
	})
}

func TestOverlayDescJSON(t *testing.T) {
	t.Run("overlay array with changes", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations:\n  \"GET /users\": {name: people, params: {page: {type: integer}}}\n")
		code, out, errOut := runIO(t, "desc", "pet", "people", "-o", "json")
		assert.Eq(t, 0, code, errOut)
		var got struct {
			Name    string
			Overlay []string `json:"overlay"`
		}
		assert.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Eq(t, "people", got.Name)
		assert.Eq(t, []string{"name", "params.query.page.type"}, got.Overlay)
	})
	t.Run("without changes output is unchanged", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, base, errOut := runIO(t, "desc", "pet", "list-users", "-o", "json")
		assert.Eq(t, 0, code, errOut)
		assert.NotContains(t, base, `"overlay"`)
		// An overlay that matches nothing changes nothing: same bytes.
		ovlWrite(t, dir, "operations:\n  \"GET /nowhere\": {hide: true}\n")
		code, out, _ := runIO(t, "desc", "pet", "list-users", "-o", "json")
		assert.Eq(t, 0, code)
		assert.Eq(t, base, out)
		assert.NotContains(t, out, `"Hidden"`)
	})
}

func TestOverlayInit(t *testing.T) {
	t.Run("round trips with no changes", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, out, errOut := runIO(t, "overlay", "init", "pet")
		assert.Eq(t, 0, code, errOut)
		path := filepath.Join(dir, "overlays", "pet.yaml")
		assert.Eq(t, path+"\n", out)
		data, err := os.ReadFile(path)
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), `"GET /users": {}  # name: list-users; List users`)
		assert.Equal(t, 5, strings.Count(string(data), ": {}  #"))
		// Reading it back changes nothing and warns about nothing.
		code, out, errOut = runIO(t, "ops", "pet")
		assert.Eq(t, 0, code)
		assert.Eq(t, "", errOut)
		assert.Contains(t, out, "secret")
		code, out, errOut = runIO(t, "config", "check")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "ok")
	})
	t.Run("perms", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix permission bits")
		}
		_, dir := ovlEnv(t)
		code, _, errOut := runIO(t, "overlay", "init", "pet")
		assert.Eq(t, 0, code, errOut)
		di, err := os.Stat(filepath.Join(dir, "overlays"))
		assert.NoError(t, err)
		assert.Eq(t, os.FileMode(0700), di.Mode().Perm())
		fi, err := os.Stat(filepath.Join(dir, "overlays", "pet.yaml"))
		assert.NoError(t, err)
		assert.Eq(t, os.FileMode(0600), fi.Mode().Perm())
	})
	t.Run("stdout writes no file", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, out, errOut := runIO(t, "overlay", "init", "pet", "--stdout")
		assert.Eq(t, 0, code, errOut)
		assert.True(t, strings.HasPrefix(out, "# apic overlay for site pet;"))
		assert.Contains(t, out, `"GET /secret": {}`)
		_, err := os.Stat(filepath.Join(dir, "overlays"))
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("existing file is an error", func(t *testing.T) {
		_, dir := ovlEnv(t)
		ovlWrite(t, dir, "operations: {}\n")
		code, _, errOut := runIO(t, "overlay", "init", "pet")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "already exists")
		data, err := os.ReadFile(filepath.Join(dir, "overlays", "pet.yaml"))
		assert.NoError(t, err)
		assert.Eq(t, "operations: {}\n", string(data))
	})
	t.Run("overlays symlink is an error", func(t *testing.T) {
		_, dir := ovlEnv(t)
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(dir, "overlays")); err != nil {
			t.Skip("symlinks unavailable: " + err.Error())
		}
		code, _, errOut := runIO(t, "overlay", "init", "pet")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "not a directory")
		entries, err := os.ReadDir(target)
		assert.NoError(t, err)
		assert.Eq(t, 0, len(entries))
	})
	t.Run("unknown site", func(t *testing.T) {
		ovlEnv(t)
		code, _, _ := runIO(t, "overlay", "init", "nope")
		assert.Eq(t, 1, code)
	})
	t.Run("hostile summary cannot inject entries", func(t *testing.T) {
		_, dir := ovlEnv(t)
		def := "operations:\n  inj:\n    get: /inj\n    summary: \"first\\nx: {hide: true}\\u2028y: {hide: true}\"\n"
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "pet-def.yaml"), []byte(def), 0600))
		code, out, errOut := runIO(t, "overlay", "init", "pet", "--stdout")
		assert.Eq(t, 0, code, errOut)
		assert.Equal(t, 1, strings.Count(out, ": {}"))
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			assert.False(t, strings.HasPrefix(strings.TrimSpace(l), "x:"), l)
		}
		code, _, errOut = runIO(t, "overlay", "init", "pet")
		assert.Eq(t, 0, code, errOut)
		code, out, errOut = runIO(t, "ops", "pet")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "inj")
		assert.Eq(t, "", errOut)
	})
	t.Run("duplicate route is keyed by name", func(t *testing.T) {
		_, dir := ovlEnv(t)
		def := "operations:\n  one:\n    get: /same\n  two:\n    get: /same\n"
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "pet-def.yaml"), []byte(def), 0600))
		code, out, errOut := runIO(t, "overlay", "init", "pet", "--stdout")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, `"one": {}`)
		assert.Contains(t, out, `"two": {}`)
		assert.NotContains(t, out, `"GET /same"`)
	})
}

func TestOverlayConfigCheck(t *testing.T) {
	t.Run("clean overlay", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, _, errOut := runIO(t, "site", "refresh", "pet")
		assert.Eq(t, 0, code, errOut)
		ovlWrite(t, dir, "operations:\n  secret: {hide: true}\n")
		code, out, _ := runIO(t, "config", "check")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "ok")
	})
	t.Run("unmatched key is reported without failing", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, _, errOut := runIO(t, "site", "refresh", "pet")
		assert.Eq(t, 0, code, errOut)
		ovlWrite(t, dir, "operations:\n  \"GET /nowhere\": {hide: true}\n")
		code, out, _ := runIO(t, "config", "check")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "GET /nowhere matches no operation")
	})
	t.Run("structural error exits 1", func(t *testing.T) {
		_, dir := ovlEnv(t)
		code, _, errOut := runIO(t, "site", "refresh", "pet")
		assert.Eq(t, 0, code, errOut)
		ovlWrite(t, dir, "operations:\n  \"GET /users/{id}\": {params: {id: {hide: true}}}\n")
		code, out, _ := runIO(t, "config", "check")
		assert.Eq(t, 1, code)
		assert.Contains(t, out, "site pet overlay: ")
		assert.Contains(t, out, "path parameter cannot be hidden")
	})
	t.Run("no cached index", func(t *testing.T) {
		_, dir := ovlEnv(t)
		// Simple sites load directly; only an OpenAPI site can lack a cache.
		f, err := os.OpenFile(filepath.Join(dir, "config.yaml"), os.O_APPEND|os.O_WRONLY, 0)
		assert.Require(t, assert.NoError(t, err))
		_, err = f.WriteString("  oas:\n    source: {type: openapi, url: 'http://127.0.0.1:1/openapi.json'}\n")
		assert.NoError(t, err)
		assert.NoError(t, f.Close())
		assert.NoError(t, os.MkdirAll(filepath.Join(dir, "overlays"), 0700))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "overlays", "oas.yaml"), []byte("operations: {}\n"), 0600))
		code, out, _ := runIO(t, "config", "check")
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "site oas overlay: no cached index; run apic site refresh oas")
	})
}

func TestOverlayReservedSiteName(t *testing.T) {
	_, dir := ovlEnv(t)
	cfg := "version: 1\nsites:\n  overlay:\n    source: {type: simple, file: " + filepath.Join(dir, "pet-def.yaml") + "}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	code, out, errOut := runIO(t, "site", "ls")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, errOut, `site "overlay" uses a reserved name and is skipped`)
	assert.NotContains(t, out, "overlay ")
}

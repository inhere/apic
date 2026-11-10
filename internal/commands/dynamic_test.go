package commands_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gookit/color"
	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/commands"
)

// recorded is one request seen by the fake API.
type recorded struct {
	Method, Path, Query, Body string
	Header                    http.Header
}

// fakeAPI is an httptest server recording requests, with a login endpoint.
type fakeAPI struct {
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []recorded
	logins int
}

const loginToken = "login-secret-token"

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		if r.URL.Path == "/api/login" {
			f.logins++
		}
		f.reqs = append(f.reqs, recorded{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/login":
			fmt.Fprintf(w, `{"data":{"token":%q,"expires_in":3600}}`, loginToken)
		case strings.HasSuffix(r.URL.Path, "/slow"):
			time.Sleep(500 * time.Millisecond)
		case strings.HasSuffix(r.URL.Path, "/cookie"):
			w.Header().Set("Set-Cookie", "sid=cookie-secret-value")
			fmt.Fprint(w, `{"ok":true}`)
		case strings.Contains(r.URL.Path, "/status/"):
			var code int
			fmt.Sscanf(r.URL.Path[strings.Index(r.URL.Path, "/status/")+len("/status/"):], "%d", &code)
			w.WriteHeader(code)
			fmt.Fprintf(w, `{"code":%d}`, code)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

func (f *fakeAPI) last(t *testing.T) recorded {
	t.Helper()
	r := f.requests()
	assert.Require(t, assert.True(t, len(r) > 0))
	return r[len(r)-1]
}

func (f *fakeAPI) loginCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logins
}

const petDef = `operations:
  list-users:
    get: /users
    summary: List users
    query: [page, env]
  get-user:
    get: /users/{id}
  create-user:
    post: /users
    body: {type: json}
  delete-user:
    delete: /users/{id}
  slow:
    get: /slow
  status:
    get: /status/{code}
`

// setup writes a config directory with site "pet" (simple definition) and returns the fake API.
func setup(t *testing.T) (*fakeAPI, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	f := newFakeAPI(t)
	def := filepath.Join(dir, "pet-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(petDef), 0600))
	cfg := fmt.Sprintf(`version: 1
sites:
  pet:
    source: {type: simple, file: %q}
    default_env: test
    default_auth: tok
    envs:
      test: {base_url: %q, base_path: /test}
      alt: {base_url: %q, base_path: /alt}
      prod: {base_url: %q, protect: true}
    auths:
      tok: {type: bearer, token: '${env:APIX_T_TOKEN}'}
      ro: {type: bearer, token: '${env:APIX_T_TOKEN}', allow_methods: [GET]}
      lg:
        type: login
        login:
          method: POST
          path: /api/login
          body: {username: u, password: '${env:APIX_T_PASS}'}
        token: {from: body, path: data.token}
        expires: {from: body, path: data.expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
`, def, f.srv.URL, f.srv.URL, f.srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APIX_T_TOKEN=tok-secret-value\nAPIX_T_PASS=pass-secret-value\n"), 0600))
	return f, dir
}

// runIO runs the real dispatch capturing stdout and stderr.
func runIO(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	or, ow, err := os.Pipe()
	assert.Require(t, assert.NoError(t, err))
	er, ew, err := os.Pipe()
	assert.Require(t, assert.NoError(t, err))
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew
	// gcli prints help through the color package, which captured stdout at init.
	color.SetOutput(ow)
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr; color.ResetOutput() }()
	code = commands.Run(commands.BuildInfo{}, args)
	assert.NoError(t, ow.Close())
	assert.NoError(t, ew.Close())
	o, _ := io.ReadAll(or)
	e, _ := io.ReadAll(er)
	return code, string(o), string(e)
}

func TestPrescan(t *testing.T) {
	spec := commands.GlobalSpec{Flags: map[string]bool{"--env": true, "-o": true, "--yes": false, "-H": true}}
	sites := []string{"pet", "other"}
	cases := []struct {
		name      string
		args      []string
		site      string
		rewritten []string
		wantErr   bool
	}{
		{"positional site", []string{"pet", "list-users"}, "pet", []string{"pet", "list-users"}, false},
		{"globals before site", []string{"--env", "prod", "--yes", "pet", "ls"}, "pet", []string{"--env", "prod", "--yes", "pet", "ls"}, false},
		{"equals form value", []string{"--env=prod", "pet"}, "pet", []string{"--env=prod", "pet"}, false},
		{"value looks like option", []string{"-H", "-x", "pet"}, "pet", []string{"-H", "-x", "pet"}, false},
		{"api separate", []string{"--api", "pet", "list-users", "--x"}, "pet", []string{"pet", "list-users", "--x"}, false},
		{"api equals", []string{"--api=pet", "list-users"}, "pet", []string{"pet", "list-users"}, false},
		{"api after globals", []string{"--env", "prod", "--api", "pet", "list-users"}, "pet", []string{"--env", "prod", "pet", "list-users"}, false},
		{"api only", []string{"--api", "pet"}, "pet", []string{"pet"}, false},
		{"api unknown site", []string{"--api", "bogus", "x"}, "", nil, true},
		{"api twice", []string{"--api", "pet", "--api", "other"}, "", nil, true},
		{"api missing value", []string{"--api"}, "", nil, true},
		{"unknown global", []string{"--bogus", "pet"}, "", nil, true},
		{"unknown short", []string{"-z"}, "", nil, true},
		{"missing value", []string{"--env"}, "", nil, true},
		{"management command", []string{"site", "ls"}, "", []string{"site", "ls"}, false},
		{"unknown first", []string{"nothing"}, "", []string{"nothing"}, false},
		{"builtin version", []string{"--version"}, "", []string{"--version"}, false},
		{"builtin help", []string{"-h"}, "", []string{"-h"}, false},
		{"options after site untouched", []string{"pet", "--bogus", "-X", "POST"}, "pet", []string{"pet", "--bogus", "-X", "POST"}, false},
		{"empty", nil, "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site, rewritten, err := commands.Prescan(tc.args, spec, sites)
			if tc.wantErr {
				assert.Err(t, err)
				assert.Contains(t, err.Error(), "")
				return
			}
			assert.NoError(t, err)
			assert.Eq(t, tc.site, site)
			assert.Eq(t, tc.rewritten, rewritten)
		})
	}
}

func TestDynamicGlobalOptionPositions(t *testing.T) {
	f, _ := setup(t)
	for i, args := range [][]string{
		{"-H", "X-T: before", "pet", "list-users"},
		{"pet", "-H", "X-T: site", "list-users"},
		{"pet", "list-users", "-H", "X-T: op"},
	} {
		code, _, errOut := runIO(t, args...)
		assert.Eq(t, 0, code, errOut)
		want := strings.TrimPrefix(args[map[int]int{0: 1, 1: 2, 2: 3}[i]], "X-T: ")
		assert.Eq(t, want, f.last(t).Header.Get("X-T"))
	}
	// --env in each position selects the alt base path.
	for _, args := range [][]string{
		{"--env", "alt", "pet", "list-users"},
		{"pet", "--env", "alt", "list-users"},
		{"pet", "list-users", "--env", "alt"},
		{"pet", "list-users", "--env=alt"},
	} {
		code, _, errOut := runIO(t, args...)
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "/alt/users", f.last(t).Path)
	}
	// repeated -H accumulates across positions.
	code, _, _ := runIO(t, "-H", "X-A: 1", "pet", "list-users", "-H", "X-B: 2")
	assert.Eq(t, 0, code)
	assert.Eq(t, "1", f.last(t).Header.Get("X-A"))
	assert.Eq(t, "2", f.last(t).Header.Get("X-B"))
	// --dry-run in each position sends nothing.
	before := len(f.requests())
	for _, args := range [][]string{{"--dry-run", "pet", "list-users"}, {"pet", "--dry-run", "list-users"}, {"pet", "list-users", "--dry-run"}} {
		code, out, _ := runIO(t, args...)
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "curl")
		assert.NotContains(t, out, "tok-secret-value")
	}
	assert.Eq(t, before, len(f.requests()))
}

func TestDynamicAPIFlag(t *testing.T) {
	f, _ := setup(t)
	for _, args := range [][]string{
		{"--api", "pet", "list-users", "--page", "2"},
		{"--api=pet", "list-users", "--page", "2"},
		{"--env", "test", "--api", "pet", "list-users", "--page", "2"},
	} {
		code, _, errOut := runIO(t, args...)
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "/test/users", f.last(t).Path)
		assert.Eq(t, "page=2", f.last(t).Query)
	}
	n := len(f.requests())
	code, out, errOut := runIO(t, "--api", "bogus", "list-users")
	assert.Eq(t, 2, code)
	assert.Eq(t, "", out)
	assert.Contains(t, errOut, "bogus")
	code, out, errOut = runIO(t, "--bogus", "pet", "list-users")
	assert.Eq(t, 2, code)
	assert.Eq(t, "", out)
	assert.Contains(t, errOut, "--bogus")
	assert.Eq(t, n, len(f.requests()))
}

func TestDynamicUnknownOperation(t *testing.T) {
	f, _ := setup(t)
	code, out, errOut := runIO(t, "pet", "list-userz")
	assert.Eq(t, 2, code)
	assert.Eq(t, "", out)
	assert.Contains(t, errOut, "unknown operation")
	assert.Contains(t, errOut, "list-userz")
	assert.Eq(t, 0, len(f.requests()))
	// full URLs and protocol-relative paths are usage errors too.
	for _, p := range []string{"//evil.example/x", "http://evil.example/x"} {
		code, _, _ = runIO(t, "pet", p)
		assert.Eq(t, 2, code)
	}
	// -X is not valid together with an operation name.
	code, _, _ = runIO(t, "pet", "list-users", "-X", "POST")
	assert.Eq(t, 2, code)
	assert.Eq(t, 0, len(f.requests()))
}

func TestDynamicNoArgsListsOperations(t *testing.T) {
	f, _ := setup(t)
	code, out, _ := runIO(t, "pet")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "list-users")
	assert.Contains(t, out, "create-user")
	assert.Eq(t, 0, len(f.requests()))
	code, _, _ = runIO(t, "pet", "-X", "POST")
	assert.Eq(t, 2, code)
}

func TestDynamicRawPath(t *testing.T) {
	f, _ := setup(t)
	code, out, errOut := runIO(t, "pet", "-X", "POST", "/api/users", "-f", "name=tom")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "ok")
	r := f.last(t)
	assert.Eq(t, "POST", r.Method)
	// raw path: base_path is not applied.
	assert.Eq(t, "/api/users", r.Path)
	assert.Eq(t, `{"name":"tom"}`, r.Body)
	assert.Eq(t, "Bearer tok-secret-value", r.Header.Get("Authorization"))
	// -X after the path, -p query, -f nested and :=
	code, _, errOut = runIO(t, "pet", "/api/users", "-X", "POST", "-p", "a=1", "-f", "u.age:=3", "--yes")
	assert.Eq(t, 0, code, errOut)
	r = f.last(t)
	assert.Eq(t, "a=1", r.Query)
	assert.Eq(t, `{"u":{"age":3}}`, r.Body)
	// -d literal and stdin
	code, _, errOut = runIO(t, "pet", "-X", "PUT", "/api/users/1", "-d", `{"x":1}`)
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, `{"x":1}`, f.last(t).Body)
	// -d - reads the body from stdin.
	in, inw, err := os.Pipe()
	assert.Require(t, assert.NoError(t, err))
	_, err = inw.WriteString(`{"from":"stdin"}`)
	assert.NoError(t, err)
	assert.NoError(t, inw.Close())
	oldIn := os.Stdin
	os.Stdin = in
	code, _, errOut = runIO(t, "pet", "-X", "POST", "/api/users", "-d", "-", "--yes")
	os.Stdin = oldIn
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, `{"from":"stdin"}`, f.last(t).Body)
	// GET default method
	code, _, _ = runIO(t, "pet", "/api/ping")
	assert.Eq(t, 0, code)
	assert.Eq(t, "GET", f.last(t).Method)
}

func TestDynamicOperationParams(t *testing.T) {
	f, _ := setup(t)
	code, _, errOut := runIO(t, "pet", "get-user", "--id", "7")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "/test/users/7", f.last(t).Path)
	// "env" clashes with the global option: use -p; --env stays global.
	code, _, errOut = runIO(t, "pet", "list-users", "-p", "env=blue", "--page", "3")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "/test/users", f.last(t).Path)
	assert.Eq(t, "env=blue&page=3", f.last(t).Query)
	code, _, _ = runIO(t, "pet", "create-user", "-d", `{"n":1}`, "--yes")
	assert.Eq(t, 0, code)
	assert.Eq(t, `{"n":1}`, f.last(t).Body)
	assert.Eq(t, "POST", f.last(t).Method)
	// unknown parameter option is a usage error.
	n := len(f.requests())
	code, _, _ = runIO(t, "pet", "list-users", "--nope", "1")
	assert.Eq(t, 2, code)
	assert.Eq(t, n, len(f.requests()))
}

func TestDynamicBasePathOpenAPI(t *testing.T) {
	f, _ := setup(t)
	spec, err := filepath.Abs(filepath.Join("..", "spec", "testdata", "min-v2.json"))
	assert.NoError(t, err)
	code, _, errOut := runIO(t, "site", "add", "demo", "--spec", spec, "--base-url", f.srv.URL)
	assert.Eq(t, 0, code, errOut)
	// "verbose" clashes with the global option, so the parameter goes through -p.
	code, _, errOut = runIO(t, "demo", "get-pet", "--id", "5", "-p", "verbose=yes")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "/api/v1/pets/5", f.last(t).Path)
	assert.Eq(t, "verbose=yes", f.last(t).Query)
	// enum is validated before the request: exit 1.
	n := len(f.requests())
	code, _, _ = runIO(t, "demo", "get-pet", "--id", "5", "-p", "verbose=maybe")
	assert.Eq(t, 1, code)
	assert.Eq(t, n, len(f.requests()))
	// env base_path overrides the definition's.
	cfgFile := filepath.Join(os.Getenv("APIX_CONFIG_DIR"), "sites", "demo.yaml")
	data, err := os.ReadFile(cfgFile)
	assert.NoError(t, err)
	data = []byte(strings.Replace(string(data), "base_url:", "base_path: /override\n    base_url:", 1))
	assert.NoError(t, os.WriteFile(cfgFile, data, 0600))
	code, _, errOut = runIO(t, "demo", "get-pet", "--id", "5")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "/override/pets/5", f.last(t).Path)
}

func TestExitCodes(t *testing.T) {
	setup(t)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"0 ok", []string{"pet", "status", "--code", "200"}, 0},
		{"0 redirect not followed", []string{"pet", "status", "--code", "302"}, 0},
		{"1 missing required param", []string{"pet", "get-user"}, 1},
		{"1 unknown site selection", []string{"pet", "list-users", "--env", "nope"}, 1},
		{"1 invalid type", []string{"pet", "-X", "POST", "/x", "-d", "@/definitely/missing.json", "--yes"}, 1},
		{"2 unknown operation", []string{"pet", "list-userz"}, 2},
		{"2 bad output", []string{"pet", "list-users", "-o", "xml"}, 2},
		{"3 timeout", []string{"pet", "slow", "--timeout", "50ms"}, 3},
		{"4 client error", []string{"pet", "status", "--code", "404"}, 4},
		{"5 server error", []string{"pet", "status", "--code", "503"}, 5},
		{"6 protected", []string{"pet", "create-user", "--env", "prod", "-d", "{}"}, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runIO(t, tc.args...)
			assert.Eq(t, tc.want, code, errOut)
			if tc.want == 4 || tc.want == 5 {
				// the body of an error response still goes to stdout.
				assert.Contains(t, out, `"code"`)
			}
		})
	}
}

func TestExitCodesNetworkRefused(t *testing.T) {
	_, dir := setup(t)
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "sites"), 0700))
	site := "source: {type: simple, file: " + filepath.Join(dir, "pet-def.yaml") + "}\ndefault_env: e\nenvs: {e: {base_url: 'http://127.0.0.1:1'}}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "sites", "down.yaml"), []byte(site), 0600))
	code, _, errOut := runIO(t, "down", "list-users")
	assert.Eq(t, 3, code, errOut)
}

func TestProtect(t *testing.T) {
	f, _ := setup(t)
	// write on a protected env without --yes in a non-terminal: 6, and no login request.
	code, _, errOut := runIO(t, "pet", "create-user", "--env", "prod", "--auth", "lg", "-d", "{}")
	assert.Eq(t, 6, code)
	assert.Contains(t, errOut, "--yes")
	assert.Eq(t, 0, f.loginCount())
	assert.Eq(t, 0, len(f.requests()))
	// raw path call is protected as well, lower case method included.
	code, _, _ = runIO(t, "pet", "-X", "post", "/x", "--env", "prod", "--auth", "lg")
	assert.Eq(t, 6, code)
	assert.Eq(t, 0, f.loginCount())
	// reads are fine and log in.
	code, _, errOut = runIO(t, "pet", "list-users", "--env", "prod", "--auth", "lg")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, 1, f.loginCount())
	// --yes lets the write through (cached token, no second login).
	code, _, errOut = runIO(t, "pet", "create-user", "--env", "prod", "--auth", "lg", "-d", "{}", "--yes")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, 1, f.loginCount())
	assert.Eq(t, "POST", f.last(t).Method)
	// dry-run is not subject to protection and sends nothing.
	n := len(f.requests())
	code, out, _ := runIO(t, "pet", "create-user", "--env", "prod", "-d", "{}", "--dry-run")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "POST")
	assert.Eq(t, n, len(f.requests()))
	// allow_methods refuses writes with exit 6 even on an unprotected env.
	code, _, errOut = runIO(t, "pet", "create-user", "--auth", "ro", "-d", "{}")
	assert.Eq(t, 6, code)
	assert.Contains(t, errOut, "allow_methods")
	assert.Eq(t, n, len(f.requests()))
}

func TestDynamicIncludeRedactsHeaders(t *testing.T) {
	setup(t)
	code, out, errOut := runIO(t, "pet", "-i", "/cookie")
	assert.Eq(t, 0, code)
	assert.Contains(t, errOut, "HTTP 200")
	assert.Contains(t, errOut, "Set-Cookie: ***")
	assert.NotContains(t, errOut, "cookie-secret-value")
	assert.NotContains(t, out, "cookie-secret-value")
	assert.Contains(t, out, `"ok"`)
	assert.NotContains(t, out, "HTTP 200")
}

func TestDynamicVerboseAndErrorsRedacted(t *testing.T) {
	setup(t)
	code, out, errOut := runIO(t, "pet", "-v", "list-users")
	assert.Eq(t, 0, code)
	assert.Contains(t, errOut, "GET")
	assert.Contains(t, errOut, "Authorization: ***")
	assert.NotContains(t, errOut, "tok-secret-value")
	assert.NotContains(t, out, "tok-secret-value")
	// a registered secret that ends up in an error message is masked.
	code, out, errOut = runIO(t, "pet", "-X", "PUT", "/x", "-d", "@tok-secret-value")
	assert.Eq(t, 1, code)
	assert.Eq(t, "", out)
	assert.Contains(t, errOut, "read body file")
	assert.NotContains(t, errOut, "tok-secret-value")
}

func TestDynamicOutputModes(t *testing.T) {
	setup(t)
	code, out, _ := runIO(t, "pet", "list-users", "-o", "json")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "{\n  \"ok\": true\n}")
	code, out, _ = runIO(t, "pet", "list-users", "-o", "raw")
	assert.Eq(t, 0, code)
	assert.Eq(t, `{"ok":true}`, out)
	// ops accepts the shared -o.
	code, out, _ = runIO(t, "ops", "pet", "-o", "json")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, `"list-users"`)
}

func TestUseCommand(t *testing.T) {
	f, dir := setup(t)
	code, out, errOut := runIO(t, "use", "pet")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "env: test")
	assert.Contains(t, out, "auth: tok")
	code, out, errOut = runIO(t, "use", "pet", "--env", "alt", "--auth", "ro")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "env: alt")
	state, err := os.ReadFile(filepath.Join(dir, "state.yaml"))
	assert.NoError(t, err)
	assert.Contains(t, string(state), "alt")
	// the selection applies without --env/--auth.
	code, _, errOut = runIO(t, "pet", "list-users")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "/alt/users", f.last(t).Path)
	// a single-call option wins over the saved selection.
	code, _, _ = runIO(t, "pet", "list-users", "--env", "test")
	assert.Eq(t, 0, code)
	assert.Eq(t, "/test/users", f.last(t).Path)
	// allow_methods of the saved auth "ro" applies.
	code, _, _ = runIO(t, "pet", "create-user", "-d", "{}")
	assert.Eq(t, 6, code)
	// only the changed part is replaced.
	code, out, _ = runIO(t, "use", "pet", "--env", "prod")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "env: prod")
	assert.Contains(t, out, "auth: ro")
	// unknown names are rejected and do not change the state.
	code, _, _ = runIO(t, "use", "pet", "--env", "nope")
	assert.Eq(t, 1, code)
	code, _, _ = runIO(t, "use", "pet", "--auth", "nope")
	assert.Eq(t, 1, code)
	code, _, _ = runIO(t, "use", "missing")
	assert.Eq(t, 1, code)
	code, out, _ = runIO(t, "use", "pet")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "env: prod")
}

func TestAuthCmd(t *testing.T) {
	f, _ := setup(t)
	code, out, errOut := runIO(t, "auth", "ls", "pet")
	assert.Eq(t, 0, code, errOut)
	for _, want := range []string{"tok", "ro", "lg", "bearer", "login"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "tok-secret-value")
	// static auths need no login and have no state.
	code, out, _ = runIO(t, "auth", "login", "pet")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "no login needed")
	assert.Eq(t, 0, f.loginCount())
	code, out, _ = runIO(t, "auth", "status", "pet")
	assert.Eq(t, 0, code)
	assert.NotContains(t, out, "tok-secret-value")
	// login auth: status before, login, status, logout.
	code, out, _ = runIO(t, "auth", "status", "pet", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "logged out")
	code, out, errOut = runIO(t, "auth", "login", "pet", "--auth", "lg", "-v")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "logged in")
	assert.Eq(t, 1, f.loginCount())
	for _, s := range []string{out, errOut} {
		assert.NotContains(t, s, loginToken)
		assert.NotContains(t, s, "pass-secret-value")
	}
	code, out, _ = runIO(t, "auth", "status", "pet", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "logged in")
	assert.Contains(t, out, "expires")
	assert.NotContains(t, out, loginToken)
	// a call reuses the cached token; login again forces a new login.
	code, _, _ = runIO(t, "pet", "list-users", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.Eq(t, 1, f.loginCount())
	assert.Eq(t, "Bearer "+loginToken, f.last(t).Header.Get("Authorization"))
	code, _, _ = runIO(t, "auth", "login", "pet", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.Eq(t, 2, f.loginCount())
	code, out, _ = runIO(t, "auth", "logout", "pet", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.NotContains(t, out, loginToken)
	code, out, _ = runIO(t, "auth", "status", "pet", "--auth", "lg")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "logged out")
	// unknown site / auth.
	code, _, _ = runIO(t, "auth", "status", "missing")
	assert.Eq(t, 1, code)
	code, _, _ = runIO(t, "auth", "status", "pet", "--auth", "nope")
	assert.Eq(t, 1, code)
}

func TestDynamicBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("version: 1\nsites: [broken\n"), 0600))
	// management commands that do not need the configuration still work.
	code, out, _ := runIO(t, "config", "path")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, dir)
	code, _, _ = runIO(t, "--version")
	assert.Eq(t, 0, code)
	// a call that targets a site reports the configuration error.
	code, _, errOut := runIO(t, "pet", "list-users")
	assert.Eq(t, 1, code)
	assert.Contains(t, errOut, "config.yaml")
	code, _, _ = runIO(t, "--api", "pet", "list-users")
	assert.Eq(t, 1, code)
	code, _, _ = runIO(t, "--bogus", "config", "path")
	assert.Eq(t, 2, code)
}

func TestDynamicEnumInOptionHelp(t *testing.T) {
	f, dir := setup(t)
	data, err := os.ReadFile(filepath.Join("..", "spec", "testdata", "min-v2.json"))
	assert.NoError(t, err)
	spec := strings.Replace(string(data), `{"name": "q", "in": "query", "type": "string"}`, `{"name": "keyword", "in": "query", "type": "string", "enum": ["aa", "bb"]}`, 1)
	assert.NotEq(t, string(data), spec)
	file := filepath.Join(dir, "enum.json")
	assert.NoError(t, os.WriteFile(file, []byte(spec), 0600))
	code, _, errOut := runIO(t, "site", "add", "enumsite", "--spec", file, "--base-url", f.srv.URL)
	assert.Eq(t, 0, code, errOut)
	code, out, _ := runIO(t, "enumsite", "pets-search", "--help")
	assert.Eq(t, 0, code)
	assert.Contains(t, out, "aa, bb")
	code, _, errOut = runIO(t, "enumsite", "pets-search", "--keyword", "bb")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "keyword=bb", f.last(t).Query)
	code, _, _ = runIO(t, "enumsite", "pets-search", "--keyword", "zz")
	assert.Eq(t, 1, code)
}

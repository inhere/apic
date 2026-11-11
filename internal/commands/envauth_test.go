package commands_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

const eaDef = `operations:
  me:
    get: /me
  status:
    get: /status/{code}
  items:
    get: /items
    params:
      - {name: state, in: query, enum: [opened, closed, merged]}
`

// eaWrite writes config.yaml, the simple definition and .env into a fresh config dir.
func eaWrite(t *testing.T, cfgFmt, env string, args ...any) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	def := filepath.Join(dir, "def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(eaDef), 0o600))
	all := append([]any{def}, args...)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(fmt.Sprintf(cfgFmt, all...)), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600))
	return dir
}

// eaDh writes site "dh" (per-environment auths) and a credential-less site "bare".
func eaDh(t *testing.T) (*fakeAPI, string, string) {
	t.Helper()
	f := newFakeAPI(t)
	scripts := filepath.Join(t.TempDir(), "links")
	cfg := `version: 1
defaults: {link_dir: %[3]q}
sites:
  dh:
    source: {type: simple, file: %[1]q}
    default_env: build
    default_auth: dflt
    envs:
      build: {base_url: %[2]q, auth: build}
      factory: {base_url: %[2]q, auth: factory}
      plain: {base_url: %[2]q}
      bad: {base_url: %[2]q, auth: ghost}
    auths:
      build: {type: bearer, token: '${env:EA_BUILD}'}
      factory: {type: bearer, token: '${env:EA_FACTORY}', allow_methods: [GET, HEAD]}
      other: {type: bearer, token: '${env:EA_OTHER}'}
      dflt: {type: bearer, token: '${env:EA_DFLT}'}
  bare:
    source: {type: simple, file: %[1]q}
    default_env: test
    envs:
      test: {base_url: %[2]q}
`
	dir := eaWrite(t, cfg, "EA_BUILD=tok-build\nEA_FACTORY=tok-factory\nEA_OTHER=tok-other\nEA_DFLT=tok-dflt\n", f.srv.URL, scripts)
	return f, dir, scripts
}

func eaBearer(f *fakeAPI, t *testing.T) string {
	t.Helper()
	return f.last(t).Header.Get("Authorization")
}

func TestEnvAuthPrecedence(t *testing.T) {
	f, _, _ := eaDh(t)
	t.Run("no auth", func(t *testing.T) {
		code, _, errOut := runIO(t, "bare", "me")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "", eaBearer(f, t))
	})
	t.Run("default_auth", func(t *testing.T) {
		code, _, errOut := runIO(t, "dh", "me", "--env", "plain")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-dflt", eaBearer(f, t))
	})
	t.Run("env declared beats default_auth", func(t *testing.T) {
		code, _, errOut := runIO(t, "dh", "me", "--env", "factory")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-factory", eaBearer(f, t))
	})
	t.Run("flag beats env declared", func(t *testing.T) {
		code, _, errOut := runIO(t, "dh", "me", "--env", "factory", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-other", eaBearer(f, t))
	})
	t.Run("saved beats default_auth only without env auth", func(t *testing.T) {
		code, _, errOut := runIO(t, "use", "dh", "--env", "plain", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		code, _, errOut = runIO(t, "dh", "me")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-other", eaBearer(f, t))
		code, _, errOut = runIO(t, "dh", "me", "--env", "build")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-build", eaBearer(f, t))
	})
	t.Run("env declares an unknown auth", func(t *testing.T) {
		code, _, errOut := runIO(t, "dh", "me", "--env", "bad")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, `site "dh" env "bad": no auth "ghost"`)
		code, _, errOut = runIO(t, "dh", "me", "--auth", "ghost")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, `site "dh" has no auth "ghost"`)
	})
}

func TestEnvAuthScenario(t *testing.T) {
	f, _, _ := eaDh(t)
	code, out, errOut := runIO(t, "use", "dh", "--env", "factory", "--auth", "build")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, errOut, `note: env "factory" declares auth "factory"; the saved auth applies only to environments without one`)
	// the saved auth is shadowed by the declaration of the saved env.
	assert.Contains(t, out, "auth: factory (env)\n")

	t.Run("saved auth does not reach the declaring env", func(t *testing.T) {
		code, _, errOut := runIO(t, "dh", "me", "--env", "factory")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "Bearer tok-factory", eaBearer(f, t))
	})
	t.Run("allow_methods applies to the declared auth", func(t *testing.T) {
		before := len(f.requests())
		code, _, errOut := runIO(t, "dh", "-X", "POST", "/things", "--env", "factory", "--yes")
		assert.Eq(t, 6, code)
		assert.Contains(t, errOut, "not allowed by this auth (allow_methods)")
		assert.Eq(t, before, len(f.requests()))
	})
	t.Run("status shows the declared auth", func(t *testing.T) {
		code, out, errOut := runIO(t, "auth", "status", "dh", "--env", "factory")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "env=factory auth=factory")
	})
	t.Run("note follows the saved env", func(t *testing.T) {
		code, _, errOut := runIO(t, "use", "dh", "--env", "plain")
		assert.Eq(t, 0, code, errOut)
		code, _, errOut = runIO(t, "use", "dh", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		assert.NotContains(t, errOut, "note:")
		code, _, errOut = runIO(t, "use", "dh", "--env", "build", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, errOut, `note: env "build" declares auth "build"`)
		// the same auth as the env declares needs no note.
		code, _, errOut = runIO(t, "use", "dh", "--auth", "build")
		assert.Eq(t, 0, code, errOut)
		assert.NotContains(t, errOut, "note:")
	})
}

func TestEnvAuthDisplay(t *testing.T) {
	_, dir, _ := eaDh(t)
	t.Run("use suffixes", func(t *testing.T) {
		code, out, errOut := runIO(t, "use", "dh")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "env: build\nauth: build (env)\n")
		code, out, errOut = runIO(t, "use", "dh", "--env", "plain")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "auth: dflt (default)\n")
		code, out, errOut = runIO(t, "use", "dh", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "auth: other (saved)\n")
		code, out, errOut = runIO(t, "use", "bare")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "auth: (none)\n")
	})
	t.Run("display tolerates a mistyped auth", func(t *testing.T) {
		code, out, errOut := runIO(t, "use", "dh", "--env", "bad")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "auth: ghost (env)\n")
		code, _, errOut = runIO(t, "auth", "ls", "dh")
		assert.Eq(t, 0, code, errOut)
	})
	t.Run("auth ls marks the effective auth of the saved env", func(t *testing.T) {
		code, _, errOut := runIO(t, "use", "dh", "--env", "factory", "--auth", "other")
		assert.Eq(t, 0, code, errOut)
		code, out, errOut := runIO(t, "auth", "ls", "dh")
		assert.Eq(t, 0, code, errOut)
		marked := ""
		for _, line := range strings.Split(out, "\n") {
			if strings.HasSuffix(line, "*") {
				marked += strings.Fields(line)[0] + ";"
			}
		}
		assert.Eq(t, "factory;", marked)
		assert.NoError(t, os.Remove(filepath.Join(dir, "state.yaml")))
		code, out, errOut = runIO(t, "auth", "ls", "dh")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "build")
		for _, line := range strings.Split(out, "\n") {
			if strings.HasSuffix(line, "*") {
				assert.Eq(t, "build", strings.Fields(line)[0])
			}
		}
	})
}

func TestEnvAuthLogoutOnlyThatIdentity(t *testing.T) {
	f := newFakeAPI(t)
	cfg := `version: 1
sites:
  lo:
    source: {type: simple, file: %[1]q}
    default_env: e1
    envs:
      e1: {base_url: %[2]q, auth: a}
      e2: {base_url: %[2]q, auth: a}
    auths:
      a: {type: login, login: {method: POST, path: /api/login, body: {user: u}}, token: {from: body, path: data.token}, expires: {from: body, path: data.expires_in}, apply: {header: Authorization, value: "Bearer {token}"}}
      b: {type: login, login: {method: POST, path: /api/login, body: {user: u}}, token: {from: body, path: data.token}, expires: {from: body, path: data.expires_in}, apply: {header: Authorization, value: "Bearer {token}"}}
`
	dir := eaWrite(t, cfg, "", f.srv.URL)
	for _, args := range [][]string{{"--env", "e1"}, {"--env", "e1", "--auth", "b"}, {"--env", "e2"}} {
		code, _, errOut := runIO(t, append([]string{"lo", "me"}, args...)...)
		assert.Eq(t, 0, code, errOut)
	}
	assert.Eq(t, []string{"lo.e1.a.json", "lo.e1.b.json", "lo.e2.a.json"}, eaCacheNames(t, dir))
	code, out, errOut := runIO(t, "auth", "logout", "lo", "--env", "e1", "--auth", "b")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "logged out")
	assert.Eq(t, []string{"lo.e1.a.json", "lo.e2.a.json"}, eaCacheNames(t, dir))
}

func eaCacheNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(dir, "cache", "tokens"))
	if os.IsNotExist(err) {
		return []string{}
	}
	assert.NoError(t, err)
	names := []string{}
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestEnvAuthLink(t *testing.T) {
	f, _, scripts := eaDh(t)
	code, _, errOut := runIO(t, "link", "dh", "--name", "dhctl", "--env", "factory")
	assert.Eq(t, 0, code, errOut)
	data, err := os.ReadFile(filepath.Join(scripts, "dhctl"))
	assert.Require(t, assert.NoError(t, err))
	assert.Contains(t, string(data), "--api dh --env factory")
	assert.NotContains(t, string(data), "--auth")
	// the saved selection must not change what the shortcut sends.
	code, _, errOut = runIO(t, "use", "dh", "--auth", "other")
	assert.Eq(t, 0, code, errOut)
	code, _, errOut = runIO(t, "--api", "dh", "--env", "factory", "me")
	assert.Eq(t, 0, code, errOut)
	assert.Eq(t, "Bearer tok-factory", eaBearer(f, t))
}

func TestEnvAuthConfigCheck(t *testing.T) {
	f, _, _ := eaDh(t)
	_ = f
	code, out, errOut := runIO(t, "config", "check")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, `site dh env bad: auth "ghost" is not defined`)
	assert.NotContains(t, out, "default_auth")
	f2 := newFakeAPI(t)
	eaWrite(t, `version: 1
sites:
  dh:
    source: {type: simple, file: %[1]q}
    default_auth: nope
    envs:
      bad: {base_url: %[2]q, auth: ghost}
`, "", f2.srv.URL)
	code, out, errOut = runIO(t, "config", "check")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, `site dh: default_auth "nope" is not defined`)
	assert.Contains(t, out, `site dh env bad: auth "ghost" is not defined`)
}

func TestEnvAuthSkill(t *testing.T) {
	f, _, _ := eaDh(t)
	_ = f
	out := filepath.Join(t.TempDir(), "skills")
	code, _, errOut := runIO(t, "skill", "dh", "--out", out)
	assert.Eq(t, 0, code, errOut)
	data, err := os.ReadFile(filepath.Join(out, "apic-dh", "SKILL.md"))
	assert.Require(t, assert.NoError(t, err))
	text := string(data)
	assert.Contains(t, text, "- build (default); auth build\n")
	assert.Contains(t, text, "- factory; auth factory\n")
	assert.Contains(t, text, "An explicit --auth overrides the environment's auth; pass --env explicitly.")
	assert.NotContains(t, text, "tok-")
}

func TestSimpleEnumE2E(t *testing.T) {
	f := newFakeAPI(t)
	dir := eaWrite(t, `version: 1
sites:
  en:
    source: {type: simple, file: %[1]q}
    default_env: test
    envs:
      test: {base_url: %[2]q}
`, "", f.srv.URL)
	code, out, errOut := runIO(t, "desc", "en", "items")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "opened,closed,merged")
	code, _, errOut = runIO(t, "en", "items", "--state", "merged")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, f.last(t).Query, "state=merged")
	before := len(f.requests())
	code, _, errOut = runIO(t, "en", "items", "--state", "bogus")
	assert.Eq(t, 1, code)
	assert.Contains(t, errOut, "bogus")
	assert.Eq(t, before, len(f.requests()))
	// an empty enum list is a definition error: the site does not load.
	bad := strings.Replace(eaDef, "enum: [opened, closed, merged]", "enum: []", 1)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "def.yaml"), []byte(bad), 0o600))
	code, _, errOut = runIO(t, "en", "items")
	assert.Eq(t, 1, code)
	assert.Contains(t, errOut, "enum")
}

// eaSnapshot records the names, contents and mtimes of the token cache files.
func eaSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	root := filepath.Join(dir, "cache", "tokens")
	for _, name := range eaCacheNames(t, dir) {
		b, err := os.ReadFile(filepath.Join(root, name))
		assert.NoError(t, err)
		st, err := os.Stat(filepath.Join(root, name))
		assert.NoError(t, err)
		snap[name] = string(b) + "|" + st.ModTime().Format(time.RFC3339Nano)
	}
	return snap
}

func TestDryRunMissingVars(t *testing.T) {
	f := newFakeAPI(t)
	stub := buildExecStub(t)
	count := filepath.Join(t.TempDir(), "count")
	marker := filepath.Join(t.TempDir(), "marker")
	q := func(s string) string { return fmt.Sprintf("%q", s) }
	cfg := `version: 1
sites:
  dr:
    source: {type: simple, file: %[1]q}
    default_env: t
    envs:
      t: {base_url: %[2]q}
    auths:
      bearer: {type: bearer, token: '${env:EA_MISS_BEARER}'}
      multi: {type: bearer, token: '${env:EA_MISS_Z}${env:EA_MISS_B}${env:EA_MISS_Z}'}
      mix: {type: bearer, token: '${env:EA_SET}-${env:EA_MISS_MIX}'}
      keyh: {type: apikey, in: header, name: X-Key, value: '${env:EA_SET}-${env:EA_MISS_KEYH}'}
      keyq: {type: apikey, in: query, name: k, value: '${env:EA_SET}-${env:EA_MISS_KEYQ}'}
      basic: {type: basic, username: user, password: '${env:EA_MISS_PASS}'}
      lgm:
        type: login
        login: {method: POST, path: /api/login, body: {user: u, password: '${env:EA_MISS_LOGIN}'}}
        token: {from: body, path: data.token}
        expires: {from: body, path: data.expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
      exm: {type: exec, exec: {command: [%[3]s, %[4]s, "token=tok-exec"], ttl: 1h, env: {K: '${env:EA_MISS_EXENV}'}}}
      exc: {type: exec, exec: {command: [%[3]s, %[5]s, "token=tok-exec", '${env:EA_MISS_EXCMD}'], ttl: 1h}}
      nest: {type: bearer, token: '${env:EA_NEST}'}
      nouser: {type: basic, username: '${env:EA_MISS_USER}', password: pw}
      nomethods: {type: bearer, token: lit, allow_methods: ['${env:EA_MISS_METHODS}']}
      nohdr:
        type: login
        login: {method: POST, path: /api/login, body: {user: u}}
        token: {from: body, path: data.token}
        apply: {header: '${env:EA_MISS_HDR}', value: "Bearer {token}"}
      noval:
        type: login
        login: {method: POST, path: /api/login, body: {user: u}}
        token: {from: body, path: data.token}
        apply: {header: Authorization, value: '${env:EA_MISS_APPLYVAL}'}
      nopath:
        type: login
        login: {method: POST, path: '${env:EA_MISS_PATH}', body: {user: u}}
        token: {from: body, path: data.token}
        apply: {header: Authorization, value: "Bearer {token}"}
  src:
    source: {type: openapi, url: '%[2]s/openapi.json', auth: srcauth}
    default_env: t
    envs:
      t: {base_url: %[2]q}
    auths:
      srcauth: {type: bearer, token: '${env:EA_MISS_SRC}'}
`
	// the definition path (arg 1) is shared; the site "src" ignores it.
	env := "EA_SET=set-secret-value\nEA_NEST=${env:EA_MISS_NESTED}\nEA_MISS_LOGIN=pw-seed\nEA_MISS_EXENV=env-seed\n"
	dir := eaWrite(t, cfg, env, f.srv.URL, q(stub), q("count="+count), q("count="+marker))
	envPath := filepath.Join(dir, ".env")

	missing := []struct{ auth, name string }{
		{"bearer", "EA_MISS_BEARER"}, {"mix", "EA_MISS_MIX"}, {"keyh", "EA_MISS_KEYH"}, {"keyq", "EA_MISS_KEYQ"},
		{"basic", "EA_MISS_PASS"}, {"exc", "EA_MISS_EXCMD"},
	}
	// login and exec(env) variables are set while seeding their caches, then removed.
	seedCode := func(auth string) {
		code, _, errOut := runIO(t, "dr", "me", "--auth", auth)
		assert.Eq(t, 0, code, errOut)
	}
	seedCode("lgm")
	seedCode("exm")
	assert.Eq(t, 1, countLines(t, count))
	assert.NoError(t, os.WriteFile(envPath, []byte("EA_SET=set-secret-value\nEA_NEST=${env:EA_MISS_NESTED}\n"), 0o600))
	missing = append(missing, struct{ auth, name string }{"lgm", "EA_MISS_LOGIN"}, struct{ auth, name string }{"exm", "EA_MISS_EXENV"})

	t.Run("dry-run warns and exits 0; real call exits 1", func(t *testing.T) {
		for _, m := range missing {
			code, out, errOut := runIO(t, "dr", "me", "--auth", m.auth, "--dry-run")
			assert.Eq(t, 0, code, m.auth+": "+errOut)
			assert.Eq(t, "warning: dry-run: "+m.name+" is not set; a real call would fail\n", errOut, m.auth)
			assert.NotContains(t, out+errOut, "set-secret-value", m.auth)
			code, _, errOut = runIO(t, "dr", "me", "--auth", m.auth)
			assert.Eq(t, 1, code, m.auth+": "+errOut)
		}
	})
	t.Run("warnings are sorted and deduplicated", func(t *testing.T) {
		code, _, errOut := runIO(t, "dr", "me", "--auth", "multi", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "warning: dry-run: EA_MISS_B is not set; a real call would fail\nwarning: dry-run: EA_MISS_Z is not set; a real call would fail\n", errOut)
	})
	t.Run("a value holding a reference is not expanded again", func(t *testing.T) {
		code, _, errOut := runIO(t, "dr", "me", "--auth", "nest", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "", errOut)
	})
	t.Run("plain fields still fail", func(t *testing.T) {
		for _, a := range []string{"nouser", "nomethods", "nohdr", "noval", "nopath"} {
			code, _, errOut := runIO(t, "dr", "me", "--auth", a, "--dry-run")
			assert.Eq(t, 1, code, a+": "+errOut)
		}
	})
	t.Run("auth status stays strict", func(t *testing.T) {
		code, _, errOut := runIO(t, "auth", "status", "dr", "--auth", "bearer")
		assert.Eq(t, 1, code, errOut)
		assert.Contains(t, errOut, "EA_MISS_BEARER")
	})
	t.Run("source auth stays strict", func(t *testing.T) {
		code, _, errOut := runIO(t, "src", "me", "--dry-run")
		assert.Eq(t, 1, code)
		assert.Contains(t, errOut, "apply definition credentials failed")
		assert.NotContains(t, errOut, "warning: dry-run")
	})
	t.Run("no command run, no cache written or removed", func(t *testing.T) {
		before := eaSnapshot(t, dir)
		assert.Eq(t, 2, len(before))
		time.Sleep(20 * time.Millisecond)
		for _, a := range []string{"lgm", "exm", "exc", "bearer"} {
			code, _, errOut := runIO(t, "dr", "me", "--auth", a, "--dry-run")
			assert.Eq(t, 0, code, errOut)
		}
		assert.Eq(t, before, eaSnapshot(t, dir))
		assert.Eq(t, 1, countLines(t, count))
		_, err := os.Stat(marker)
		assert.True(t, os.IsNotExist(err))
		assert.Eq(t, 1, f.loginCount())
	})
}

func TestInvalidateE2E(t *testing.T) {
	f := newFakeAPI(t)
	stub := buildExecStub(t)
	q := func(s string) string { return fmt.Sprintf("%q", s) }
	cfg := `version: 1
sites:
  iv:
    source: {type: simple, file: %[1]q}
    default_env: t
    envs:
      t: {base_url: %[2]q}
      t2: {base_url: %[2]q}
    auths:
      lgi:
        type: login
        login: {method: POST, path: /api/login, body: {user: u}}
        token: {from: body, path: data.token}
        expires: {from: body, path: data.expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
      exi: {type: exec, exec: {command: [%[3]s, "token=tok-exec"], ttl: 1h}}
      keep: {type: exec, exec: {command: [%[3]s, "token=tok-keep"], ttl: 1h}}
`
	dir := eaWrite(t, cfg, "", f.srv.URL, q(stub))
	// seed other identities that must survive.
	for _, args := range [][]string{{"--auth", "keep"}, {"--auth", "lgi", "--env", "t2"}} {
		code, _, errOut := runIO(t, append([]string{"iv", "me"}, args...)...)
		assert.Eq(t, 0, code, errOut)
	}
	for _, auth := range []string{"lgi", "exi"} {
		t.Run(auth, func(t *testing.T) {
			code, _, errOut := runIO(t, "iv", "me", "--auth", auth)
			assert.Eq(t, 0, code, errOut)
			assert.Contains(t, eaCacheNames(t, dir), "iv.t."+auth+".json")
			code, _, errOut = runIO(t, "iv", "status", "--code", "401", "--auth", auth)
			assert.Eq(t, 4, code, errOut)
			assert.NotContains(t, eaCacheNames(t, dir), "iv.t."+auth+".json")
			code, out, errOut := runIO(t, "auth", "status", "iv", "--auth", auth)
			assert.Eq(t, 0, code, errOut)
			assert.Contains(t, out, "logged out")
			assert.Contains(t, eaCacheNames(t, dir), "iv.t.keep.json")
			assert.Contains(t, eaCacheNames(t, dir), "iv.t2.lgi.json")
		})
	}
}

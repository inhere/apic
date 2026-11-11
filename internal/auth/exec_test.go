package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

// buildStub compiles testdata/stub into dir and returns the executable path.
func buildStub(t *testing.T, dir string) string {
	t.Helper()
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
	cmd.Dir = filepath.Join("testdata", "stub")
	cmd.Env = os.Environ()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, b)
	}
	return out
}

type execFixture struct {
	stub, cfgDir, cacheDir string
	deps                   Deps
	now                    time.Time
}

func newExecFixture(t *testing.T) *execFixture {
	t.Helper()
	f := &execFixture{cfgDir: t.TempDir(), cacheDir: t.TempDir(), now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	f.stub = buildStub(t, t.TempDir())
	f.deps = deps(t, f.cacheDir)
	f.deps.ConfigDir = f.cfgDir
	f.deps.Now = func() time.Time { return f.now }
	return f
}

func (f *execFixture) spec(ops ...string) model.AuthSpec {
	return model.AuthSpec{Type: "exec", Exec: model.ExecSpec{Command: append([]string{f.stub}, ops...)}}
}

func (f *execFixture) provider(t *testing.T, spec model.AuthSpec) *Provider {
	t.Helper()
	return mustNew(t, "dev", spec, model.Env{BaseURL: "http://example.test"}, f.deps)
}

func lines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func apply(t *testing.T, p *Provider) (*http.Request, error) {
	t.Helper()
	r := newReq(t)
	return r, p.Apply(context.Background(), r, false)
}

func TestExecText(t *testing.T) {
	f := newExecFixture(t)
	t.Run("bearer default", func(t *testing.T) {
		p := f.provider(t, f.spec("token=tok-abcd1"))
		r, err := apply(t, p)
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer tok-abcd1", r.Header.Get("Authorization"))
		assert.Eq(t, "***", p.redact("tok-abcd1"))
		assert.Eq(t, "***", p.redact("Bearer tok-abcd1"))
	})
	t.Run("custom header and query", func(t *testing.T) {
		s := f.spec("token=tok-abcd2")
		s.Apply = model.ApplySpec{Header: "X-Tok", Value: "{token}"}
		r, err := apply(t, f.provider(t, s))
		assert.NoErr(t, err)
		assert.Eq(t, "tok-abcd2", r.Header.Get("X-Tok"))
		assert.Eq(t, "", r.Header.Get("Authorization"))
		s.Apply = model.ApplySpec{Query: "access_token", Value: "{token}"}
		r, err = apply(t, f.provider(t, s))
		assert.NoErr(t, err)
		assert.Eq(t, "tok-abcd2", r.URL.Query().Get("access_token"))
	})
	t.Run("login apply is not used by exec", func(t *testing.T) {
		s := f.spec("token=tok-abcd3")
		s.Login.Apply = model.ApplySpec{Header: "X-Wrong"}
		r, err := apply(t, f.provider(t, s))
		assert.NoErr(t, err)
		assert.Eq(t, "", r.Header.Get("X-Wrong"))
		assert.Eq(t, "Bearer tok-abcd3", r.Header.Get("Authorization"))
	})
}

func TestExecTokenValidation(t *testing.T) {
	f := newExecFixture(t)
	for name, token := range map[string]string{
		"multi-line":      `token=aaaa\nbbbb`,
		"control char":    `token=aa\x01aa`,
		"tab inside":      `token=aa\taa`,
		"carriage return": `token=aaaa\rbbbb`,
		"empty":           `token=`,
		"blank":           `token=\x20\x20`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := apply(t, f.provider(t, f.spec(token)))
			assert.Err(t, err)
			assert.Eq(t, model.ExitLocal, code(err))
			assert.NotContains(t, err.Error(), "aaaa")
		})
	}
	t.Run("surrounding whitespace is trimmed", func(t *testing.T) {
		r, err := apply(t, f.provider(t, f.spec(`token=\x20\x20tok-trim1\x20`)))
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer tok-trim1", r.Header.Get("Authorization"))
	})
}

func (f *execFixture) jsonSpec(doc string, tokenPath, expPath, ttl string) model.AuthSpec {
	s := f.spec("json=" + doc)
	s.Exec.Format = "json"
	s.Exec.Token = model.ValueFrom{Path: tokenPath}
	s.Exec.Expires = model.ValueFrom{Path: expPath}
	s.Exec.TTL = ttl
	return s
}

func (f *execFixture) cached(t *testing.T, p *Provider) (cacheEntry, bool) {
	t.Helper()
	return p.readCache()
}

func TestExecJSON(t *testing.T) {
	f := newExecFixture(t)
	t.Run("token and numeric expires", func(t *testing.T) {
		p := f.provider(t, f.jsonSpec(`{"data":{"token":"jt-12345"},"expires_in":3600}`, "data.token", "expires_in", ""))
		r, err := apply(t, p)
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer jt-12345", r.Header.Get("Authorization"))
		e, ok := f.cached(t, p)
		assert.True(t, ok)
		assert.Eq(t, f.now.Add(time.Hour), e.ExpiresAt)
		assert.Eq(t, "jt-12345", e.Token)
	})
	t.Run("string expires", func(t *testing.T) {
		p := f.provider(t, f.jsonSpec(`{"t":"jt-12346","e":"120"}`, "t", "e", ""))
		_, err := apply(t, p)
		assert.NoErr(t, err)
		e, ok := f.cached(t, p)
		assert.True(t, ok)
		assert.Eq(t, f.now.Add(2*time.Minute), e.ExpiresAt)
	})
	t.Run("token must be a string", func(t *testing.T) {
		_, err := apply(t, f.provider(t, f.jsonSpec(`{"t":12345678}`, "t", "", "")))
		assert.Err(t, err)
		_, err = apply(t, f.provider(t, f.jsonSpec(`{"t":"a\nb\nc\nd"}`, "t", "", "")))
		assert.Err(t, err)
		_, err = apply(t, f.provider(t, f.jsonSpec(`{"t":"a\u0001bcde"}`, "t", "", "")))
		assert.Err(t, err)
		_, err = apply(t, f.provider(t, f.jsonSpec(`{"x":"abcdefg"}`, "t", "", "")))
		assert.Err(t, err)
		_, err = apply(t, f.provider(t, f.jsonSpec(`not json`, "t", "", "")))
		assert.Err(t, err)
		assert.Eq(t, model.ExitLocal, code(err))
	})
}

func TestExecExpiryAndCache(t *testing.T) {
	f := newExecFixture(t)
	cases := []struct {
		name      string
		spec      model.AuthSpec
		wantCache bool
		wantExp   time.Duration
	}{
		{"expires", f.jsonSpec(`{"t":"tok-exp-1","e":60}`, "t", "e", ""), true, time.Minute},
		{"expires wins over ttl", f.jsonSpec(`{"t":"tok-exp-2","e":60}`, "t", "e", "1h"), true, time.Minute},
		{"ttl only (text)", func() model.AuthSpec { s := f.spec("token=tok-ttl-1"); s.Exec.TTL = "30m"; return s }(), true, 30 * time.Minute},
		{"expires zero falls back to ttl", f.jsonSpec(`{"t":"tok-exp-3","e":0}`, "t", "e", "10m"), true, 10 * time.Minute},
		{"expires negative falls back to ttl", f.jsonSpec(`{"t":"tok-exp-4","e":-5}`, "t", "e", "10m"), true, 10 * time.Minute},
		{"expires unparseable falls back to ttl", f.jsonSpec(`{"t":"tok-exp-5","e":"soon"}`, "t", "e", "10m"), true, 10 * time.Minute},
		{"expires zero, no ttl", f.jsonSpec(`{"t":"tok-exp-6","e":0}`, "t", "e", ""), false, 0},
		{"expires negative, no ttl", f.jsonSpec(`{"t":"tok-exp-7","e":-1}`, "t", "e", ""), false, 0},
		{"neither", f.spec("token=tok-none-1"), false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = os.RemoveAll(f.cacheDir)
			p := f.provider(t, c.spec)
			_, err := apply(t, p)
			assert.NoErr(t, err)
			e, ok := p.readCache()
			assert.Eq(t, c.wantCache, ok)
			if ok {
				assert.Eq(t, f.now.Add(c.wantExp), e.ExpiresAt)
			}
			_, statErr := os.Stat(p.cachePath())
			assert.Eq(t, c.wantCache, statErr == nil)
			assert.Eq(t, c.spec.Exec.Expires.Path != "" || c.spec.Exec.TTL != "", p.Status().Cacheable)
			assert.Eq(t, c.wantCache, p.Status().LoggedIn)
		})
	}
}

func TestExecCacheReuse(t *testing.T) {
	f := newExecFixture(t)
	counter := filepath.Join(t.TempDir(), "count")
	spec := f.spec("token=tok-reuse-1", "count="+counter)
	spec.Exec.TTL = "1h"
	p := f.provider(t, spec)
	_, err := apply(t, p)
	assert.NoErr(t, err)
	_, err = apply(t, p)
	assert.NoErr(t, err)
	assert.Eq(t, 1, lines(t, counter))
	// A new process (provider) reads the cache.
	r, err := apply(t, f.provider(t, spec))
	assert.NoErr(t, err)
	assert.Eq(t, "Bearer tok-reuse-1", r.Header.Get("Authorization"))
	assert.Eq(t, 1, lines(t, counter))
	// Expired cache runs the command again.
	f.now = f.now.Add(2 * time.Hour)
	_, err = apply(t, f.provider(t, spec))
	assert.NoErr(t, err)
	assert.Eq(t, 2, lines(t, counter))

	t.Run("uncacheable runs per process, reused inside one", func(t *testing.T) {
		c2 := filepath.Join(t.TempDir(), "count")
		s := f.spec("token=tok-nc-1", "count="+c2)
		p := f.provider(t, s)
		_, _ = apply(t, p)
		_, _ = apply(t, p)
		assert.Eq(t, 1, lines(t, c2))
		_, _ = apply(t, f.provider(t, s))
		assert.Eq(t, 2, lines(t, c2))
	})
	t.Run("401 refresh runs the command again", func(t *testing.T) {
		c3 := filepath.Join(t.TempDir(), "count")
		s := f.spec("token=tok-401-1", "count="+c3)
		s.Exec.TTL = "1h"
		p := f.provider(t, s)
		_, _ = apply(t, p)
		assert.NoErr(t, p.Refresh(context.Background()))
		assert.Eq(t, 2, lines(t, c3))
		r, err := apply(t, p)
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer tok-401-1", r.Header.Get("Authorization"))
		assert.Eq(t, 2, lines(t, c3))
	})
	t.Run("dry run does not execute", func(t *testing.T) {
		c4 := filepath.Join(t.TempDir(), "count")
		s := f.spec("token=tok-dry-1", "count="+c4)
		r := newReq(t)
		assert.NoErr(t, f.provider(t, s).Apply(context.Background(), r, true))
		assert.Eq(t, "Bearer <exec-token>", r.Header.Get("Authorization"))
		assert.Eq(t, 0, lines(t, c4))
	})
	t.Run("dry run uses a valid cache", func(t *testing.T) {
		s := f.spec("token=tok-dry-2")
		s.Exec.TTL = "1h"
		_, err := apply(t, f.provider(t, s))
		assert.NoErr(t, err)
		r := newReq(t)
		assert.NoErr(t, f.provider(t, s).Apply(context.Background(), r, true))
		assert.Eq(t, "Bearer tok-dry-2", r.Header.Get("Authorization"))
	})
	t.Run("logout clears", func(t *testing.T) {
		s := f.spec("token=tok-out-1")
		s.Exec.TTL = "1h"
		p := f.provider(t, s)
		_, _ = apply(t, p)
		assert.True(t, p.Status().LoggedIn)
		assert.NoErr(t, p.Logout())
		assert.False(t, p.Status().LoggedIn)
	})
}

func TestExecZeroExpiryCacheIgnored(t *testing.T) {
	f := newExecFixture(t)
	counter := filepath.Join(t.TempDir(), "count")
	s := f.spec("token=tok-fresh-1", "count="+counter)
	s.Exec.TTL = "1h"
	p := f.provider(t, s)
	assert.NoErr(t, p.writeCache(cacheEntry{Token: "tok-stale-1", Fingerprint: p.fingerprint()}))
	_, ok := p.readCache()
	assert.False(t, ok)
	assert.False(t, p.Status().LoggedIn)
	r, err := apply(t, p)
	assert.NoErr(t, err)
	assert.Eq(t, "Bearer tok-fresh-1", r.Header.Get("Authorization"))
	assert.Eq(t, 1, lines(t, counter))
}

func TestExecFingerprint(t *testing.T) {
	f := newExecFixture(t)
	base := f.spec("token=x")
	base.Exec.Env = map[string]string{"A": "1"}
	base.Exec.Format = "json"
	fp := func(s model.AuthSpec, baseURL string) string {
		return mustNew(t, "dev", s, model.Env{BaseURL: baseURL}, f.deps).fingerprint()
	}
	ref := fp(base, "http://a.test")
	assert.Eq(t, ref, fp(base, "http://a.test"))
	mut := map[string]func(s *model.AuthSpec){
		"command": func(s *model.AuthSpec) {
			s.Exec.Command = append([]string{}, s.Exec.Command...)
			s.Exec.Command[1] = "token=y"
		},
		"env":    func(s *model.AuthSpec) { s.Exec.Env = map[string]string{"A": "2"} },
		"format": func(s *model.AuthSpec) { s.Exec.Format = "text" },
		"apply":  func(s *model.AuthSpec) { s.Apply.Header = "X-Other" },
		"token":  func(s *model.AuthSpec) { s.Exec.Token.Path = "p" },
		"ttl":    func(s *model.AuthSpec) { s.Exec.TTL = "5m" },
	}
	for name, m := range mut {
		t.Run(name, func(t *testing.T) {
			s := base
			m(&s)
			assert.NotEq(t, ref, fp(s, "http://a.test"))
		})
	}
	t.Run("base_url", func(t *testing.T) { assert.NotEq(t, ref, fp(base, "http://b.test")) })
	t.Run("changed fingerprint invalidates cache", func(t *testing.T) {
		s := f.spec("token=tok-fp-1")
		s.Exec.TTL = "1h"
		_, _ = apply(t, f.provider(t, s))
		s.Apply.Header = "X-Other"
		_, ok := f.provider(t, s).readCache()
		assert.False(t, ok)
	})
}

func TestExecLoginFingerprintReplay(t *testing.T) {
	// A phase-1 cache file, fingerprinted with the original algorithm, stays valid.
	ls := newLoginServer(t)
	dir := t.TempDir()
	env := model.Env{BaseURL: ls.URL}
	spec := loginSpec("pw-secret")
	raw, _ := json.Marshal(spec.Login)
	sum := sha256.Sum256([]byte(env.BaseURL + "\n" + string(raw)))
	old := cacheEntry{Token: "phase1-token", ExpiresAt: time.Now().Add(time.Hour), Fingerprint: hex.EncodeToString(sum[:])}
	data, err := json.Marshal(old)
	assert.NoErr(t, err)
	path := filepath.Join(dir, "tokens", "site.dev.main.json")
	assert.NoErr(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assert.NoErr(t, os.WriteFile(path, data, 0o600))
	p := mustNew(t, "dev", spec, env, deps(t, dir))
	r := newReq(t)
	assert.NoErr(t, p.Apply(context.Background(), r, false))
	assert.Eq(t, "Bearer phase1-token", r.Header.Get("Authorization"))
	assert.Eq(t, int32(0), ls.hits.Load())
	si := p.Status()
	assert.True(t, si.LoggedIn)
	assert.True(t, si.Cacheable)
}

func TestExecEnvironment(t *testing.T) {
	f := newExecFixture(t)
	t.Setenv("STUB_PARENT", "from-parent")
	info := filepath.Join(t.TempDir(), "info")
	stdin := filepath.Join(t.TempDir(), "stdin")
	s := f.spec("token=tok-env-1", "info="+info, "stdin="+stdin)
	s.Exec.Env = map[string]string{"STUB_FOO": "bar"}
	_, err := apply(t, f.provider(t, s))
	assert.NoErr(t, err)
	b, err := os.ReadFile(info)
	assert.NoErr(t, err)
	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		k, v, _ := strings.Cut(l, "=")
		got[k] = v
	}
	want, _ := filepath.EvalSymlinks(f.cfgDir)
	have, _ := filepath.EvalSymlinks(got["cwd"])
	assert.Eq(t, want, have)
	assert.Eq(t, "site", got["site"])
	assert.Eq(t, "dev", got["env"])
	assert.Eq(t, "main", got["auth"])
	assert.Eq(t, "bar", got["foo"])
	assert.Eq(t, "from-parent", got["parent"])
	in, err := os.ReadFile(stdin)
	assert.NoErr(t, err)
	assert.Eq(t, "eof", string(in))
}

func TestExecFailures(t *testing.T) {
	f := newExecFixture(t)
	t.Run("non-zero exit shows last stderr line only", func(t *testing.T) {
		p := f.provider(t, f.spec("errline=first line", "errline=\x1b[31mred last\x1b[0m", "exit=3"))
		_, err := apply(t, p)
		assert.Err(t, err)
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Eq(t, "auth main: exec failed (exit 3): red last", err.Error())
		assert.NotContains(t, err.Error(), f.stub)
		assert.NotContains(t, err.Error(), filepath.Base(f.stub))
	})
	t.Run("last line is redacted and capped", func(t *testing.T) {
		p := f.provider(t, f.spec("errline="+strings.Repeat("é", 300), "exit=2"))
		_, err := apply(t, p)
		assert.Err(t, err)
		assert.Eq(t, 200, len([]rune(strings.TrimPrefix(err.Error(), "auth main: exec failed (exit 2): "))))
		f.deps.Secrets.Add("hunter2-secret")
		p = f.provider(t, f.spec("errline=bad hunter2-secret here", "exit=2"))
		_, err = apply(t, p)
		assert.NotContains(t, err.Error(), "hunter2-secret")
		assert.Contains(t, err.Error(), "bad *** here")
	})
	t.Run("command not found", func(t *testing.T) {
		s := f.spec()
		s.Exec.Command = []string{filepath.Join(f.cfgDir, "no-such-cmd")}
		_, err := apply(t, f.provider(t, s))
		assert.Err(t, err)
		assert.Contains(t, err.Error(), "command not found")
		assert.NotContains(t, err.Error(), "no-such-cmd")
		s.Exec.Command = []string{"no-such-cmd-in-path-xyz"}
		_, err = apply(t, f.provider(t, s))
		assert.Err(t, err)
		assert.NotContains(t, err.Error(), "no-such-cmd")
	})
	t.Run("timeout", func(t *testing.T) {
		s := f.spec("sleep=10s")
		s.Exec.Timeout = "300ms"
		start := time.Now()
		_, err := apply(t, f.provider(t, s))
		assert.Err(t, err)
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Contains(t, err.Error(), "exec timed out after 300ms")
		assert.True(t, time.Since(start) < 5*time.Second)
	})
	t.Run("request context deadline", func(t *testing.T) {
		s := f.spec("sleep=10s")
		s.Exec.Timeout = "30s"
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := f.provider(t, s).Apply(ctx, newReq(t), false)
		assert.Err(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.NotContains(t, err.Error(), "30s")
		assert.True(t, time.Since(start) < 5*time.Second)
	})
	t.Run("output over 4 MiB", func(t *testing.T) {
		for _, op := range []string{"big", "bigerr"} {
			start := time.Now()
			_, err := apply(t, f.provider(t, f.spec(op, "sleep=10s")))
			assert.Err(t, err)
			assert.Contains(t, err.Error(), "exceeded 4 MiB")
			assert.True(t, time.Since(start) < 5*time.Second)
		}
	})
}

func TestExecResolveCommand(t *testing.T) {
	f := newExecFixture(t)
	t.Run("relative path resolves against the config dir", func(t *testing.T) {
		binDir := filepath.Join(f.cfgDir, "bin")
		assert.NoErr(t, os.MkdirAll(binDir, 0o755))
		src, err := os.ReadFile(f.stub)
		assert.NoErr(t, err)
		name := filepath.Base(f.stub)
		assert.NoErr(t, os.WriteFile(filepath.Join(binDir, name), src, 0o755))
		s := f.spec("token=tok-rel-1")
		s.Exec.Command[0] = "bin/" + name
		r, err := apply(t, f.provider(t, s))
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer tok-rel-1", r.Header.Get("Authorization"))
		s.Exec.Command[0] = "./bin/" + name
		_, err = apply(t, f.provider(t, s))
		assert.NoErr(t, err)
	})
	t.Run("bare name uses PATH", func(t *testing.T) {
		t.Setenv("PATH", filepath.Dir(f.stub)+string(os.PathListSeparator)+os.Getenv("PATH"))
		s := f.spec("token=tok-path-1")
		s.Exec.Command[0] = strings.TrimSuffix(filepath.Base(f.stub), ".exe")
		_, err := apply(t, f.provider(t, s))
		assert.NoErr(t, err)
	})
	t.Run("bare name never runs from the current directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("PATH=. resolution differs on windows")
		}
		counter := filepath.Join(t.TempDir(), "count")
		t.Chdir(filepath.Dir(f.stub))
		t.Setenv("PATH", ".")
		s := f.spec("token=tok-dot-1", "count="+counter)
		s.Exec.Command[0] = filepath.Base(f.stub)
		_, err := apply(t, f.provider(t, s))
		assert.Err(t, err)
		assert.Eq(t, 0, lines(t, counter))
		assert.NotContains(t, err.Error(), filepath.Base(f.stub))
	})
}

func TestCheckBatchArgs(t *testing.T) {
	t.Run("extension", func(t *testing.T) {
		for path, reject := range map[string]bool{
			"a.bat": true, "a.BAT": true, "a.cmd": true, "a.CmD": true, `C:\x\y.Cmd`: true,
			"a.exe": false, "a": false, "a.bat.exe": false, "a.batx": false, "a.ps1": false,
		} {
			err := checkBatchArgs(path, []string{"x&y"})
			assert.Eq(t, reject, err != nil, path)
		}
	})
	t.Run("metacharacters", func(t *testing.T) {
		for _, c := range []string{"&", "|", "<", ">", "^", "%", "!", `"`, "\n", "\r"} {
			err := checkBatchArgs("a.cmd", []string{"ok", "a" + c + "b"})
			assert.Err(t, err, c)
			assert.Contains(t, err.Error(), "argument 2")
			assert.NotContains(t, err.Error(), "a"+c+"b")
		}
	})
	t.Run("safe arguments", func(t *testing.T) {
		assert.NoErr(t, checkBatchArgs("a.cmd", []string{"token", "--audience", "pet", "a-b_c.d:e/f"}))
		assert.NoErr(t, checkBatchArgs("a.cmd", nil))
	})
}

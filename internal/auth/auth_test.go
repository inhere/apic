package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inhere/apic/internal/model"
)

func deps(t *testing.T, dir string) Deps {
	s := &model.Secrets{}
	return Deps{
		Client:   &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		Secrets:  s,
		Redactor: &model.Redactor{Secrets: s},
		CacheDir: dir,
	}
}

func newReq(t *testing.T) *http.Request {
	r, err := http.NewRequest("GET", "http://example.test/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustNew(t *testing.T, env string, spec model.AuthSpec, e model.Env, d Deps) *Provider {
	p, err := New("site", env, "main", spec, e, d)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func code(err error) int {
	var ce *model.CodedError
	if errors.As(err, &ce) {
		return ce.C
	}
	return -1
}

func TestAuthStatic(t *testing.T) {
	ctx := context.Background()
	d := deps(t, t.TempDir())
	cases := []struct {
		name string
		spec model.AuthSpec
		chk  func(r *http.Request) bool
	}{
		{"none", model.AuthSpec{Type: "none"}, func(r *http.Request) bool { return r.Header.Get("Authorization") == "" }},
		{"bearer", model.AuthSpec{Type: "bearer", Token: "tok-123"}, func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer tok-123" }},
		{"basic", model.AuthSpec{Type: "basic", Username: "u", Password: "pass1234"}, func(r *http.Request) bool {
			u, pw, ok := r.BasicAuth()
			return ok && u == "u" && pw == "pass1234"
		}},
		{"apikey-header", model.AuthSpec{Type: "apikey", In: "header", Name: "X-Key", Value: "kvalue1"}, func(r *http.Request) bool { return r.Header.Get("X-Key") == "kvalue1" }},
		{"apikey-query", model.AuthSpec{Type: "apikey", In: "query", Name: "key", Token: "ktoken1"}, func(r *http.Request) bool { return r.URL.Query().Get("key") == "ktoken1" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mustNew(t, "dev", c.spec, model.Env{}, d)
			r := newReq(t)
			if err := p.Apply(ctx, r, false); err != nil {
				t.Fatal(err)
			}
			if !c.chk(r) {
				t.Fatalf("unexpected request: %v %v", r.Header, r.URL)
			}
			if err := p.Refresh(ctx); !errors.Is(err, model.ErrNoRefresh) {
				t.Fatalf("refresh = %v", err)
			}
		})
	}
	if got := d.Redactor.Redact("tok-123 pass1234 kvalue1 ktoken1"); strings.Contains(got, "tok-123") || strings.Contains(got, "pass1234") || strings.Contains(got, "kvalue1") || strings.Contains(got, "ktoken1") {
		t.Fatalf("secrets not registered: %s", got)
	}
}

func TestAuthUnknownType(t *testing.T) {
	_, err := New("s", "e", "a", model.AuthSpec{Type: "weird"}, model.Env{}, Deps{})
	if code(err) != model.ExitLocal {
		t.Fatalf("err = %v", err)
	}
}

type loginServer struct {
	*httptest.Server
	hits   atomic.Int32
	status atomic.Int32
}

func newLoginServer(t *testing.T) *loginServer {
	ls := &loginServer{}
	ls.status.Store(200)
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := ls.hits.Add(1)
		if r.URL.Path != "/login" || r.Method != "POST" {
			http.Error(w, "bad", 404)
			return
		}
		if st := int(ls.status.Load()); st != 200 {
			http.Error(w, "SECRET-BODY-ECHO", st)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("X-Tok", "hdr-token")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"token": "tok-" + string(rune('A'+n)) + "-secret", "ttl": "3600"},
		})
	}))
	t.Cleanup(ls.Close)
	return ls
}

func loginSpec(pw string) model.AuthSpec {
	return model.AuthSpec{Type: "login", Login: model.LoginSpec{
		Path:    "/login",
		Body:    map[string]any{"username": "admin", "password": pw},
		Token:   model.ValueFrom{From: "body", Path: "data.token"},
		Expires: model.ValueFrom{Path: "data.ttl"},
		Apply:   model.ApplySpec{Header: "Authorization"},
	}}
}

func TestAuthLoginCacheReuse(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	dir := t.TempDir()
	env := model.Env{BaseURL: ls.URL}
	p1 := mustNew(t, "dev", loginSpec("pw-secret"), env, deps(t, dir))
	r := newReq(t)
	if err := p1.Apply(ctx, r, false); err != nil {
		t.Fatal(err)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer tok-B-secret" {
		t.Fatalf("auth = %q", got)
	}
	p2 := mustNew(t, "dev", loginSpec("pw-secret"), env, deps(t, dir))
	r2 := newReq(t)
	if err := p2.Apply(ctx, r2, false); err != nil {
		t.Fatal(err)
	}
	if r2.Header.Get("Authorization") != r.Header.Get("Authorization") || ls.hits.Load() != 1 {
		t.Fatalf("hits=%d auth=%q", ls.hits.Load(), r2.Header.Get("Authorization"))
	}
	if ok, exp := p2.Status(); !ok || exp.IsZero() {
		t.Fatalf("status = %v %v", ok, exp)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(dir, "tokens", "site.dev.main.json"))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("perm = %v", st.Mode().Perm())
		}
	}
	if err := p2.Logout(); err != nil {
		t.Fatal(err)
	}
	if err := p2.Logout(); err != nil {
		t.Fatalf("second logout: %v", err)
	}
	if ok, _ := p2.Status(); ok {
		t.Fatal("still logged in")
	}
}

func TestAuthLoginExpiry(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	now := time.Unix(1_000_000, 0)
	d := deps(t, t.TempDir())
	d.Now = func() time.Time { return now }
	p := mustNew(t, "dev", loginSpec("pw-secret"), model.Env{BaseURL: ls.URL}, d)
	r := newReq(t)
	_ = p.Apply(ctx, r, false)
	now = now.Add(3500 * time.Second) // within 30s skew of 3600
	_ = p.Apply(ctx, newReq(t), false)
	if ls.hits.Load() != 1 {
		t.Fatalf("hits = %d", ls.hits.Load())
	}
	now = now.Add(100 * time.Second)
	r2 := newReq(t)
	if err := p.Apply(ctx, r2, false); err != nil {
		t.Fatal(err)
	}
	if ls.hits.Load() != 2 || r2.Header.Get("Authorization") == r.Header.Get("Authorization") {
		t.Fatalf("hits=%d", ls.hits.Load())
	}
}

func TestAuthLoginEnvIsolationAndFingerprint(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	dir := t.TempDir()
	env := model.Env{BaseURL: ls.URL}
	_ = mustNew(t, "dev", loginSpec("pw-secret"), env, deps(t, dir)).Apply(ctx, newReq(t), false)
	_ = mustNew(t, "prod", loginSpec("pw-secret"), env, deps(t, dir)).Apply(ctx, newReq(t), false)
	if ls.hits.Load() != 2 {
		t.Fatalf("env isolation: hits = %d", ls.hits.Load())
	}
	_ = mustNew(t, "dev", loginSpec("other-pw"), env, deps(t, dir)).Apply(ctx, newReq(t), false)
	if ls.hits.Load() != 3 {
		t.Fatalf("fingerprint: hits = %d", ls.hits.Load())
	}
}

func TestAuthLoginRefresh(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	p := mustNew(t, "dev", loginSpec("pw-secret"), model.Env{BaseURL: ls.URL}, deps(t, t.TempDir()))
	_ = p.Apply(ctx, newReq(t), false)
	if err := p.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	r := newReq(t)
	_ = p.Apply(ctx, r, false)
	if ls.hits.Load() != 2 || r.Header.Get("Authorization") != "Bearer tok-C-secret" {
		t.Fatalf("hits=%d auth=%q", ls.hits.Load(), r.Header.Get("Authorization"))
	}
}

func TestAuthLoginErrors(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	for st, want := range map[int]int{401: model.ExitHTTP4xx, 503: model.ExitHTTP5xx} {
		ls.status.Store(int32(st))
		p := mustNew(t, "dev", loginSpec("pw-secret"), model.Env{BaseURL: ls.URL}, deps(t, t.TempDir()))
		err := p.Apply(ctx, newReq(t), false)
		if code(err) != want || strings.Contains(err.Error(), "SECRET-BODY-ECHO") {
			t.Fatalf("status %d: err = %v", st, err)
		}
	}
	// network error
	url := ls.URL
	ls.Close()
	p := mustNew(t, "dev", loginSpec("pw-secret"), model.Env{BaseURL: url}, deps(t, t.TempDir()))
	if err := p.Apply(ctx, newReq(t), false); code(err) != model.ExitNetwork {
		t.Fatalf("network err = %v", err)
	}
	// token missing
	ls2 := newLoginServer(t)
	spec := loginSpec("pw-secret")
	spec.Login.Token.Path = "nope"
	p = mustNew(t, "dev", spec, model.Env{BaseURL: ls2.URL}, deps(t, t.TempDir()))
	if err := p.Apply(ctx, newReq(t), false); code(err) != model.ExitLocal {
		t.Fatalf("missing token err = %v", err)
	}
}

func TestAuthLoginDryRun(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	dir := t.TempDir()
	env := model.Env{BaseURL: ls.URL}
	p := mustNew(t, "dev", loginSpec("pw-secret"), env, deps(t, dir))
	r := newReq(t)
	if err := p.Apply(ctx, r, true); err != nil {
		t.Fatal(err)
	}
	if ls.hits.Load() != 0 || r.Header.Get("Authorization") != "Bearer <login-token>" {
		t.Fatalf("hits=%d auth=%q", ls.hits.Load(), r.Header.Get("Authorization"))
	}
	// with a cached token, dry-run uses it
	_ = p.Apply(ctx, newReq(t), false)
	r2 := newReq(t)
	_ = mustNew(t, "dev", loginSpec("pw-secret"), env, deps(t, dir)).Apply(ctx, r2, true)
	if ls.hits.Load() != 1 || r2.Header.Get("Authorization") != "Bearer tok-B-secret" {
		t.Fatalf("hits=%d auth=%q", ls.hits.Load(), r2.Header.Get("Authorization"))
	}
}

func TestAuthLoginHeaderTokenQueryApply(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	spec := loginSpec("pw-secret")
	spec.Login.Token = model.ValueFrom{From: "header", Path: "X-Tok"}
	spec.Login.Expires = model.ValueFrom{}
	spec.Login.Apply = model.ApplySpec{Query: "access_token", Value: "{token}"}
	p := mustNew(t, "dev", spec, model.Env{BaseURL: ls.URL}, deps(t, t.TempDir()))
	r := newReq(t)
	if err := p.Apply(ctx, r, false); err != nil {
		t.Fatal(err)
	}
	if r.URL.Query().Get("access_token") != "hdr-token" {
		t.Fatalf("url = %v", r.URL)
	}
	if ok, exp := p.Status(); !ok || !exp.IsZero() {
		t.Fatalf("status = %v %v", ok, exp)
	}
}

func TestAuthLoginVerboseRedacted(t *testing.T) {
	ctx := context.Background()
	ls := newLoginServer(t)
	var buf bytes.Buffer
	d := deps(t, t.TempDir())
	d.Verbose = &buf
	spec := loginSpec("pw-secret")
	spec.Login.Path = "/login?k=pw-secret"
	p := mustNew(t, "dev", spec, model.Env{BaseURL: ls.URL}, d)
	// server only matches path /login; query is ignored by r.URL.Path
	if err := p.Apply(ctx, newReq(t), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "> login POST") || strings.Contains(out, "pw-secret") || strings.Contains(out, "tok-") {
		t.Fatalf("verbose = %q", out)
	}
}

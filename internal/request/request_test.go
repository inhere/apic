package request

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func code(err error) int {
	var ce *model.CodedError
	if errors.As(err, &ce) {
		return ce.C
	}
	return -1
}

type fakeAuth struct {
	secrets  *model.Secrets
	query    bool
	refresh  error
	applies  int
	refreshN int
	token    string
	invalN   int
	invalErr error
}

func (f *fakeAuth) Apply(_ context.Context, req *http.Request, _ bool) error {
	f.applies++
	f.secrets.Add(f.token)
	if f.query {
		q := req.URL.Query()
		q.Set("apikey", f.token)
		req.URL.RawQuery = q.Encode()
	} else {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	return nil
}

func (f *fakeAuth) Refresh(context.Context) error { f.refreshN++; return f.refresh }

func (f *fakeAuth) Invalidate() error { f.invalN++; return f.invalErr }

func rd(s ...string) *model.Redactor {
	sec := &model.Secrets{}
	for _, v := range s {
		sec.Add(v)
	}
	return &model.Redactor{Secrets: sec}
}

func TestBuildRawPathRejected(t *testing.T) {
	for _, p := range []string{"//evil", "http://x", "x", ""} {
		_, err := Build(nil, Input{Method: "GET", Path: p}, model.Env{BaseURL: "http://h"}, "")
		assert.Eq(t, model.ExitUsage, code(err), p)
	}
	pr, err := Build(nil, Input{Path: "/a", P: []string{"x=1"}}, model.Env{BaseURL: "http://h/"}, "/ignored")
	assert.NoErr(t, err)
	assert.Eq(t, "http://h/a?x=1", pr.URL)
	assert.Eq(t, "GET", pr.Method)
}

func TestBuildBasePathOverride(t *testing.T) {
	op := &model.Operation{Method: "get", Path: "/users/{id}", Params: []model.Param{{Name: "id", In: "path", Required: true}}}
	// caller picks env.BasePath over index default; Build uses what it is given
	pr, err := Build(op, Input{Method: "get", Params: map[string]string{"id": "a b/1"}}, model.Env{BaseURL: "http://h", BasePath: "/v2"}, "/v2")
	assert.NoErr(t, err)
	assert.Eq(t, "http://h/v2/users/a%20b%2F1", pr.URL)
}

func TestBuildQueryAndPathSameName(t *testing.T) {
	op := &model.Operation{Path: "/x/{id}", Params: []model.Param{
		{Name: "id", In: "path", Required: true}, {Name: "id", In: "query"}}}
	pr, err := Build(op, Input{P: []string{"path.id=7", "query.id=1"}}, model.Env{BaseURL: "http://h"}, "")
	assert.NoErr(t, err)
	assert.Eq(t, "http://h/x/7?id=1", pr.URL)
	// ambiguous plain name
	_, err = Build(op, Input{P: []string{"id=1"}}, model.Env{BaseURL: "http://h"}, "")
	assert.Eq(t, model.ExitUsage, code(err))
}

func TestBuildValidation(t *testing.T) {
	op := &model.Operation{Path: "/x", Params: []model.Param{
		{Name: "n", In: "query", Type: "integer"},
		{Name: "e", In: "query", Enum: []string{"a", "b"}},
		{Name: "r", In: "header", Required: true},
	}}
	env := model.Env{BaseURL: "http://h"}
	_, err := Build(op, Input{Params: map[string]string{"n": "1"}}, env, "")
	assert.Eq(t, model.ExitLocal, code(err))
	_, err = Build(op, Input{P: []string{"r=1", "n=x"}}, env, "")
	assert.Eq(t, model.ExitLocal, code(err))
	_, err = Build(op, Input{P: []string{"r=1", "e=c"}}, env, "")
	assert.Eq(t, model.ExitLocal, code(err))
	pr, err := Build(op, Input{P: []string{"r=1", "e=a"}, Params: map[string]string{"n": "5"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, "1", pr.Header.Get("r"))
}

func TestBuildBody(t *testing.T) {
	env := model.Env{BaseURL: "http://h"}
	pr, err := Build(nil, Input{Method: "post", Path: "/a", Fields: []string{"user.name=tom", "age:=3", "tags:=[\"a\"]"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, `{"age":3,"tags":["a"],"user":{"name":"tom"}}`, string(pr.Body))
	assert.Eq(t, "application/json", pr.Header.Get("Content-Type"))

	_, err = Build(nil, Input{Path: "/a", Fields: []string{"a:=notjson"}}, env, "")
	assert.Eq(t, model.ExitLocal, code(err))
	_, err = Build(nil, Input{Path: "/a", Data: "{}", Fields: []string{"a=1"}}, env, "")
	assert.Eq(t, model.ExitUsage, code(err))

	form := &model.Operation{Path: "/f", Body: &model.BodySpec{Type: "form"}}
	pr, err = Build(form, Input{Fields: []string{"a=1 2", "b=x"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, "a=1+2&b=x", string(pr.Body))
	assert.Eq(t, "application/x-www-form-urlencoded", pr.Header.Get("Content-Type"))

	xml := &model.Operation{Path: "/x", Body: &model.BodySpec{Type: "xml"}}
	_, err = Build(xml, Input{Fields: []string{"a=1"}}, env, "")
	assert.Eq(t, model.ExitUsage, code(err))

	// -H overrides auto Content-Type; stdin and @file
	pr, err = Build(nil, Input{Path: "/a", Data: "-", Stdin: strings.NewReader("hi"), Headers: []string{"Content-Type: text/plain"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, "hi", string(pr.Body))
	assert.Eq(t, "text/plain", pr.Header.Get("Content-Type"))
	f := filepath.Join(t.TempDir(), "b.json")
	assert.NoErr(t, os.WriteFile(f, []byte(`{"k":1}`), 0o600))
	pr, err = Build(nil, Input{Path: "/a", Data: "@" + f}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, `{"k":1}`, string(pr.Body))
}

func TestBuildBodyTypedFields(t *testing.T) {
	env := model.Env{BaseURL: "http://h"}
	op := &model.Operation{Method: "POST", Path: "/t", Body: &model.BodySpec{Type: "json", Schema: model.SchemaSummary{Fields: []model.Field{
		{Name: "mock", Type: "boolean"}, {Name: "n", Type: "integer"}, {Name: "rate", Type: "number"}, {Name: "id", Type: "string"}, {Name: "tags", Type: "array"},
	}}}}
	pr, err := Build(op, Input{Method: "POST", Fields: []string{"mock=true", "n=3", "rate=1.5", "id=7", "tags=a", "other=false", "x.mock=true"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, `{"id":"7","mock":true,"n":3,"other":"false","rate":1.5,"tags":"a","x":{"mock":"true"}}`, string(pr.Body))

	// k:=json is kept as written, even against the declared type.
	pr, err = Build(op, Input{Method: "POST", Fields: []string{"mock:=\"yes\"", "n:=null"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, `{"mock":"yes","n":null}`, string(pr.Body))

	for _, f := range []string{"mock=yes", "mock=1", "n=1.5", "n=+5", "n=007", "rate=Inf", "rate=0x10", `rate="1"`} {
		_, err = Build(op, Input{Method: "POST", Fields: []string{f}}, env, "")
		assert.Eq(t, model.ExitLocal, code(err), f)
		assert.Contains(t, err.Error(), ":=<json>", f)
	}

	// form bodies stay text.
	form := &model.Operation{Path: "/f", Body: &model.BodySpec{Type: "form", Schema: op.Body.Schema}}
	pr, err = Build(form, Input{Fields: []string{"mock=yes"}}, env, "")
	assert.NoErr(t, err)
	assert.Eq(t, "mock=yes", string(pr.Body))
}

func call(t *testing.T, srv *httptest.Server, pr *Prepared, a Authorizer, r *model.Redactor) Call {
	t.Helper()
	return Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL}, Authorizer: a, Redactor: r, Timeout: 5 * time.Second}
}

func prep(t *testing.T, base, method, path string, in Input) *Prepared {
	t.Helper()
	in.Method, in.Path = method, path
	pr, err := Build(nil, in, model.Env{BaseURL: base}, "")
	assert.NoErr(t, err)
	return pr
}

func TestExecuteRedirectNotFollowed(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Redirect(w, r, "http://127.0.0.1:1/other", http.StatusFound)
	}))
	defer srv.Close()
	resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), nil, nil))
	assert.NoErr(t, err)
	assert.Eq(t, 302, resp.Status)
	assert.Contains(t, resp.Header.Get("Location"), "/other")
	assert.Eq(t, int32(1), atomic.LoadInt32(&hits))
}

func TestExecute401RetryOnceReplaysStdinBody(t *testing.T) {
	var n int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(r.Body)
		bodies = append(bodies, b.String())
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	sec := &model.Secrets{}
	fa := &fakeAuth{secrets: sec, token: "tok-12345"}
	pr := prep(t, srv.URL, "POST", "/a", Input{Data: "-", Stdin: strings.NewReader("payload")})
	resp, err := Execute(context.Background(), call(t, srv, pr, fa, &model.Redactor{Secrets: sec}))
	assert.NoErr(t, err)
	assert.Eq(t, 401, resp.Status)
	assert.Eq(t, int32(2), atomic.LoadInt32(&n))
	assert.Eq(t, []string{"payload", "payload"}, bodies)
	assert.Eq(t, 1, fa.refreshN)
	assert.Eq(t, 2, fa.applies)
}

func TestExecute401NoRefreshReturnsOriginal(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	sec := &model.Secrets{}
	fa := &fakeAuth{secrets: sec, token: "tok-12345", refresh: model.ErrNoRefresh}
	resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, nil))
	assert.NoErr(t, err)
	assert.Eq(t, 401, resp.Status)
	assert.Eq(t, "nope", string(resp.Body))
	assert.Eq(t, int32(1), atomic.LoadInt32(&n))
}

func TestExecuteNetworkErrorRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // connection refused
	sec := &model.Secrets{}
	fa := &fakeAuth{secrets: sec, token: "SECRETKEY99", query: true}
	c := Call{Prepared: prep(t, base, "GET", "/a", Input{}), Env: model.Env{BaseURL: base}, Authorizer: fa,
		Redactor: &model.Redactor{Secrets: sec}, Timeout: 2 * time.Second}
	_, err := Execute(context.Background(), c)
	assert.Eq(t, model.ExitNetwork, code(err))
	assert.NotContains(t, err.Error(), "SECRETKEY99")
	assert.Contains(t, err.Error(), "***")
}

func TestExecuteTimeoutIs3(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	c := call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), nil, nil)
	c.Timeout = 100 * time.Millisecond
	_, err := Execute(context.Background(), c)
	assert.Eq(t, model.ExitNetwork, code(err))
}

func TestCurlNoPlaintext(t *testing.T) {
	sec := &model.Secrets{}
	fa := &fakeAuth{secrets: sec, token: "SECRETKEY99"}
	pr := prep(t, "http://h", "POST", "/a", Input{Data: `{"it's":"SECRETKEY99"}`, P: []string{"k=SECRETKEY99"}})
	c := Call{Prepared: pr, Env: model.Env{BaseURL: "http://h", Insecure: true}, Authorizer: fa, Redactor: &model.Redactor{Secrets: sec}, DryRun: true}
	resp, err := Execute(context.Background(), c)
	assert.NoErr(t, err)
	assert.NotContains(t, resp.Curl, "SECRETKEY99")
	assert.NotContains(t, resp.Curl, "\n")
	assert.Contains(t, resp.Curl, "-k")
	assert.Contains(t, resp.Curl, `it'\''s`)
	assert.Contains(t, resp.Curl, "Authorization: ***")
}

func TestProtectLowercaseMethod(t *testing.T) {
	t.Setenv("APIC_NO_PROMPT", "")
	env := model.Env{Protect: true}
	assert.Eq(t, model.ExitProtected, code(CheckProtect("post", env, model.AuthSpec{}, false, false, nil)))
	assert.NoErr(t, CheckProtect("post", env, model.AuthSpec{}, true, false, nil))
	assert.NoErr(t, CheckProtect("get", env, model.AuthSpec{}, false, false, nil))
	// interactive
	yes := func(string) bool { return true }
	assert.NoErr(t, CheckProtect("post", env, model.AuthSpec{}, false, true, yes))
	assert.Eq(t, model.ExitProtected, code(CheckProtect("post", env, model.AuthSpec{}, false, true, nil)))
	t.Setenv("APIC_NO_PROMPT", "1")
	assert.Eq(t, model.ExitProtected, code(CheckProtect("post", env, model.AuthSpec{}, false, true, yes)))
}

func TestAllowMethodsRejectExit6(t *testing.T) {
	a := model.AuthSpec{AllowMethods: []string{"get"}}
	assert.Eq(t, 6, code(CheckProtect("DELETE", model.Env{}, a, true, false, nil)))
	assert.NoErr(t, CheckProtect("GET", model.Env{}, a, false, false, nil))
}

func TestDryRunSkipsProtectAndSendsNothing(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&n, 1) }))
	defer srv.Close()
	c := call(t, srv, prep(t, srv.URL, "DELETE", "/a", Input{}), nil, nil)
	c.Env.Protect = true
	c.DryRun = true
	resp, err := Execute(context.Background(), c)
	assert.NoErr(t, err)
	assert.Contains(t, resp.Curl, "-X DELETE")
	assert.Eq(t, int32(0), atomic.LoadInt32(&n))
	// non-dry is rejected before Apply
	c.DryRun = false
	fa := &fakeAuth{secrets: &model.Secrets{}, token: "tok-12345"}
	c.Authorizer = fa
	_, err = Execute(context.Background(), c)
	assert.Eq(t, model.ExitProtected, code(err))
	assert.Eq(t, 0, fa.applies)
	assert.Eq(t, int32(0), atomic.LoadInt32(&n))
}

func TestInsecureWarnsAndCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	pr := prep(t, srv.URL, "GET", "/a", Input{})

	// default: untrusted cert fails with network error
	c := Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL}, Timeout: 5 * time.Second}
	_, err := Execute(context.Background(), c)
	assert.Eq(t, model.ExitNetwork, code(err))

	// insecure: works and warns on stderr
	var stderr bytes.Buffer
	c.Env.Insecure = true
	c.Stderr = &stderr
	resp, err := Execute(context.Background(), c)
	assert.NoErr(t, err)
	assert.Eq(t, "ok", string(resp.Body))
	assert.Contains(t, stderr.String(), "warning")

	// ca_file trusts the self-signed cert
	pemPath := filepath.Join(t.TempDir(), "ca.pem")
	cert := srv.Certificate()
	pemBytes := pemEncode(cert.Raw)
	assert.NoErr(t, os.WriteFile(pemPath, pemBytes, 0o600))
	c2 := Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL, CAFile: pemPath}, Timeout: 5 * time.Second}
	resp, err = Execute(context.Background(), c2)
	assert.NoErr(t, err)
	assert.Eq(t, 200, resp.Status)

	_, err = NewClient(model.Env{CAFile: filepath.Join(t.TempDir(), "missing")})
	assert.Eq(t, model.ExitLocal, code(err))
}

func TestExecute401Invalidate(t *testing.T) {
	newSrv := func(codes ...int) (*httptest.Server, *int32) {
		var n int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i := int(atomic.AddInt32(&n, 1)) - 1
			if i >= len(codes) {
				i = len(codes) - 1
			}
			w.WriteHeader(codes[i])
		}))
		t.Cleanup(srv.Close)
		return srv, &n
	}

	t.Run("retry still 401 invalidates once", func(t *testing.T) {
		srv, n := newSrv(401)
		fa := &fakeAuth{secrets: &model.Secrets{}, token: "tok-12345"}
		resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, nil))
		assert.NoErr(t, err)
		assert.Eq(t, 401, resp.Status)
		assert.Eq(t, int32(2), atomic.LoadInt32(n))
		assert.Eq(t, 1, fa.refreshN)
		assert.Eq(t, 1, fa.invalN)
	})

	t.Run("retry succeeds does not invalidate", func(t *testing.T) {
		srv, _ := newSrv(401, 200)
		fa := &fakeAuth{secrets: &model.Secrets{}, token: "tok-12345"}
		resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, nil))
		assert.NoErr(t, err)
		assert.Eq(t, 200, resp.Status)
		assert.Eq(t, 0, fa.invalN)
	})

	t.Run("static auth without refresh does not invalidate", func(t *testing.T) {
		srv, _ := newSrv(401)
		fa := &fakeAuth{secrets: &model.Secrets{}, token: "tok-12345", refresh: model.ErrNoRefresh}
		resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, nil))
		assert.NoErr(t, err)
		assert.Eq(t, 401, resp.Status)
		assert.Eq(t, 0, fa.invalN)
	})

	t.Run("invalidate error warns redacted and keeps response", func(t *testing.T) {
		srv, _ := newSrv(401)
		sec := &model.Secrets{}
		fa := &fakeAuth{secrets: sec, token: "tok-12345", invalErr: errors.New("remove cache for tok-12345 failed")}
		c := call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, &model.Redactor{Secrets: sec})
		var stderr bytes.Buffer
		c.Stderr = &stderr
		resp, err := Execute(context.Background(), c)
		assert.NoErr(t, err)
		assert.Eq(t, 401, resp.Status)
		assert.Eq(t, "warning: drop cached token: remove cache for *** failed\n", stderr.String())
	})

	t.Run("invalidate error without stderr is silent", func(t *testing.T) {
		srv, _ := newSrv(401)
		fa := &fakeAuth{secrets: &model.Secrets{}, token: "tok-12345", invalErr: errors.New("boom")}
		resp, err := Execute(context.Background(), call(t, srv, prep(t, srv.URL, "GET", "/a", Input{}), fa, nil))
		assert.NoErr(t, err)
		assert.Eq(t, 401, resp.Status)
	})
}

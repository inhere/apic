package spec_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
)

func TestOpenAPIEquivalent(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	var indexes []*model.Index
	for _, file := range []string{"min-v2.json", "min-v3.json"} {
		t.Run(file, func(t *testing.T) {
			idx, err := (spec.OpenAPILoader{}).Load(context.Background(), model.Source{File: file}, "testdata")
			assert.Require(t, assert.NoError(t, err))
			assert.Eq(t, 4, len(idx.Ops))
			assert.Eq(t, "/api/v1", idx.DefaultBasePath)
			assert.Eq(t, "http://demo.local", idx.DefaultBaseURL)
			indexes = append(indexes, idx)
		})
	}
	assert.True(t, reflect.DeepEqual(indexes[0], indexes[1]))
	for _, op := range indexes[0].Ops {
		switch op.Name {
		case "get-pet":
			assert.Eq(t, "GET", op.Method)
			assert.Eq(t, []model.Param{{Name: "id", In: "path", Required: true, Type: "integer"}, {Name: "verbose", In: "query", Type: "string", Enum: []string{"yes", "no"}}}, op.Params)
			assert.Eq(t, []model.Field{{Name: "name", Type: "string", Required: true}}, op.Responses["200"].Fields)
		case "create-pet":
			assert.Eq(t, "json", op.Body.Type)
			assert.Eq(t, []model.Field{{Name: "age", Type: "integer"}, {Name: "name", Type: "string", Required: true}}, op.Body.Schema.Fields)
		case "submit-pet":
			assert.Eq(t, "form", op.Body.Type)
		case "pets-search":
			assert.Eq(t, "Search pets", op.Summary)
		}
	}
}

func TestOpenAPIYAML(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name string
		doc  string
		base string
	}{
		{"v2", "swagger: '2.0'\ninfo: {title: Demo, version: '1'}\nhost: demo.local\nbasePath: /\npaths:\n  /pets:\n    get:\n      responses:\n        '200': {description: OK}\n", ""},
		{"v3", "openapi: 3.0.3\ninfo: {title: Demo, version: '1'}\nservers: [{url: 'http://demo.local/'}]\npaths:\n  /pets:\n    get:\n      responses:\n        '200': {description: OK}\n", "/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "spec.yaml")
			assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte(tc.doc), 0600)))
			idx, err := (spec.OpenAPILoader{}).Load(context.Background(), model.Source{File: file}, dir)
			assert.Require(t, assert.NoError(t, err))
			assert.Eq(t, "http://demo.local", idx.DefaultBaseURL)
			assert.Eq(t, tc.base, idx.DefaultBasePath)
			assert.Require(t, assert.Eq(t, 1, len(idx.Ops)))
			assert.Eq(t, "GET", idx.Ops[0].Method)
		})
	}
}

func TestOpenAPIBodyMedia(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ media, want string }{
		{"application/json", "json"},
		{"application/x-www-form-urlencoded", "form"},
		{"multipart/form-data", "form"},
		{"application/xml", "xml"},
		{"application/octet-stream", "raw"},
		{"application/problem+json", "raw"},
	} {
		t.Run(tc.media, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "spec.json")
			doc := fmt.Sprintf(`{"openapi":"3.0.3","info":{"title":"Demo","version":"1"},"paths":{"/pet":{"post":{"requestBody":{"content":{%q:{"schema":{"type":"object"}}}},"responses":{"200":{"description":"OK"}}}}}}`, tc.media)
			assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte(doc), 0600)))
			idx, err := (spec.OpenAPILoader{}).Load(context.Background(), model.Source{File: file}, dir)
			assert.Require(t, assert.NoError(t, err))
			assert.Require(t, assert.Eq(t, 1, len(idx.Ops)))
			assert.Require(t, assert.True(t, idx.Ops[0].Body != nil))
			assert.Eq(t, tc.want, idx.Ops[0].Body.Type)
		})
	}
}

func TestOpenAPIURLScheme(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	raw, err := os.ReadFile(filepath.Join("testdata", "min-v2.json"))
	assert.Require(t, assert.NoError(t, err))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(raw) }))
	defer server.Close()
	// URL scheme is used when Swagger 2.0 omits schemes, including on TLS fetches.
	idx, _, err := spec.Index(context.Background(), "demo", model.Source{URL: server.URL}, t.TempDir(), spec.IndexOptions{Client: server.Client()})
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, "https://demo.local", idx.DefaultBaseURL)
}

func TestFetchETag(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Eq(t, "Bearer demo", r.Header.Get("Authorization"))
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("demo"))
	}))
	defer server.Close()
	src := model.Source{URL: server.URL + "/openapi?secret=demo"}
	for _, tc := range []struct {
		name, etag string
		modified   bool
	}{{"first", "", false}, {"conditional", `"v1"`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, etag, notModified, err := spec.Fetch(context.Background(), src, t.TempDir(), spec.FetchOptions{ETag: tc.etag, Apply: func(r *http.Request) error { r.Header.Set("Authorization", "Bearer demo"); return nil }})
			assert.Require(t, assert.NoError(t, err))
			assert.Eq(t, tc.modified, notModified)
			if !tc.modified {
				assert.Eq(t, "demo", string(raw))
				assert.Eq(t, `"v1"`, etag)
			}
		})
	}
}

func TestFetchErrors(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer server.Close()
	_, _, _, err := spec.Fetch(context.Background(), model.Source{URL: server.URL + "/spec?secret=demo"}, t.TempDir(), spec.FetchOptions{})
	assertCode(t, err, model.ExitLocal)
	assert.True(t, !strings.Contains(err.Error(), "secret"))
	client := &http.Client{Transport: roundTripFail{}}
	_, _, _, err = spec.Fetch(context.Background(), model.Source{URL: "http://demo.local/spec?secret=demo"}, t.TempDir(), spec.FetchOptions{Client: client})
	assertCode(t, err, model.ExitNetwork)
	assert.True(t, !strings.Contains(err.Error(), "secret"))
}

type roundTripFail struct{}

func (roundTripFail) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("network unavailable " + r.URL.String())
}

func TestFetchRedirectAndApplyError(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	src := model.Source{URL: server.URL + "/spec?secret=demo"}
	_, _, _, err := spec.Fetch(context.Background(), src, t.TempDir(), spec.FetchOptions{Apply: func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer demo")
		return nil
	}})
	assertCode(t, err, model.ExitLocal)
	assert.True(t, !reached.Load())
	_, _, _, err = spec.Fetch(context.Background(), src, t.TempDir(), spec.FetchOptions{Apply: func(r *http.Request) error {
		return errors.New(r.URL.String())
	}})
	assertCode(t, err, model.ExitLocal)
	assert.True(t, !strings.Contains(err.Error(), "secret"))
}

func TestCacheRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	raw, err := os.ReadFile(filepath.Join("testdata", "min-v3.json"))
	assert.Require(t, assert.NoError(t, err))
	var failing atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	src := model.Source{URL: server.URL, TTL: time.Millisecond}
	first, warnings, err := spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{})
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, 0, len(warnings))
	for _, suffix := range []string{".raw", ".index.json", ".meta.json"} {
		_, err := os.Stat(filepath.Join(dir, "cache", "specs", "demo"+suffix))
		assert.Require(t, assert.NoError(t, err))
	}
	src.TTL = time.Hour
	cached, _, err := spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{})
	assert.Require(t, assert.NoError(t, err))
	assert.True(t, reflect.DeepEqual(first, cached))
	assert.Eq(t, int32(1), calls.Load())
	_, warnings, err = spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{Refresh: true})
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, 0, len(warnings))
	assert.Eq(t, int32(2), calls.Load())
	failing.Store(true)
	// Expire the cache deterministically: on Windows the clock is coarse, so a
	// 1ns TTL alone may still look fresh right after the previous fetch.
	metaFile := filepath.Join(dir, "cache", "specs", "demo.meta.json")
	assert.Require(t, assert.NoError(t, os.WriteFile(metaFile, []byte(`{"etag":"\"v1\"","fetched_at":"2000-01-01T00:00:00Z"}`), 0o600)))
	src.TTL = time.Nanosecond
	stale, warnings, err := spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{})
	assert.Require(t, assert.NoError(t, err))
	assert.True(t, reflect.DeepEqual(first, stale))
	assert.Eq(t, 1, len(warnings))
	assert.Require(t, assert.NoError(t, spec.Remove(dir, "demo")))
	_, _, err = spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{})
	assertCode(t, err, model.ExitLocal)
	assert.True(t, strings.Contains(err.Error(), "run apic site refresh demo"))
}

func TestIndexCacheOnly(t *testing.T) {
	t.Run("stale cache does not fetch", func(t *testing.T) {
		dir := t.TempDir()
		raw, err := os.ReadFile(filepath.Join("testdata", "min-v3.json"))
		assert.Require(t, assert.NoError(t, err))
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			_, _ = w.Write(raw)
		}))
		defer server.Close()
		src := model.Source{URL: server.URL, TTL: time.Nanosecond}
		first, _, err := spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{Client: server.Client()})
		assert.Require(t, assert.NoError(t, err))
		cached, _, err := spec.Index(context.Background(), "demo", src, dir, spec.IndexOptions{CacheOnly: true, Client: server.Client()})
		assert.Require(t, assert.NoError(t, err))
		assert.True(t, reflect.DeepEqual(first, cached))
		assert.Eq(t, int32(1), calls.Load())
	})

	t.Run("missing cache does not fetch", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
		defer server.Close()
		_, _, err := spec.Index(context.Background(), "demo", model.Source{URL: server.URL}, t.TempDir(), spec.IndexOptions{CacheOnly: true, Client: server.Client()})
		assertCode(t, err, model.ExitLocal)
		assert.Contains(t, err.Error(), "no cached index for demo")
		assert.Eq(t, int32(0), calls.Load())
	})

	t.Run("simple source still loads", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "demo.yaml")
		assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte("operations:\n  one: {get: /one}\n"), 0600)))
		idx, _, err := spec.Index(context.Background(), "demo", model.Source{Type: "simple", File: file}, dir, spec.IndexOptions{CacheOnly: true})
		assert.Require(t, assert.NoError(t, err))
		assert.Eq(t, "one", idx.Ops[0].Name)
	})
}

func TestCacheSimpleNoCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	file := filepath.Join(dir, "demo.yaml")
	assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte("operations:\n  one: {get: /one}\n"), 0600)))
	idx, _, err := spec.Index(context.Background(), "demo", model.Source{Type: "simple", File: file}, dir, spec.IndexOptions{})
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, "one", idx.Ops[0].Name)
	assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte("operations:\n  two: {get: /two}\n"), 0600)))
	idx, _, err = spec.Index(context.Background(), "demo", model.Source{Type: "simple", File: file}, dir, spec.IndexOptions{})
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, "two", idx.Ops[0].Name)
	_, err = os.Stat(filepath.Join(dir, "cache", "specs", "demo.index.json"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestRealSpec(t *testing.T) {
	dir := os.Getenv("APIC_REAL_SPEC_DIR")
	if dir == "" {
		t.Skip("APIC_REAL_SPEC_DIR is not set")
	}
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		file  string
		count int
		big   bool
	}{
		{"big-v2.json", 30, true},
		{"small-v2.json", 9, false},
	} {
		t.Run(tc.file, func(t *testing.T) {
			idx, err := (spec.OpenAPILoader{}).Load(context.Background(), model.Source{File: filepath.Join(dir, tc.file)}, t.TempDir())
			assert.Require(t, assert.NoError(t, err))
			assert.Eq(t, tc.count, len(idx.Ops))
			if tc.big {
				assert.True(t, strings.HasPrefix(idx.DefaultBaseURL, "http://"))
			} else {
				assert.Eq(t, "", idx.DefaultBaseURL)
			}
			var names []string
			for _, op := range idx.Ops {
				if tc.big {
					assert.True(t, !strings.HasPrefix(op.Name, "post-"))
				}
				if len(names) < 10 {
					names = append(names, op.Name)
				}
			}
			t.Logf("first 10 operations: %v", names)
		})
	}
}

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	assert.Require(t, assert.Error(t, err))
	var coded *model.CodedError
	assert.Require(t, assert.True(t, errors.As(err, &coded)))
	assert.Eq(t, code, coded.Code())
}

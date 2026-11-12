package commands_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/request"
)

// pageDef is the definition used by the pagination cases. "list" declares no
// page, "paged" declares page and per_page, "req" makes page required, "create"
// is not a GET and "collide" uses the names the pagination options take.
const pageDef = `operations:
  list:
    get: /items
  paged:
    get: /items
    params:
      - {name: page, in: query, type: integer}
      - {name: per_page, in: query, type: integer}
  req:
    get: /items
    params:
      - {name: page, in: query, required: true, type: integer}
  create:
    post: /items
  collide:
    get: /items
    query: [all-pages, max-pages]
`

// pageAPI is an httptest server that records every request. The test sets handle
// to answer the list requests; the default answer is a small JSON object.
type pageAPI struct {
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []recorded
	handle func(w http.ResponseWriter, r *http.Request)
}

func newPageAPI(t *testing.T) *pageAPI {
	t.Helper()
	p := &pageAPI{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.reqs = append(p.reqs, recorded{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		h := p.handle
		p.mu.Unlock()
		if h == nil {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		h(w, r)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *pageAPI) setHandle(h func(w http.ResponseWriter, r *http.Request)) {
	p.mu.Lock()
	p.handle = h
	p.mu.Unlock()
}

func (p *pageAPI) requests() []recorded {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]recorded(nil), p.reqs...)
}

// reset forgets the requests seen so far, so a case counts only its own.
func (p *pageAPI) reset() {
	p.mu.Lock()
	p.reqs = nil
	p.mu.Unlock()
}

// queries returns the decoded query of every request, in order.
func (p *pageAPI) queries(t *testing.T) []url.Values {
	t.Helper()
	var out []url.Values
	for _, r := range p.requests() {
		v, err := url.ParseQuery(r.Query)
		assert.Require(t, assert.NoError(t, err))
		out = append(out, v)
	}
	return out
}

// firstQuery returns the query of the first request, failing the test when none.
func (p *pageAPI) firstQuery(t *testing.T) url.Values {
	t.Helper()
	q := p.queries(t)
	assert.Require(t, assert.True(t, len(q) > 0))
	return q[0]
}

// pageSite writes a fresh configuration directory with site "dh" (simple
// definition) and an exec auth "ex" that appends a line to the returned count
// file on every run. paginate is an already indented YAML fragment, or "" for a
// site without one.
func pageSite(t *testing.T, api *pageAPI, stub, def, paginate string) (count string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	api.reset()
	defPath := filepath.Join(dir, "dh-def.yaml")
	assert.NoError(t, os.WriteFile(defPath, []byte(def), 0o600))
	count = filepath.Join(t.TempDir(), "count")
	cfg := fmt.Sprintf(`version: 1
sites:
  dh:
    source: {type: simple, file: %q}
    default_env: test
    envs:
      test: {base_url: %q}
    auths:
      ex:
        type: exec
        exec: {command: [%q, %q, %q]}
    default_auth: ex
%s`, defPath, api.srv.URL, stub, "count="+count, "token=tok-page-1", paginate)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	return count
}

// pageItems returns the items of the requested page as JSON objects. base is the
// first page number (1, or 0 for a start: 0 site); size comes from the sz query
// parameter, defaulting to size and capped at cap when that is > 0.
func pageItems(r *http.Request, pg, sz string, total, size, cap, base int) []map[string]any {
	page := base
	if v := r.URL.Query().Get(pg); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			page = n
		}
	}
	if sz != "" {
		if v := r.URL.Query().Get(sz); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				size = n
			}
		}
	}
	if cap > 0 && size > cap {
		size = cap
	}
	start := (page - base) * size
	items := []map[string]any{}
	for i := start; i < start+size && i < total; i++ {
		items = append(items, map[string]any{"id": i + 1, "name": fmt.Sprintf("n%d", i+1)})
	}
	return items
}

// writeJSON writes v as a JSON response body.
func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// arrayHandler answers like GitLab: the body itself is the list of one page.
func arrayHandler(pg, sz string, total, size, base int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, pageItems(r, pg, sz, total, size, 0, base))
	}
}

// deployhubHandler answers like DeployHub: data.list plus data.total.
func deployhubHandler(pg, sz string, total, size, cap int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
			"list":  pageItems(r, pg, sz, total, size, cap, 1),
			"total": total,
		}})
	}
}

// statusHandler answers page n >= failAt with the given status and body, and the
// other pages like arrayHandler.
func statusHandler(status, failAt int, body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page >= failAt {
			w.WriteHeader(status)
			fmt.Fprint(w, body)
			return
		}
		writeJSON(w, pageItems(r, "page", "", 9, 3, 0, 1))
	}
}

func TestAllPages(t *testing.T) {
	api := newPageAPI(t)
	stub := buildExecStub(t)

	t.Run("gitlab style merges every page", func(t *testing.T) {
		count := pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 7, 3, 1))
		code, out, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 7, strings.Count(out, `"name"`), out)
		assert.Eq(t, 3, len(api.requests()))
		for i, q := range api.queries(t) {
			assert.Eq(t, strconv.Itoa(i+1), q.Get("page"))
		}
		assert.True(t, countLines(t, count) >= 1)
	})

	t.Run("deployhub style stops at the total", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page, items: data.list, total: data.total}\n")
		api.setHandle(deployhubHandler("page", "", 6, 3, 0))
		code, out, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 6, strings.Count(out, `"name"`), out)
		// Six rows in two pages: the total stops the loop, no third request.
		assert.Eq(t, 2, len(api.requests()))
	})

	t.Run("server caps the size, size_value 5", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page, size: per_page, size_value: 5}\n")
		// The server ignores the requested 5 and returns 2 per page: the first page
		// must not be taken as the last one.
		api.setHandle(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, pageItems(r, "page", "per_page", 5, 5, 2, 1))
		})
		code, out, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 5, strings.Count(out, `"name"`), out)
		assert.Eq(t, 3, len(api.requests()))
		assert.Eq(t, "5", api.firstQuery(t).Get("per_page"))
	})

	t.Run("one page without total asks twice", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 2, 3, 1))
		code, out, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 2, strings.Count(out, `"name"`), out)
		assert.Eq(t, 2, len(api.requests()))
	})

	t.Run("start page sources", func(t *testing.T) {
		t.Run("user value", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
			api.setHandle(arrayHandler("page", "", 9, 3, 1))
			code, out, errOut := runIO(t, "dh", "paged", "--page", "3", "--all-pages")
			assert.Eq(t, 0, code, errOut)
			assert.Eq(t, "3", api.firstQuery(t).Get("page"))
			assert.Eq(t, 3, strings.Count(out, `"name"`), out)
		})
		t.Run("user value through -p", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
			api.setHandle(arrayHandler("page", "", 9, 3, 1))
			code, _, errOut := runIO(t, "dh", "list", "-p", "query.page=2", "--all-pages")
			assert.Eq(t, 0, code, errOut)
			assert.Eq(t, "2", api.firstQuery(t).Get("page"))
		})
		t.Run("start 0", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, "    paginate: {page: page, start: 0}\n")
			api.setHandle(arrayHandler("page", "", 5, 3, 0))
			code, out, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 0, code, errOut)
			assert.Eq(t, "0", api.firstQuery(t).Get("page"))
			assert.Eq(t, 5, strings.Count(out, `"name"`), out)
		})
	})

	t.Run("user size beats size_value", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page, size: per_page, size_value: 9}\n")
		api.setHandle(arrayHandler("page", "per_page", 8, 2, 1))
		code, out, errOut := runIO(t, "dh", "paged", "--per_page", "2", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 8, strings.Count(out, `"name"`), out)
		for _, q := range api.queries(t) {
			assert.Eq(t, "2", q.Get("per_page"))
		}
	})

	t.Run("required page is filled in", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 4, 3, 1))
		code, out, errOut := runIO(t, "dh", "req", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 4, strings.Count(out, `"name"`), out)
		assert.Eq(t, "1", api.firstQuery(t).Get("page"))
	})

	t.Run("raw path call", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 6, 2, 1))
		code, out, errOut := runIO(t, "dh", "/items", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 6, strings.Count(out, `"name"`), out)
		assert.True(t, len(api.requests()) >= 2)
	})

	t.Run("failures", func(t *testing.T) {
		paginate := "    paginate: {page: page}\n"
		t.Run("page 2 returns 500", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(statusHandler(http.StatusInternalServerError, 2, `{"err":"boom"}`))
			code, out, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 5, code)
			assert.Contains(t, out, "boom")
			assert.Contains(t, errOut, "note: page 2 returned HTTP 500; 1 earlier pages discarded")
		})
		t.Run("page 1 returns 500", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(statusHandler(http.StatusInternalServerError, 1, `{"err":"boom"}`))
			code, out, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 5, code)
			assert.Contains(t, out, "boom")
			assert.NotContains(t, errOut, "discarded")
		})
		t.Run("page 2 returns 302", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(statusHandler(http.StatusFound, 2, ""))
			code, _, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Contains(t, errOut, "note: page 2 returned HTTP 302; 1 earlier pages discarded")
		})
		t.Run("page 2 is not JSON", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					fmt.Fprint(w, "hello")
					return
				}
				writeJSON(w, pageItems(r, "page", "", 9, 3, 0, 1))
			})
			code, _, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Contains(t, errOut, "--all-pages failed: page 2: response is not JSON")
		})
		t.Run("items is not an array", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, "    paginate: {page: page, items: data.list}\n")
			api.setHandle(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{"data": map[string]any{"list": "x"}})
			})
			code, _, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Contains(t, errOut, `--all-pages failed: page 1: items "data.list" is not an array`)
		})
		t.Run("total is not an integer", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, "    paginate: {page: page, items: data.list, total: data.total}\n")
			api.setHandle(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{"data": map[string]any{"list": []any{}, "total": "x"}})
			})
			code, _, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Contains(t, errOut, `total "data.total" is not a non-negative integer`)
		})
		t.Run("the page parameter is ignored", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, pageItems(r, "none", "", 9, 3, 0, 1))
			})
			code, _, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Contains(t, errOut, "repeats")
		})
		t.Run("max-pages truncates with a warning", func(t *testing.T) {
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(arrayHandler("page", "", 9, 3, 1))
			code, out, errOut := runIO(t, "dh", "list", "--max-pages", "2")
			assert.Eq(t, 0, code, errOut)
			assert.Eq(t, 6, strings.Count(out, `"name"`), out)
			assert.Eq(t, 2, len(api.requests()))
			assert.Contains(t, errOut, "warning: stopped after 2 pages (--max-pages); more may exist")
		})
		t.Run("default limit fails", func(t *testing.T) {
			old := request.DefaultMaxPages
			request.DefaultMaxPages = 2
			t.Cleanup(func() { request.DefaultMaxPages = old })
			pageSite(t, api, stub, pageDef, paginate)
			api.setHandle(arrayHandler("page", "", 9, 3, 1))
			code, out, errOut := runIO(t, "dh", "list", "--all-pages")
			assert.Eq(t, 7, code)
			assert.Eq(t, "", out)
			assert.Contains(t, errOut, "reached the default limit of 2 pages")
		})
	})

	t.Run("usage errors before authentication", func(t *testing.T) {
		cases := []struct {
			name     string
			paginate string
			args     []string
		}{
			{"no paginate", "", []string{"dh", "list", "--all-pages"}},
			{"post operation", "    paginate: {page: page}\n", []string{"dh", "create", "--all-pages"}},
			{"lowercase -X post", "    paginate: {page: page}\n", []string{"dh", "/items", "-X", "post", "--all-pages"}},
			{"page is not an integer", "    paginate: {page: page}\n", []string{"dh", "paged", "--page", "abc", "--all-pages"}},
			{"size is not an integer", "    paginate: {page: page, size: per_page}\n", []string{"dh", "paged", "--per_page", "x", "--all-pages"}},
			{"max-pages zero", "    paginate: {page: page}\n", []string{"dh", "list", "--all-pages", "--max-pages", "0"}},
			{"max-pages not a number", "    paginate: {page: page}\n", []string{"dh", "list", "--all-pages", "--max-pages", "abc"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				count := pageSite(t, api, stub, pageDef, tc.paginate)
				api.setHandle(arrayHandler("page", "", 9, 3, 1))
				before := len(api.requests())
				code, out, errOut := runIO(t, tc.args...)
				assert.Eq(t, 2, code, errOut)
				assert.Eq(t, "", out)
				// Nothing was sent and the exec auth of the call never ran.
				assert.Eq(t, before, len(api.requests()))
				assert.Eq(t, 0, countLines(t, count))
			})
		}
	})

	t.Run("dry-run prints one curl", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 9, 3, 1))
		before := len(api.requests())
		code, out, errOut := runIO(t, "dh", "list", "--all-pages", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 1, strings.Count(out, "curl"))
		assert.Contains(t, errOut, "note: dry-run: --all-pages would request further pages")
		assert.Eq(t, before, len(api.requests()))
	})

	t.Run("pick and table apply to the merged array", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 5, 3, 1))
		code, out, errOut := runIO(t, "dh", "list", "--all-pages", "--pick", "*.{id,name}", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		assert.Eq(t, []string{"id", "name"}, strings.Fields(lines[0]))
		assert.Eq(t, 6, len(lines))
	})

	t.Run("insecure warning is printed once", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("APIC_CONFIG_DIR", dir)
		api.reset()
		defPath := filepath.Join(dir, "dh-def.yaml")
		assert.NoError(t, os.WriteFile(defPath, []byte(pageDef), 0o600))
		cfg := fmt.Sprintf(`version: 1
sites:
  dh:
    source: {type: simple, file: %q}
    default_env: test
    envs:
      test: {base_url: %q, insecure: true}
    paginate: {page: page}
`, defPath, api.srv.URL)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
		api.setHandle(arrayHandler("page", "", 9, 3, 1))
		code, _, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 1, strings.Count(errOut, "warning: TLS certificate verification is disabled"))
		assert.True(t, len(api.requests()) >= 2)
	})

	t.Run("an expired token on page 2 is renewed", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("APIC_CONFIG_DIR", dir)
		api.reset()
		defPath := filepath.Join(dir, "dh-def.yaml")
		assert.NoError(t, os.WriteFile(defPath, []byte(pageDef), 0o600))
		var mu sync.Mutex
		logins := 0
		api.setHandle(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				mu.Lock()
				logins++
				n := logins
				mu.Unlock()
				writeJSON(w, map[string]any{"token": fmt.Sprintf("t%d", n), "expires_in": 3600})
				return
			}
			// The first token only opens the first page; page 2 needs a new one.
			want := "Bearer t1"
			if r.URL.Query().Get("page") != "1" {
				want = "Bearer t2"
			}
			if r.Header.Get("Authorization") != want {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"err":"expired"}`)
				return
			}
			writeJSON(w, pageItems(r, "page", "", 6, 3, 0, 1))
		})
		cfg := fmt.Sprintf(`version: 1
sites:
  dh:
    source: {type: simple, file: %q}
    default_env: test
    envs:
      test: {base_url: %q}
    default_auth: lg
    auths:
      lg:
        type: login
        login: {method: POST, path: /login}
        token: {from: body, path: token}
        expires: {from: body, path: expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
    paginate: {page: page}
`, defPath, api.srv.URL)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
		code, out, errOut := runIO(t, "dh", "list", "--all-pages")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, 6, strings.Count(out, `"name"`), out)
		assert.Eq(t, 2, logins)
	})

	t.Run("option names stay reserved", func(t *testing.T) {
		pageSite(t, api, stub, pageDef, "    paginate: {page: page}\n")
		api.setHandle(arrayHandler("page", "", 9, 3, 1))
		// --all-pages and --max-pages are the global options, not parameters of the
		// operation: --max-pages 1 limits the run instead of being sent as a query.
		code, out, errOut := runIO(t, "dh", "collide", "--max-pages", "1", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.NotContains(t, out, "all-pages=")
		assert.NotContains(t, out, "max-pages=")
		assert.Contains(t, out, "page=1")
		// -p still passes both names to the request.
		code, out, errOut = runIO(t, "dh", "collide", "-p", "all-pages=v", "-p", "max-pages=9", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "all-pages=v")
		assert.Contains(t, out, "max-pages=9")
	})
}

// pickListDef serves the DeployHub shape used by the pick command checks.
const pickListDef = `operations:
  items:
    get: /items
`

// TestPickCommand covers the command-level part of the pick definition: the
// table columns of a group and the exit code of a syntax error.
func TestPickCommand(t *testing.T) {
	api := newPageAPI(t)
	api.setHandle(deployhubHandler("page", "", 3, 3, 0))
	pageSite(t, api, buildExecStub(t), pickListDef, "")

	t.Run("table columns follow the group", func(t *testing.T) {
		code, out, errOut := runIO(t, "dh", "items", "--pick", "data.list.*.{id,name}", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		assert.Eq(t, []string{"id", "name"}, strings.Fields(lines[0]))
		assert.Eq(t, 4, len(lines))
	})

	t.Run("syntax error exits 2 before the request", func(t *testing.T) {
		before := len(api.requests())
		for _, bad := range []string{"data.list.{a", "data..list", ",data"} {
			code, out, errOut := runIO(t, "dh", "items", "--pick", bad)
			assert.Eq(t, 2, code, errOut)
			assert.Eq(t, "", out)
		}
		assert.Eq(t, before, len(api.requests()))
	})
}

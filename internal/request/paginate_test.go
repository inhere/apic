package request

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

// pageLog records the page number and raw query of every request a test server got.
type pageLog struct {
	mu      sync.Mutex
	pages   []int
	queries []string
}

func (l *pageLog) add(page int, rawQuery string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pages = append(l.pages, page)
	l.queries = append(l.queries, rawQuery)
}

func (l *pageLog) seen() ([]int, []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]int(nil), l.pages...), append([]string(nil), l.queries...)
}

// pageSrv serves /list and records what each request asked for.
func pageSrv(t *testing.T, h func(page int, w http.ResponseWriter)) (*httptest.Server, *pageLog) {
	t.Helper()
	log := &pageLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		log.add(page, r.URL.RawQuery)
		h(page, w)
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

// pageCall builds a GET call whose URL already carries page=7, so the loop must
// overwrite it. The returned Prepared belongs to the caller and must stay as is.
func pageCall(t *testing.T, srv *httptest.Server) (Call, *Prepared) {
	t.Helper()
	pr := prep(t, srv.URL, "GET", "/list", Input{P: []string{"page=7", "size=5"}})
	sec := &model.Secrets{}
	return Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL}, Redactor: &model.Redactor{Secrets: sec}, Timeout: 5 * time.Second}, pr
}

func writeBody(w http.ResponseWriter, body string) {
	_, _ = w.Write([]byte(body))
}

// rows renders [{"id":1},{"id":2}].
func rows(ids ...int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d}`, id)
	}
	b.WriteByte(']')
	return b.String()
}

// idsFrom returns up to n ids starting at from, never above max.
func idsFrom(from, n, max int) []int {
	var ids []int
	for id := from; id <= max && len(ids) < n; id++ {
		ids = append(ids, id)
	}
	return ids
}

// dhBody wraps a list and a total the way a data-envelope API does.
func dhBody(list string, total int) string {
	return `{"data":{"list":` + list + `,"total":` + strconv.Itoa(total) + `}}`
}

// pageAuth rejects the first token until Refresh swaps in a good one.
type pageAuth struct {
	mu       sync.Mutex
	token    string
	refreshN int
}

func (a *pageAuth) Apply(_ context.Context, req *http.Request, _ bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a *pageAuth) Refresh(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refreshN++
	a.token = "good"
	return nil
}

func (a *pageAuth) Invalidate() error { return nil }

func TestPaginate(t *testing.T) {
	const max = 10

	t.Run("capped page size still collects every row", func(t *testing.T) {
		// Five rows; the server caps a page at two rows whatever size it was asked.
		srv, log := pageSrv(t, func(page int, w http.ResponseWriter) {
			writeBody(w, rows(idsFrom((page-1)*2+1, 2, 5)...))
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 0, res.FailedAt)
		assert.Eq(t, false, res.Truncated)
		assert.Eq(t, rows(1, 2, 3, 4, 5), string(res.Resp.Body))
		pages, _ := log.seen()
		assert.Eq(t, []int{1, 2, 3}, pages)
		assert.Eq(t, 3, res.Pages)
	})

	t.Run("total stops as soon as the rows are complete", func(t *testing.T) {
		srv, log := pageSrv(t, func(page int, w http.ResponseWriter) {
			if page == 1 {
				writeBody(w, dhBody(rows(1, 2), 3))
				return
			}
			writeBody(w, dhBody(rows(3), 3))
		})
		c, _ := pageCall(t, srv)
		spec := model.Paginate{Page: "page", Items: "data.list", Total: "data.total"}
		res, err := Paginate(context.Background(), c, PageOptions{Spec: spec, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, rows(1, 2, 3), string(res.Resp.Body))
		pages, _ := log.seen()
		assert.Eq(t, []int{1, 2}, pages)
	})

	t.Run("single page without total costs one extra request", func(t *testing.T) {
		srv, log := pageSrv(t, func(page int, w http.ResponseWriter) {
			w.Header().Set("X-Page", strconv.Itoa(page))
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			writeBody(w, rows())
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, rows(1, 2), string(res.Resp.Body))
		pages, _ := log.seen()
		assert.Eq(t, []int{1, 2}, pages)
		// status and headers come from the last page
		assert.Eq(t, http.StatusOK, res.Resp.Status)
		assert.Eq(t, "2", res.Resp.Header.Get("X-Page"))
	})

	t.Run("first page 500 is the failing page", func(t *testing.T) {
		srv, _ := pageSrv(t, func(_ int, w http.ResponseWriter) {
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(w, "boom")
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 1, res.FailedAt)
		assert.Eq(t, 0, res.Discarded)
		assert.Eq(t, http.StatusInternalServerError, res.Resp.Status)
		assert.Eq(t, "boom", string(res.Resp.Body))
	})

	t.Run("second page 500 keeps that page as is", func(t *testing.T) {
		srv, _ := pageSrv(t, func(page int, w http.ResponseWriter) {
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(w, "server error")
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 2, res.FailedAt)
		assert.Eq(t, 1, res.Discarded)
		assert.Eq(t, 2, res.Pages)
		assert.Eq(t, http.StatusInternalServerError, res.Resp.Status)
		assert.Eq(t, "server error", string(res.Resp.Body))
	})

	t.Run("second page 302 is a failure", func(t *testing.T) {
		srv, _ := pageSrv(t, func(page int, w http.ResponseWriter) {
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			w.Header().Set("Location", "/login")
			w.WriteHeader(http.StatusFound)
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 2, res.FailedAt)
		assert.Eq(t, 1, res.Discarded)
		assert.Eq(t, http.StatusFound, res.Resp.Status)
	})

	t.Run("second page is not JSON", func(t *testing.T) {
		for _, body := range []string{"<html>oops</html>", ""} {
			srv, _ := pageSrv(t, func(page int, w http.ResponseWriter) {
				if page == 1 {
					writeBody(w, rows(1, 2))
					return
				}
				writeBody(w, body)
			})
			c, _ := pageCall(t, srv)
			_, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
			assert.Eq(t, model.ExitPostProcess, code(err), body)
			assert.Contains(t, err.Error(), "--all-pages failed: page 2: response is not JSON")
		}
	})

	t.Run("items path is not an array", func(t *testing.T) {
		srv, _ := pageSrv(t, func(_ int, w http.ResponseWriter) {
			writeBody(w, `{"data":{"list":{"a":1}}}`)
		})
		c, _ := pageCall(t, srv)
		_, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page", Items: "data.list"}, First: 1, Max: max})
		assert.Eq(t, model.ExitPostProcess, code(err))
		assert.Contains(t, err.Error(), `items "data.list" is not an array`)
	})

	t.Run("total is not a non-negative integer", func(t *testing.T) {
		for _, total := range []string{`"abc"`, "-3", "1.5"} {
			srv, _ := pageSrv(t, func(_ int, w http.ResponseWriter) {
				writeBody(w, `{"data":{"list":[{"id":1}],"total":`+total+`}}`)
			})
			c, _ := pageCall(t, srv)
			spec := model.Paginate{Page: "page", Items: "data.list", Total: "data.total"}
			_, err := Paginate(context.Background(), c, PageOptions{Spec: spec, First: 1, Max: max})
			assert.Eq(t, model.ExitPostProcess, code(err), total)
			assert.Contains(t, err.Error(), "total")
		}
	})

	t.Run("repeated page exits 7", func(t *testing.T) {
		// The server ignores the page parameter.
		srv, _ := pageSrv(t, func(_ int, w http.ResponseWriter) {
			writeBody(w, rows(1, 2))
		})
		c, _ := pageCall(t, srv)
		_, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.Eq(t, model.ExitPostProcess, code(err))
		assert.Contains(t, err.Error(), "--all-pages failed: page 2 repeats page 1; check paginate.page")
	})

	t.Run("explicit max pages truncates and keeps the pages", func(t *testing.T) {
		srv, log := pageSrv(t, func(page int, w http.ResponseWriter) {
			writeBody(w, rows(idsFrom((page-1)*2+1, 2, 6)...))
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: 2, Explicit: true})
		assert.NoErr(t, err)
		assert.Eq(t, true, res.Truncated)
		assert.Eq(t, 2, res.Pages)
		assert.Eq(t, rows(1, 2, 3, 4), string(res.Resp.Body))
		pages, _ := log.seen()
		assert.Eq(t, []int{1, 2}, pages)
	})

	t.Run("default max pages is an error", func(t *testing.T) {
		old := DefaultMaxPages
		DefaultMaxPages = 2
		t.Cleanup(func() { DefaultMaxPages = old })

		srv, _ := pageSrv(t, func(page int, w http.ResponseWriter) {
			writeBody(w, rows(idsFrom((page-1)*2+1, 2, 1<<20)...))
		})
		c, _ := pageCall(t, srv)
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: DefaultMaxPages})
		assert.Nil(t, res)
		assert.Eq(t, model.ExitPostProcess, code(err))
		assert.Contains(t, err.Error(), "reached the default limit of 2 pages")
	})

	t.Run("second page retries after 401", func(t *testing.T) {
		// Page two rejects the stale token; Execute refreshes it and retries once.
		log := &pageLog{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			log.add(page, r.URL.RawQuery)
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			if r.Header.Get("Authorization") != "Bearer good" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeBody(w, rows(3))
		}))
		t.Cleanup(srv.Close)
		auth := &pageAuth{token: "stale"}
		c, _ := pageCall(t, srv)
		c.Authorizer = auth
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 1, auth.refreshN)
		assert.Eq(t, rows(1, 2, 3), string(res.Resp.Body))
		assert.Eq(t, 2, res.Pages)
	})

	t.Run("dry run builds the first page only", func(t *testing.T) {
		srv, log := pageSrv(t, func(_ int, w http.ResponseWriter) {
			writeBody(w, rows(1, 2))
		})
		c, pr := pageCall(t, srv)
		c.DryRun = true
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, 1, res.Pages)
		assert.Contains(t, res.Resp.Curl, "page=1")
		pages, _ := log.seen()
		assert.Eq(t, 0, len(pages))
		assert.Contains(t, pr.URL, "page=7")
	})

	t.Run("insecure warning is printed once", func(t *testing.T) {
		calls := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			calls++
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			writeBody(w, rows())
		}))
		t.Cleanup(srv.Close)

		pr := prep(t, srv.URL, "GET", "/list", Input{})
		var stderr bytes.Buffer
		c := Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL, Insecure: true}, Redactor: rd(), Timeout: 5 * time.Second, Stderr: &stderr}
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, rows(1, 2), string(res.Resp.Body))
		assert.StrCount(t, stderr.String(), "insecure: true", 1)
		assert.Eq(t, 2, calls)
	})

	t.Run("pages do not leak into the caller URL", func(t *testing.T) {
		srv, log := pageSrv(t, func(page int, w http.ResponseWriter) {
			if page == 1 {
				writeBody(w, rows(1, 2))
				return
			}
			writeBody(w, rows(3))
		})
		c, pr := pageCall(t, srv)
		want := pr.URL
		res, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
		assert.NoErr(t, err)
		assert.Eq(t, rows(1, 2, 3), string(res.Resp.Body))
		assert.Eq(t, want, pr.URL)
		assert.Eq(t, want, c.Prepared.URL)
		_, queries := log.seen()
		assert.Eq(t, 2, len(queries))
		assert.Contains(t, queries[0], "page=1")
		assert.Contains(t, queries[1], "page=2")
		for _, q := range queries {
			assert.Eq(t, 1, strings.Count(q, "page="), q)
		}
	})

	t.Run("non-GET method is a usage error", func(t *testing.T) {
		hits := 0
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
		t.Cleanup(srv.Close)
		for _, method := range []string{"POST", "post"} {
			pr := prep(t, srv.URL, method, "/list", Input{})
			c := Call{Prepared: pr, Env: model.Env{BaseURL: srv.URL}, Redactor: rd(), Timeout: 5 * time.Second}
			_, err := Paginate(context.Background(), c, PageOptions{Spec: model.Paginate{Page: "page"}, First: 1, Max: max})
			assert.Eq(t, model.ExitUsage, code(err), method)
		}
		assert.Eq(t, 0, hits)
	})
}

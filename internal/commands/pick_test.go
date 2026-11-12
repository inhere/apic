package commands_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const pickDef = `operations:
  items:
    get: /items
    query: [pick]
  plain:
    get: /plain
  empty:
    get: /empty
  status:
    get: /status/{code}
`

// setupPick writes a site "demo" backed by a handler serving JSON, text, 204 and status codes.
func setupPick(t *testing.T) *atomic.Int32 {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/items":
			fmt.Fprint(w, `{"data":{"items":[{"id":1,"name":"a"},{"id":2,"name":"b"}],"title":"hello"},"z":1,"a":2}`)
		case "/plain":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "not json")
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"data":{"id":9},"err":"nf"}`)
		}
	}))
	t.Cleanup(srv.Close)
	def := filepath.Join(dir, "demo-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(pickDef), 0600))
	cfg := fmt.Sprintf(`version: 1
sites:
  demo:
    source: {type: simple, file: %q}
    default_env: test
    envs:
      test: {base_url: %q}
`, def, srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0600))
	return &hits
}

func TestPick(t *testing.T) {
	hits := setupPick(t)

	t.Run("array and string", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.items.*.id")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "1")
		assert.Contains(t, out, "2")
		code, out, errOut = runIO(t, "demo", "items", "--pick", "data.title")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "hello\n", out)
		// the option also works before the site name and on a raw path call.
		code, out, errOut = runIO(t, "--pick", "data.title", "demo", "items")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "hello\n", out)
		code, out, errOut = runIO(t, "demo", "/items", "--pick", "data.title")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "hello\n", out)
	})

	t.Run("yaml and table", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.items.*.id", "-o", "yaml")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "- 1")
		code, out, errOut = runIO(t, "demo", "items", "--pick", "data.items", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "name")
	})

	t.Run("exit 7 leaves stdout empty", func(t *testing.T) {
		for _, args := range [][]string{
			{"demo", "plain", "--pick", "a"},
			{"demo", "empty", "--pick", "a"},
			{"demo", "items", "--pick", "no.such.path"},
			{"demo", "plain", "-o", "yaml"},
		} {
			code, out, errOut := runIO(t, args...)
			assert.Eq(t, 7, code, errOut)
			assert.Eq(t, "", out)
			assert.Contains(t, errOut, " succeeded;")
			assert.Contains(t, errOut, "HTTP 20")
		}
	})

	t.Run("invalid path sends no request", func(t *testing.T) {
		before := hits.Load()
		for _, bad := range []string{"a..b", ".a", "a."} {
			code, out, errOut := runIO(t, "demo", "items", "--pick", bad)
			assert.Eq(t, 2, code, errOut)
			assert.Eq(t, "", out)
		}
		// dry-run validates the path too, but its output is unaffected.
		code, _, _ := runIO(t, "demo", "items", "--pick", "a..b", "--dry-run")
		assert.Eq(t, 2, code)
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.title", "-o", "yaml", "--dry-run")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, "curl")
		assert.Eq(t, before, hits.Load())
	})

	t.Run("4xx keeps the body and exit code", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "status", "--code", "404", "--pick", "data.id")
		assert.Eq(t, 4, code, errOut)
		assert.Contains(t, out, `"err"`)
		assert.Contains(t, errOut, "HTTP 404")
	})

	t.Run("no pick keeps the old behaviour", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "plain")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "not json", out)
		code, out, errOut = runIO(t, "demo", "empty", "-o", "json")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "", out)
	})
}

func TestPickParamFallback(t *testing.T) {
	setupPick(t)
	// --pick is the global option; a query parameter named pick goes through -p.
	code, out, errOut := runIO(t, "demo", "/items", "-p", "pick=v", "--dry-run")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "pick=v")
	code, out, errOut = runIO(t, "demo", "items", "-p", "pick=v", "--dry-run")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "pick=v")
	// --pick does not become the query parameter.
	code, out, errOut = runIO(t, "demo", "items", "--pick", "data.title", "--dry-run")
	assert.Eq(t, 0, code, errOut)
	assert.NotContains(t, out, "pick=")
}

func TestOutputModes(t *testing.T) {
	setupPick(t)
	for _, mode := range []string{"auto", "json", "raw", "yaml", "table"} {
		code, _, errOut := runIO(t, "demo", "items", "-o", mode)
		assert.Eq(t, 0, code, mode+": "+errOut)
	}
	code, _, errOut := runIO(t, "demo", "items", "-o", "xml")
	assert.Eq(t, 2, code)
	assert.Contains(t, errOut, "auto, json, raw, yaml or table")
}

func TestPickProjection(t *testing.T) {
	setupPick(t)
	t.Run("table columns follow the group", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.items.*.{id,name}", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		assert.Eq(t, []string{"id", "name"}, strings.Fields(lines[0]))
		assert.Eq(t, 3, len(lines))
		code, out, errOut = runIO(t, "demo", "items", "--pick", "data.items.*.{name,id}", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, []string{"name", "id"}, strings.Fields(strings.SplitN(out, "\n", 2)[0]))
	})
	t.Run("root object from several items", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "z,a,missing", "-o", "json")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, `"z": 1`)
		assert.Contains(t, out, `"missing": null`)
		assert.True(t, strings.Index(out, `"z"`) < strings.Index(out, `"a"`))
	})
	t.Run("missing keys below star are null", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.items.*.{id,missing}", "-o", "json")
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out, `"missing": null`)
	})
	t.Run("object result prints as a key/value table", func(t *testing.T) {
		code, out, errOut := runIO(t, "demo", "items", "--pick", "data.items.*.{id,name},data.total", "-o", "table")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, []string{"KEY", "VALUE"}, strings.Fields(strings.SplitN(out, "\n", 2)[0]))
		assert.Contains(t, out, "items")
		assert.Contains(t, out, "total")
	})
	t.Run("group needs an object", func(t *testing.T) {
		code, _, errOut := runIO(t, "demo", "items", "--pick", "data.title.{a,b}")
		assert.Eq(t, 7, code)
		assert.Contains(t, errOut, "needs an object")
	})
}

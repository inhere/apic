package commands_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const bundleDef = "# mirror definition\noperations:\n  list-items:\n    get: /items\n    summary: list items\n  get-item:\n    get: /items/{id}\n"

// setupMirror creates a managed simple site "mirror" with an overlay in dir.
func setupMirror(t *testing.T, siteExtra string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	def := filepath.Join(dir, "src-def.yaml")
	assert.NoError(t, os.WriteFile(def, []byte(bundleDef), 0600))
	code, _ := runCommand(t, "site", "add", "mirror", "--def", def, "--base-url", "https://api.example.com")
	assert.Require(t, assert.Eq(t, 0, code))
	if siteExtra != "" {
		file := filepath.Join(dir, "sites", "mirror.yaml")
		data, err := os.ReadFile(file)
		assert.NoError(t, err)
		assert.NoError(t, os.WriteFile(file, append(data, []byte(siteExtra)...), 0600))
	}
	return dir
}

func opsOf(t *testing.T, site string) string {
	t.Helper()
	code, out := runCommand(t, "ops", site, "-o", "json")
	assert.Require(t, assert.Eq(t, 0, code))
	return out
}

func TestSiteExportImportRoundTrip(t *testing.T) {
	dir := setupMirror(t, "auths:\n  ci:\n    type: bearer\n    token: ${env:MIRROR_TOKEN}\n")
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "overlays"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "overlays", "mirror.yaml"), []byte("# note\noperations:\n  list-items: {summary: all items}\n"), 0600))
	want := opsOf(t, "mirror")
	out := filepath.Join(t.TempDir(), "mirror.bundle.yaml")
	code, _, stderr := runIO(t, "site", "export", "mirror", "--out", out)
	assert.Eq(t, 0, code, stderr)
	data, err := os.ReadFile(out)
	assert.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "kind: site")
	assert.Contains(t, text, "---\nkind: definition\n# mirror definition\n")
	assert.Contains(t, text, "---\nkind: overlay\n# note\n")
	assert.Contains(t, text, "${env:MIRROR_TOKEN}")
	assert.NotContains(t, text, "file:")
	// An existing --out is never overwritten.
	code, _, _ = runIO(t, "site", "export", "mirror", "--out", out)
	assert.Eq(t, 1, code)

	fresh := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", fresh)
	code, stdout, stderr := runIO(t, "site", "import", out)
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, stdout, "imported site mirror: sites/mirror.yaml defs/mirror.yaml overlays/mirror.yaml")
	assert.Eq(t, want, opsOf(t, "mirror"))
	site, err := os.ReadFile(filepath.Join(fresh, "sites", "mirror.yaml"))
	assert.NoError(t, err)
	assert.Contains(t, string(site), "defs/mirror.yaml")
	def, err := os.ReadFile(filepath.Join(fresh, "defs", "mirror.yaml"))
	assert.NoError(t, err)
	assert.Eq(t, bundleDef, string(def))
	// The imported site is managed, so it can be removed again.
	code, _ = runCommand(t, "site", "rm", "mirror")
	assert.Eq(t, 0, code)
	// Import under another name from stdin is not needed; --name is.
	code, stdout, _ = runIO(t, "site", "import", out, "--name", "copy")
	assert.Eq(t, 0, code)
	assert.Contains(t, stdout, "imported site copy: sites/copy.yaml defs/copy.yaml overlays/copy.yaml")
}

func TestSiteExportRefusesPlaintext(t *testing.T) {
	setupMirror(t, "auths:\n  ci:\n    type: bearer\n    token: s3cret-literal\n")
	code, stdout, stderr := runIO(t, "site", "export", "mirror")
	assert.Eq(t, 1, code)
	assert.Eq(t, "", stdout)
	assert.Contains(t, stderr, "site mirror auth ci token: plaintext credential")
	assert.NotContains(t, stderr, "s3cret-literal")
	code, stdout, _ = runIO(t, "site", "export", "mirror", "--allow-plaintext")
	assert.Eq(t, 0, code)
	assert.Contains(t, stdout, "s3cret-literal")
}

func TestSiteExportOpenAPINotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	code, _ := runCommand(t, "site", "add", "demo", "--spec", fixture(t, "min-v2.json"))
	assert.Require(t, assert.Eq(t, 0, code))
	code, stdout, stderr := runIO(t, "site", "export", "demo")
	assert.Eq(t, 0, code)
	assert.Contains(t, stderr, "not embedded")
	assert.NotContains(t, stdout, "kind: definition")
	assert.NotContains(t, stdout, "file:")
}

func TestSiteImportExecGate(t *testing.T) {
	setupMirror(t, "auths:\n  ci:\n    type: bearer\n    token: ${exec:vault read token}\n  sso:\n    type: exec\n    exec:\n      command: [get-token, --fresh]\n")
	bundleFile := filepath.Join(t.TempDir(), "b.yaml")
	code, _, stderr := runIO(t, "site", "export", "mirror", "--out", bundleFile)
	assert.Eq(t, 0, code, stderr)
	fresh := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", fresh)
	code, _, stderr = runIO(t, "site", "import", bundleFile)
	assert.Eq(t, 2, code)
	assert.Contains(t, stderr, "site mirror auth ci token: vault read token")
	assert.Contains(t, stderr, "site mirror auth sso exec.command: get-token --fresh")
	assert.Contains(t, stderr, "--yes")
	_, err := os.Stat(filepath.Join(fresh, "sites", "mirror.yaml"))
	assert.True(t, os.IsNotExist(err))
	code, stdout, stderr := runIO(t, "site", "import", bundleFile, "--yes")
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, stdout, "imported site mirror")
	assert.Contains(t, stderr, "runs ${exec:}")
}

func TestSiteImportConflicts(t *testing.T) {
	dir := setupMirror(t, "")
	bundleFile := filepath.Join(t.TempDir(), "b.yaml")
	code, _, _ := runIO(t, "site", "export", "mirror", "--out", bundleFile)
	assert.Eq(t, 0, code)
	code, _, stderr := runIO(t, "site", "import", bundleFile)
	assert.Eq(t, 1, code)
	assert.Contains(t, stderr, "already exists")
	// --force replaces the files of a sites/ site.
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "defs"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "defs", "mirror.yaml"), []byte("operations:\n  other: {get: /o}\n"), 0600))
	code, _, stderr = runIO(t, "site", "import", bundleFile, "--force")
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, opsOf(t, "mirror"), "list-items")
	// A stray file without a site is a conflict too.
	code, _ = runCommand(t, "site", "rm", "mirror")
	assert.Eq(t, 0, code)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "defs", "mirror.yaml"), []byte("x"), 0600))
	code, _, stderr = runIO(t, "site", "import", bundleFile)
	assert.Eq(t, 1, code)
	assert.Contains(t, stderr, "defs/mirror.yaml already exists")
	// A site from config.yaml is never replaced, even with --force.
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("version: 1\nsites:\n  fixed:\n    source: {type: simple, file: defs/mirror.yaml}\n    envs: {default: {base_url: \"https://api.example.com\"}}\n"), 0600))
	code, _, stderr = runIO(t, "site", "import", bundleFile, "--name", "fixed", "--force")
	assert.Eq(t, 1, code)
	assert.Contains(t, stderr, "config.yaml")
}

func TestSiteImportRejectsBadBundles(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	write := func(text string) string {
		p := filepath.Join(t.TempDir(), "b.yaml")
		assert.NoError(t, os.WriteFile(p, []byte(text), 0600))
		return p
	}
	site := "kind: site\nname: x\nsite:\n  source: {type: simple}\n  envs: {default: {base_url: \"https://a.example.com\"}}\n"
	for name, text := range map[string]string{
		"no definition":  site,
		"unknown kind":   site + "---\nkind: other\n",
		"bad definition": site + "---\nkind: definition\noperations:\n  Bad_Name: {get: /x}\n",
		"two sites":      site + "---\n" + site,
		"bad overlay":    site + "---\nkind: definition\noperations:\n  a: {get: /a}\n---\nkind: overlay\noperations:\n  a: {body: nope}\n",
		"no kind":        "operations: {}\n",
	} {
		code, _, _ := runIO(t, "site", "import", write(text))
		assert.Eq(t, 1, code, name)
	}
	_, err := os.Stat(filepath.Join(os.Getenv("APIC_CONFIG_DIR"), "sites", "x.yaml"))
	assert.True(t, os.IsNotExist(err))
	code, _, _ := runIO(t, "site", "import", write(site+"---\nkind: definition\noperations:\n  a: {get: /a}\n"), "--name", "ops")
	assert.Eq(t, 1, code, "reserved name")
}

func TestSiteImportURL(t *testing.T) {
	dir := setupMirror(t, "")
	bundleFile := filepath.Join(t.TempDir(), "b.yaml")
	code, _, _ := runIO(t, "site", "export", "mirror", "--out", bundleFile)
	assert.Eq(t, 0, code)
	data, err := os.ReadFile(bundleFile)
	assert.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.yaml":
			_, _ = w.Write(data)
		case "/big.yaml":
			_, _ = w.Write([]byte(strings.Repeat("#", 1<<20+10)))
		case "/missing.yaml":
			http.NotFound(w, r)
		default:
			http.Error(w, "boom", http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	_ = dir
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	code, _, stderr := runIO(t, "site", "import", srv.URL+"/big.yaml")
	assert.Eq(t, 1, code)
	assert.Contains(t, stderr, "larger than")
	code, _, _ = runIO(t, "site", "import", srv.URL+"/missing.yaml")
	assert.Eq(t, 4, code)
	code, _, _ = runIO(t, "site", "import", srv.URL+"/other.yaml")
	assert.Eq(t, 5, code)
	code, _, stderr = runIO(t, "site", "import", srv.URL+"/ok.yaml")
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, opsOf(t, "mirror"), "get-item")
}

func TestSiteAddDefURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/def.yaml":
			_, _ = w.Write([]byte(bundleDef))
		case "/bad.yaml":
			_, _ = w.Write([]byte("operations:\n  Bad_Name: {get: /x}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	code, stdout, stderr := runIO(t, "site", "add", "remote", "--def", srv.URL+"/def.yaml", "--base-url", "https://api.example.com")
	assert.Eq(t, 0, code, stderr)
	assert.Contains(t, stdout, "remote\t2\t")
	site, err := os.ReadFile(filepath.Join(dir, "sites", "remote.yaml"))
	assert.NoError(t, err)
	assert.Contains(t, string(site), "file: defs/remote.yaml")
	assert.Contains(t, opsOf(t, "remote"), "list-items")
	// A failing validation leaves no downloaded file behind.
	code, _, _ = runIO(t, "site", "add", "bad", "--def", srv.URL+"/bad.yaml", "--base-url", "https://api.example.com")
	assert.Eq(t, 1, code)
	_, err = os.Stat(filepath.Join(dir, "defs", "bad.yaml"))
	assert.True(t, os.IsNotExist(err))
	code, _, _ = runIO(t, "site", "add", "gone", "--def", srv.URL+"/nope.yaml", "--base-url", "https://api.example.com")
	assert.Eq(t, 4, code)
	// An existing defs file blocks the add and is kept.
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "defs", "keep.yaml"), []byte("x"), 0600))
	code, _, _ = runIO(t, "site", "add", "keep", "--def", srv.URL+"/def.yaml", "--base-url", "https://api.example.com")
	assert.Eq(t, 1, code)
	kept, _ := os.ReadFile(filepath.Join(dir, "defs", "keep.yaml"))
	assert.Eq(t, "x", string(kept))
}

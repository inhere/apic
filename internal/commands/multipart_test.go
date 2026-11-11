package commands_test

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// mpPart is one multipart part as the server saw it.
type mpPart struct{ Name, Filename, Type, Data string }

// mpServer records the Content-Type, parts and raw body of the last request.
type mpServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	ct   string
	raw  string
	part []mpPart
	n    int
}

func mpNewServer(t *testing.T) *mpServer {
	t.Helper()
	s := &mpServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.n++
		s.ct, s.raw, s.part = r.Header.Get("Content-Type"), string(body), nil
		if mt, params, err := mime.ParseMediaType(s.ct); err == nil && strings.HasPrefix(mt, "multipart/") {
			mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(p)
				s.part = append(s.part, mpPart{p.FormName(), p.FileName(), p.Header.Get("Content-Type"), string(data)})
			}
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *mpServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func (s *mpServer) find(name string) (mpPart, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.part {
		if p.Name == name {
			return p, true
		}
	}
	return mpPart{}, false
}

// mpSetup writes site "up" with a multipart op "upload", a json op "make" and
// returns the server and a local file path.
func mpSetup(t *testing.T) (*mpServer, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	s := mpNewServer(t)
	def := "operations:\n  upload:\n    post: /up\n    body: {type: multipart}\n  make:\n    post: /make\n    body: {type: json}\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "up-def.yaml"), []byte(def), 0o600))
	cfg := fmt.Sprintf("version: 1\nsites:\n  up:\n    source: {type: simple, file: %q}\n    default_env: test\n    envs:\n      test: {base_url: %q}\n", filepath.Join(dir, "up-def.yaml"), s.srv.URL)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600))
	file := filepath.Join(dir, "a.txt")
	assert.NoError(t, os.WriteFile(file, []byte("file-content"), 0o600))
	return s, file
}

func TestMultipart(t *testing.T) {
	s, file := mpSetup(t)

	t.Run("-F text and file parts", func(t *testing.T) {
		code, _, errOut := runIO(t, "up", "upload", "-F", "title=hello", "-F", "doc=@"+file+";type=text/plain;filename=x.txt")
		assert.Eq(t, 0, code, errOut)
		mt, _, err := mime.ParseMediaType(s.ct)
		assert.NoError(t, err)
		assert.Eq(t, "multipart/form-data", mt)
		p, ok := s.find("title")
		assert.True(t, ok)
		assert.Eq(t, "hello", p.Data)
		assert.Eq(t, "", p.Filename)
		p, ok = s.find("doc")
		assert.True(t, ok)
		assert.Eq(t, "file-content", p.Data)
		assert.Eq(t, "x.txt", p.Filename)
		assert.Eq(t, "text/plain", p.Type)
	})

	t.Run("-F after the site name and before the op, like -f", func(t *testing.T) {
		code, _, errOut := runIO(t, "up", "-F", "a=b", "upload")
		assert.Eq(t, 0, code, errOut)
		p, ok := s.find("a")
		assert.True(t, ok)
		assert.Eq(t, "b", p.Data)
	})

	t.Run("-F before the site name is unknown, same as -f", func(t *testing.T) {
		code, _, _ := runIO(t, "-F", "a=b", "up", "upload")
		assert.Eq(t, 2, code)
	})

	t.Run("-f on multipart sends text parts, @ is literal", func(t *testing.T) {
		code, _, errOut := runIO(t, "up", "upload", "-f", "k=@/etc/passwd", "-f", "n:=5")
		assert.Eq(t, 0, code, errOut)
		p, ok := s.find("k")
		assert.True(t, ok)
		assert.Eq(t, "@/etc/passwd", p.Data)
		assert.Eq(t, "", p.Filename)
		p, ok = s.find("n")
		assert.True(t, ok)
		assert.Eq(t, "5", p.Data)
	})

	t.Run("-f name with CR/LF exits 2", func(t *testing.T) {
		before := s.count()
		code, _, _ := runIO(t, "up", "upload", "-f", "a\nb=1")
		assert.Eq(t, 2, code)
		assert.Eq(t, before, s.count())
	})

	t.Run("combination errors exit 2 and send nothing", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
		}{
			{"-F with -d", []string{"up", "upload", "-F", "a=b", "-d", "x"}},
			{"-F with -f", []string{"up", "upload", "-F", "a=b", "-f", "c=d"}},
			{"-F with -H Content-Type", []string{"up", "upload", "-F", "a=b", "-H", "Content-Type: text/plain"}},
			{"-F with -p header.Content-Type", []string{"up", "upload", "-F", "a=b", "-p", "header.Content-Type=text/plain"}},
			{"-F on json op", []string{"up", "make", "-F", "a=b"}},
			{"-d on multipart without Content-Type", []string{"up", "upload", "-d", "raw"}},
			{"invalid -F value", []string{"up", "upload", "-F", "novalue"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				before := s.count()
				code, _, _ := runIO(t, c.args...)
				assert.Eq(t, 2, code)
				assert.Eq(t, before, s.count())
			})
		}
	})

	t.Run("-d on multipart with Content-Type is sent as-is", func(t *testing.T) {
		code, _, errOut := runIO(t, "up", "upload", "-d", "raw-body", "-H", "Content-Type: multipart/form-data; boundary=zz")
		assert.Eq(t, 0, code, errOut)
		assert.Eq(t, "raw-body", s.raw)
		assert.Eq(t, "multipart/form-data; boundary=zz", s.ct)
	})

	t.Run("raw path call with -F sends multipart", func(t *testing.T) {
		code, _, errOut := runIO(t, "up", "-X", "POST", "/up", "-F", "a=b")
		assert.Eq(t, 0, code, errOut)
		p, ok := s.find("a")
		assert.True(t, ok)
		assert.Eq(t, "b", p.Data)
	})

	t.Run("-F without a path exits 2", func(t *testing.T) {
		code, _, _ := runIO(t, "up", "-F", "a=b")
		assert.Eq(t, 2, code)
	})

	t.Run("dry-run prints curl and sends nothing", func(t *testing.T) {
		before := s.count()
		code, out, errOut := runIO(t, "up", "upload", "--dry-run", "-F", "title=hello", "-F", "doc=@"+file)
		assert.Eq(t, 0, code, errOut)
		assert.Contains(t, out+errOut, "--form-string")
		assert.Contains(t, out+errOut, "-F")
		assert.Eq(t, before, s.count())
	})
}

func TestCompletionForm(t *testing.T) {
	mpSetup(t)
	code, out, errOut := runIO(t, "--in-completion", "up", "upload", "-")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "-F")
}

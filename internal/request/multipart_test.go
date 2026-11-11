package request

import (
	"bytes"
	"context"
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
	"github.com/inhere/apic/internal/model"
)

const sentinel = "SENTINEL-FILE-CONTENT-XYZ"

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	assert.NoErr(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func mpOp(required bool) *model.Operation {
	return &model.Operation{Method: "post", Path: "/up", Body: &model.BodySpec{Type: "multipart", Required: required}}
}

func TestParsePart(t *testing.T) {
	t.Run("valid forms", func(t *testing.T) {
		p, err := ParsePart("k=v;x=y")
		assert.NoErr(t, err)
		assert.Eq(t, Part{Name: "k", Value: "v;x=y"}, p)
		// '<' and '@' inside the value are plain text
		p, err = ParsePart("k=<file")
		assert.NoErr(t, err)
		assert.Eq(t, "<file", p.Value)
		p, err = ParsePart("k=a=b@c")
		assert.NoErr(t, err)
		assert.Eq(t, "a=b@c", p.Value)
		p, err = ParsePart("k=")
		assert.NoErr(t, err)
		assert.Eq(t, "", p.Value)

		p, err = ParsePart("file=@./dir/a.XLSX")
		assert.NoErr(t, err)
		assert.Eq(t, Part{Name: "file", File: "./dir/a.XLSX", Filename: "a.XLSX",
			Type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}, p)

		p, err = ParsePart("file=@./a.bin;type=application/zip;filename=report.bin")
		assert.NoErr(t, err)
		assert.Eq(t, "application/zip", p.Type)
		assert.Eq(t, "report.bin", p.Filename)

		p, err = ParsePart(`file=@"./a;b.bin";type=text/plain`)
		assert.NoErr(t, err)
		assert.Eq(t, "./a;b.bin", p.File)
		assert.Eq(t, "a;b.bin", p.Filename)
		assert.Eq(t, "text/plain", p.Type)

		p, err = ParsePart(`file=@"./q\"x\\y.bin";filename="a;b.txt"`)
		assert.NoErr(t, err)
		assert.Eq(t, `./q"x\y.bin`, p.File)
		assert.Eq(t, "a;b.txt", p.Filename)
	})
	t.Run("type inference", func(t *testing.T) {
		cases := map[string]string{
			"a.xls": "application/vnd.ms-excel", "a.CSV": "text/csv", "a.json": "application/json",
			"a.txt": "text/plain", "a.xml": "application/xml", "a.zip": "application/zip",
			"a.gz": "application/gzip", "a.pdf": "application/pdf", "a.png": "image/png",
			"a.jpg": "image/jpeg", "a.JPEG": "image/jpeg", "a.gif": "image/gif", "a.webp": "image/webp",
			"a.doc":  "application/msword",
			"a.docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"a.bin":  "application/octet-stream", "noext": "application/octet-stream",
		}
		for f, want := range cases {
			p, err := ParsePart("f=@" + f)
			assert.NoErr(t, err, f)
			assert.Eq(t, want, p.Type, f)
		}
	})
	t.Run("exit 2 cases", func(t *testing.T) {
		bad := []string{
			"novalue", "=v", "=@f", "f=@", `f=@""`, "f=@-", "f=@-;type=a/b",
			"f=@a.txt;foo=bar", "f=@a.txt;", "f=@a.txt;type", "f=@a.txt;type=a/b;type=c/d",
			`f=@"a.txt`, `f=@"a.txt"x`, `f=@a.txt;filename="x`, "f=@a.txt;filename=",
			"f=@a.txt;type=text", "f=@a.txt;type=a/b;charset=x",
			`f=@a.txt;type=text/plain;x="a,secret.txt;y"`,
			"f=@a.txt;type=a/b,c", "f=@a.txt;type=a/b c", "f=@a.txt;type=a/b\r\nX: y", `f=@a.txt;type="a/b"`,
			`f=@a.txt;type=text/plain;filename=a"b`, `f=@a.txt;filename=a\b`, "f=@a.txt;filename=a\nb",
			"f\nx=v", `f"x=v`, `f\x=v`, "f\x00=v", "f\nx=@a.txt", `f"x=@a.txt`,
			"f=@a\nb.txt", "f=@a\x00b.txt", "f=@a\x7fb.txt",
			`f=@"x\"y"z`,
		}
		for _, s := range bad {
			_, err := ParsePart(s)
			assert.Eq(t, model.ExitUsage, code(err), s)
		}
		// hint texts
		_, err := ParsePart("f=@-")
		assert.Contains(t, err.Error(), "stdin is not supported; use ./-")
		// default filename taken from path that is not valid: needs explicit filename
		_, err = ParsePart(`f=@"dir/a\"b.txt"`)
		assert.Eq(t, model.ExitUsage, code(err))
		assert.Contains(t, err.Error(), ";filename=")
		p, err := ParsePart(`f=@"dir/a\"b.txt";filename=ok.txt`)
		assert.NoErr(t, err)
		assert.Eq(t, "ok.txt", p.Filename)
		// ./- is a normal path
		p, err = ParsePart("f=@./-")
		assert.NoErr(t, err)
		assert.Eq(t, "./-", p.File)
	})
	t.Run("TextPart", func(t *testing.T) {
		p, err := TextPart("a.b", "@not-a-file;type=x")
		assert.NoErr(t, err)
		assert.Eq(t, Part{Name: "a.b", Value: "@not-a-file;type=x"}, p)
		for _, n := range []string{"", "a\nb", "a\rb: x", `a"b`, `a\b`, "a\x00"} {
			_, err := TextPart(n, "v")
			assert.Eq(t, model.ExitUsage, code(err), n)
		}
	})
}

func TestAssemble(t *testing.T) {
	t.Run("raw Content-Disposition bytes", func(t *testing.T) {
		f := writeTemp(t, "a.csv", "x,y\n")
		parts := []Part{{Name: "t", Value: "hello"}, {Name: "file", File: f, Filename: "my.csv", Type: "text/csv"}}
		body, ctype, err := Assemble(parts)
		assert.NoErr(t, err)
		s := string(body)
		assert.Contains(t, s, "Content-Disposition: form-data; name=\"t\"\r\n\r\nhello\r\n")
		assert.Contains(t, s, "Content-Disposition: form-data; name=\"file\"; filename=\"my.csv\"\r\nContent-Type: text/csv\r\n\r\nx,y\n\r\n")
		assert.Eq(t, int64(4), parts[1].Size)
		mt, params, err := mime.ParseMediaType(ctype)
		assert.NoErr(t, err)
		assert.Eq(t, "multipart/form-data", mt)
		assert.NotEmpty(t, params["boundary"])
	})
	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		_, _, err := Assemble([]Part{{Name: "f", File: dir, Filename: "d", Type: "a/b"}})
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Contains(t, err.Error(), "-F f: "+dir+" is not a regular file")
		assert.Eq(t, model.ExitLocal, code(CheckFiles([]Part{{Name: "f", File: dir}})))
		_, _, err = Assemble([]Part{{Name: "f", File: filepath.Join(dir, "missing"), Filename: "m", Type: "a/b"}})
		assert.Eq(t, model.ExitLocal, code(err))
	})
	t.Run("size cap", func(t *testing.T) {
		big := filepath.Join(t.TempDir(), "big.bin")
		fh, err := os.Create(big)
		assert.NoErr(t, err)
		assert.NoErr(t, fh.Truncate(MaxMultipartBytes+1))
		assert.NoErr(t, fh.Close())
		part := Part{Name: "f", File: big, Filename: "big.bin", Type: "application/octet-stream"}
		_, _, err = Assemble([]Part{part})
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Contains(t, err.Error(), "multipart body exceeds 64 MiB; use curl for large uploads")
		err = CheckFiles([]Part{part})
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Contains(t, err.Error(), "exceeds 64 MiB")
		// text parts count towards the sum
		small := Part{Name: "t", Value: strings.Repeat("a", 10)}
		assert.Eq(t, model.ExitLocal, code(CheckFiles([]Part{small, part})))
	})
}

type recorded struct {
	ctype string
	raw   []byte
	parts []recPart
}

type recPart struct{ name, filename, ctype, body string }

func mpServer(t *testing.T, status func(n int) int) (*httptest.Server, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var got []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := recorded{ctype: r.Header.Get("Content-Type"), raw: raw}
		if _, params, err := mime.ParseMediaType(rec.ctype); err == nil && params["boundary"] != "" {
			mr := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(p)
				rec.parts = append(rec.parts, recPart{p.FormName(), p.FileName(), p.Header.Get("Content-Type"), string(b)})
			}
		}
		mu.Lock()
		got = append(got, rec)
		n := len(got)
		mu.Unlock()
		w.WriteHeader(status(n))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recorded { mu.Lock(); defer mu.Unlock(); return append([]recorded(nil), got...) }
}

func mpBuild(t *testing.T, base string, in Input) (*Prepared, error) {
	t.Helper()
	in.Method = "POST"
	return Build(mpOp(false), in, model.Env{BaseURL: base}, "")
}

func TestMultipartExecute(t *testing.T) {
	t.Run("send assembles after protect and parses", func(t *testing.T) {
		srv, got := mpServer(t, func(int) int { return 200 })
		f := writeTemp(t, "a.txt", sentinel)
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"t=v;1", "file=@" + f}})
		assert.NoErr(t, err)
		assert.Empty(t, pr.Header.Get("Content-Type"))
		assert.Nil(t, pr.Body)
		resp, err := Execute(context.Background(), call(t, srv, pr, nil, nil))
		assert.NoErr(t, err)
		assert.Eq(t, 200, resp.Status)
		recs := got()
		assert.Eq(t, 1, len(recs))
		assert.Eq(t, []recPart{{"t", "", "", "v;1"}, {"file", "a.txt", "text/plain", sentinel}}, recs[0].parts)
		assert.Eq(t, int64(len(sentinel)), pr.Parts[1].Size)
	})
	t.Run("-f text parts with k:=json", func(t *testing.T) {
		srv, got := mpServer(t, func(int) int { return 200 })
		pr, err := mpBuild(t, srv.URL, Input{Fields: []string{"a=@x;type=y", `s:= "q" `, "n:= 3 ", `o:={"a": 1}`}})
		assert.NoErr(t, err)
		_, err = Execute(context.Background(), call(t, srv, pr, nil, nil))
		assert.NoErr(t, err)
		assert.Eq(t, []recPart{{"a", "", "", "@x;type=y"}, {"s", "", "", "q"}, {"n", "", "", "3"}, {"o", "", "", `{"a": 1}`}}, got()[0].parts)
	})
	t.Run("401 retry sends identical body", func(t *testing.T) {
		srv, got := mpServer(t, func(n int) int {
			if n == 1 {
				return 401
			}
			return 200
		})
		f := writeTemp(t, "a.txt", sentinel)
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"file=@" + f}})
		assert.NoErr(t, err)
		sec := &model.Secrets{}
		fa := &fakeAuth{secrets: sec, token: "tok-12345"}
		resp, err := Execute(context.Background(), call(t, srv, pr, fa, &model.Redactor{Secrets: sec}))
		assert.NoErr(t, err)
		assert.Eq(t, 200, resp.Status)
		recs := got()
		assert.Eq(t, 2, len(recs))
		assert.Eq(t, recs[0].raw, recs[1].raw)
		assert.Eq(t, recs[0].ctype, recs[1].ctype)
	})
	t.Run("protect refusal does not open files", func(t *testing.T) {
		t.Setenv("APIC_NO_PROMPT", "1")
		srv, got := mpServer(t, func(int) int { return 200 })
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"f=@" + filepath.Join(t.TempDir(), "nonexistent")}})
		assert.NoErr(t, err)
		c := call(t, srv, pr, nil, nil)
		c.Env.Protect = true
		_, err = Execute(context.Background(), c)
		assert.Eq(t, model.ExitProtected, code(err))
		assert.Eq(t, 0, len(got()))
		// without protection the same call fails on the open (exit 1), proving the file is checked there
		c.Env.Protect = false
		_, err = Execute(context.Background(), c)
		assert.Eq(t, model.ExitLocal, code(err))
	})
	t.Run("dry-run checks files and renders curl", func(t *testing.T) {
		srv, got := mpServer(t, func(int) int { return 200 })
		f := writeTemp(t, "a.csv", sentinel)
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"t=it's @x", "file=@" + f}})
		assert.NoErr(t, err)
		c := call(t, srv, pr, nil, nil)
		c.DryRun = true
		resp, err := Execute(context.Background(), c)
		assert.NoErr(t, err)
		assert.Eq(t, 0, len(got()))
		assert.Nil(t, pr.Body)
		assert.Contains(t, resp.Curl, `--form-string 't=it'\''s @x'`)
		assert.Contains(t, resp.Curl, `-F 'file=@"`+formQuote(f)+`";type=text/csv;filename="a.csv"'`) // Windows paths carry backslashes
		assert.NotContains(t, resp.Curl, "Content-Type")
		assert.NotContains(t, resp.Curl, "--data-raw")
		assert.NotContains(t, resp.Curl, sentinel)
		// dry-run reports the same exit 1 for a bad file
		pr2, _ := mpBuild(t, srv.URL, Input{Forms: []string{"f=@" + t.TempDir()}})
		c.Prepared = pr2
		_, err = Execute(context.Background(), c)
		assert.Eq(t, model.ExitLocal, code(err))
	})
	t.Run("size cap on send and dry-run", func(t *testing.T) {
		srv, got := mpServer(t, func(int) int { return 200 })
		big := filepath.Join(t.TempDir(), "big.bin")
		fh, err := os.Create(big)
		assert.NoErr(t, err)
		assert.NoErr(t, fh.Truncate(MaxMultipartBytes+1))
		assert.NoErr(t, fh.Close())
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"f=@" + big}})
		assert.NoErr(t, err)
		_, err = Execute(context.Background(), call(t, srv, pr, nil, nil))
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Eq(t, 0, len(got()))
		c := call(t, srv, pr, nil, nil)
		c.DryRun = true
		_, err = Execute(context.Background(), c)
		assert.Eq(t, model.ExitLocal, code(err))
	})
	t.Run("-v shows parts without content and masks secrets", func(t *testing.T) {
		srv, _ := mpServer(t, func(int) int { return 200 })
		f := writeTemp(t, "SECRETNAME99.txt", sentinel)
		pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"t=SECRETVAL77", "file=@" + f}})
		assert.NoErr(t, err)
		sec := &model.Secrets{}
		sec.Add("SECRETVAL77")
		sec.Add("SECRETNAME99")
		var v bytes.Buffer
		c := call(t, srv, pr, nil, &model.Redactor{Secrets: sec})
		c.Verbose = &v
		_, err = Execute(context.Background(), c)
		assert.NoErr(t, err)
		out := v.String()
		assert.Contains(t, out, "> part t: ***\n")
		assert.Contains(t, out, "> part file: file=***.txt type=text/plain size="+"25\n")
		assert.NotContains(t, out, sentinel)
		assert.NotContains(t, out, "SECRET")
		// curl masks value, path and filename too
		c.Verbose = nil
		c.DryRun = true
		pr.Parts[1].Size = 0
		resp, err := Execute(context.Background(), c)
		assert.NoErr(t, err)
		assert.NotContains(t, resp.Curl, "SECRET")
		assert.Contains(t, resp.Curl, "--form-string 't=***'")
	})
}

func TestCurlMultipart(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://h/up", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("X-A", "1")
	parts := []Part{
		{Name: "t", Value: "v"},
		{Name: "f", File: `/tmp/a"b\c.txt`, Filename: "n.txt", Type: "text/plain"},
	}
	got := Curl(req, []byte("ignored"), parts, model.Env{}, nil)
	assert.Eq(t, `curl -X POST -H 'X-A: 1' --form-string 't=v' -F 'f=@"/tmp/a\"b\\c.txt";type=text/plain;filename="n.txt"' 'http://h/up'`, got)
	// without parts the body and Content-Type are rendered as before
	got = Curl(req, []byte("b"), nil, model.Env{}, nil)
	assert.Contains(t, got, "Content-Type: multipart")
	assert.Contains(t, got, "--data-raw 'b'")
}

func TestMultipartBuildRules(t *testing.T) {
	env := model.Env{BaseURL: "http://h"}
	usageErr := func(op *model.Operation, in Input, want string) {
		t.Helper()
		_, err := Build(op, in, env, "")
		assert.Eq(t, model.ExitUsage, code(err), want)
		assert.Contains(t, err.Error(), want)
	}
	t.Run("combinations", func(t *testing.T) {
		usageErr(mpOp(false), Input{Forms: []string{"a=1"}, Data: "x"}, "-F cannot be combined with -d or -f")
		usageErr(mpOp(false), Input{Forms: []string{"a=1"}, Fields: []string{"b=2"}}, "-F cannot be combined with -d or -f")
		usageErr(nil, Input{Path: "/a", Forms: []string{"a=1"}, Data: "x"}, "-F cannot be combined with -d or -f")
		usageErr(&model.Operation{Path: "/a"}, Input{Forms: []string{"a=1"}}, "operation has no request body; add one with an overlay (body: multipart)")
		usageErr(&model.Operation{Path: "/a", Body: &model.BodySpec{Type: "form"}}, Input{Forms: []string{"a=1"}}, "operation body is form, not multipart; fix it with an overlay (body: multipart)")
		usageErr(&model.Operation{Path: "/a", Body: &model.BodySpec{}}, Input{Forms: []string{"a=1"}}, "operation body is json, not multipart")
		usageErr(mpOp(false), Input{Forms: []string{"bad"}}, "invalid -F")
		usageErr(mpOp(false), Input{Fields: []string{"a\nb=1"}}, "multipart field name")
	})
	t.Run("raw path -F is multipart", func(t *testing.T) {
		pr, err := Build(nil, Input{Method: "post", Path: "/a", Forms: []string{"a=1"}}, env, "")
		assert.NoErr(t, err)
		assert.Eq(t, []Part{{Name: "a", Value: "1"}}, pr.Parts)
		assert.Empty(t, pr.Header.Get("Content-Type"))
	})
	t.Run("Content-Type from any source conflicts with parts", func(t *testing.T) {
		for _, in := range []Input{
			{Forms: []string{"a=1"}, Headers: []string{"Content-Type: text/plain"}},
			{Forms: []string{"a=1"}, Headers: []string{"content-type: text/plain"}},
			{Fields: []string{"a=1"}, Headers: []string{"Content-Type: x/y"}},
			{Forms: []string{"a=1"}, P: []string{"header.Content-Type=text/plain"}},
		} {
			_, err := Build(mpOp(false), in, env, "")
			assert.Eq(t, model.ExitUsage, code(err), in)
		}
		op := mpOp(false)
		op.Params = []model.Param{{Name: "Content-Type", In: "header"}}
		_, err := Build(op, Input{Forms: []string{"a=1"}, P: []string{"Content-Type=x/y"}}, env, "")
		assert.Eq(t, model.ExitUsage, code(err))
	})
	t.Run("-d needs Content-Type on multipart operations", func(t *testing.T) {
		usageErr(mpOp(false), Input{Data: "raw"}, "multipart body with -d needs -H 'Content-Type: ...'; or use -F")
		pr, err := Build(mpOp(false), Input{Data: "raw", Headers: []string{"Content-Type: multipart/form-data; boundary=zz"}}, env, "")
		assert.NoErr(t, err)
		assert.Eq(t, "raw", string(pr.Body))
		assert.Eq(t, "multipart/form-data; boundary=zz", pr.Header.Get("Content-Type"))
		assert.Empty(t, pr.Parts)
		// header supplied through -p header.* also satisfies the check
		op := mpOp(false)
		op.Params = []model.Param{{Name: "Content-Type", In: "header"}}
		_, err = Build(op, Input{Data: "raw", P: []string{"header.Content-Type=a/b"}}, env, "")
		assert.NoErr(t, err)
	})
	t.Run("required body", func(t *testing.T) {
		_, err := Build(mpOp(true), Input{}, env, "")
		assert.Eq(t, model.ExitLocal, code(err))
		assert.Contains(t, err.Error(), "request body is required (use -F, -d or -f)")
		_, err = Build(mpOp(false), Input{}, env, "")
		assert.NoErr(t, err)
		_, err = Build(&model.Operation{Path: "/a", Body: &model.BodySpec{Type: "json", Required: true}}, Input{}, env, "")
		assert.Contains(t, err.Error(), "request body is required (use -d or -f)")
	})
	t.Run("k:=json conversion shared with form and not compacted", func(t *testing.T) {
		form := &model.Operation{Path: "/f", Body: &model.BodySpec{Type: "form"}}
		pr, err := Build(form, Input{Fields: []string{`s:= "a b" `, `o:= {"a": 1} `}}, env, "")
		assert.NoErr(t, err)
		assert.Eq(t, "o=%7B%22a%22%3A+1%7D&s=a+b", string(pr.Body))
		pr, err = Build(mpOp(false), Input{Fields: []string{`o:= {"a": 1} `}}, env, "")
		assert.NoErr(t, err)
		assert.Eq(t, `{"a": 1}`, pr.Parts[0].Value)
	})
}

func TestHiddenParam(t *testing.T) {
	env := model.Env{BaseURL: "http://h"}
	op := &model.Operation{Path: "/x", Params: []model.Param{
		{Name: "secret", In: "query", Hidden: true},
		{Name: "tok", In: "header", Hidden: true},
		{Name: "vis", In: "query"},
	}}
	for _, p := range []string{"secret=1", "query.secret=1", "tok=1", "header.tok=1"} {
		_, err := Build(op, Input{P: []string{p}}, env, "")
		assert.Eq(t, model.ExitUsage, code(err), p)
		assert.Contains(t, err.Error(), "is hidden by overlay", p)
	}
	_, err := Build(op, Input{P: []string{"vis=1", "query.vis=2"}}, env, "")
	assert.NoErr(t, err)
	// the same name in another location is not hidden
	op2 := &model.Operation{Path: "/x/{id}", Params: []model.Param{
		{Name: "id", In: "path"}, {Name: "id", In: "query", Hidden: true}}}
	_, err = Build(op2, Input{P: []string{"path.id=1"}}, env, "")
	assert.NoErr(t, err)
	_, err = Build(op2, Input{P: []string{"path.id=1", "query.id=2"}}, env, "")
	assert.Eq(t, model.ExitUsage, code(err))
}

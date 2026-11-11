package output_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/output"
)

func mustPick(t *testing.T, p string) *model.Pick {
	t.Helper()
	pk, err := model.ParsePick(p)
	assert.NoError(t, err)
	return &pk
}

func render(t *testing.T, status int, body string, opt output.RenderOptions) (code int, err error, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	res := &output.Result{Status: status, Header: http.Header{}, Body: []byte(body)}
	code, err = output.Render(res, opt, &o, &e, &model.Redactor{Secrets: &model.Secrets{}})
	return code, err, o.String(), e.String()
}

func TestRenderPick(t *testing.T) {
	const body = `{"id":12345678901234567891,"name":"a<b>&","tags":["x","z"],"u":{"k":1}}`
	for _, tc := range []struct {
		name, pick, mode string
		tty              bool
		want             string
	}{
		{"auto string bare", "name", "auto", false, "a<b>&\n"},
		{"raw string bare", "name", "raw", false, "a<b>&\n"},
		{"json string quoted", "name", "json", false, "\"a<b>&\"\n"},
		{"auto big number", "id", "auto", false, "12345678901234567891\n"},
		{"auto non tty compact", "u", "auto", false, "{\"k\":1}\n"},
		{"auto tty indent", "u", "auto", true, "{\n  \"k\": 1\n}\n"},
		{"raw tty compact", "u", "raw", true, "{\"k\":1}\n"},
		{"json indent", "u", "json", false, "{\n  \"k\": 1\n}\n"},
		{"yaml string", "name", "yaml", false, "a<b>&\n"},
		{"yaml array", "tags", "yaml", false, "- x\n- z\n"},
		{"star", "tags.*", "auto", false, "[\"x\",\"z\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err, out, _ := render(t, 200, body, output.RenderOptions{Mode: tc.mode, Pick: mustPick(t, tc.pick), TTY: tc.tty})
			assert.NoError(t, err)
			assert.Eq(t, 0, code)
			assert.Eq(t, tc.want, out)
		})
	}
	t.Run("table scalar falls back to json with note", func(t *testing.T) {
		code, err, out, errOut := render(t, 200, body, output.RenderOptions{Mode: "table", Pick: mustPick(t, "name")})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Eq(t, "\"a<b>&\"\n", out)
		assert.Contains(t, errOut, "no table form")
	})
	t.Run("table of picked object", func(t *testing.T) {
		code, err, out, errOut := render(t, 200, body, output.RenderOptions{Mode: "table", Pick: mustPick(t, "u")})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "k")
		assert.Eq(t, "", errOut)
	})
}

func TestRenderYAMLTable(t *testing.T) {
	t.Run("yaml no pick", func(t *testing.T) {
		code, err, out, _ := render(t, 201, `{"b":12345678901234567891,"a":1}`, output.RenderOptions{Mode: "yaml"})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Eq(t, "b: 12345678901234567891\na: 1\n", out)
	})
	t.Run("table no pick", func(t *testing.T) {
		code, err, out, _ := render(t, 200, `[{"id":1},{"id":2}]`, output.RenderOptions{Mode: "table"})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Contains(t, out, "id")
	})
}

func TestRenderPostProcessFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		opt    output.RenderOptions
		want   string
	}{
		{"pick non json", 200, "plain text", output.RenderOptions{Mode: "auto", Pick: mustPick(t, "a")}, "not JSON"},
		{"pick empty body", 204, "", output.RenderOptions{Mode: "auto", Pick: mustPick(t, "a")}, "empty"},
		{"pick missing path", 201, `{"a":1}`, output.RenderOptions{Mode: "json", Pick: mustPick(t, "b")}, `"b"`},
		{"yaml non json", 200, "plain", output.RenderOptions{Mode: "yaml"}, "not JSON"},
		{"yaml empty", 204, "", output.RenderOptions{Mode: "yaml"}, "empty"},
		{"table non json", 200, "<html>", output.RenderOptions{Mode: "table"}, "not JSON"},
		{"table empty", 204, "", output.RenderOptions{Mode: "table"}, "empty"},
		{"trailing content", 200, `{"a":1} junk`, output.RenderOptions{Mode: "yaml"}, "not JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err, out, _ := render(t, tc.status, tc.body, tc.opt)
			assert.Err(t, err)
			assert.Eq(t, model.ExitPostProcess, code)
			assert.Eq(t, 0, len(out))
			assert.Eq(t, model.ExitPostProcess, err.(*model.CodedError).Code())
			assert.Contains(t, err.Error(), "succeeded; ")
			assert.Contains(t, err.Error(), tc.want)
		})
	}
	t.Run("message carries status", func(t *testing.T) {
		_, err, _, _ := render(t, 201, `{}`, output.RenderOptions{Mode: "auto", Pick: mustPick(t, "x")})
		assert.Contains(t, err.Error(), "HTTP 201 succeeded; ")
	})
}

func TestRenderStatusHandling(t *testing.T) {
	t.Run("3xx passthrough with warning", func(t *testing.T) {
		code, err, out, errOut := render(t, 302, `{"a":1}`, output.RenderOptions{Mode: "auto", Pick: mustPick(t, "a")})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Eq(t, `{"a":1}`, out)
		assert.Contains(t, errOut, "warning")
		_, _, _, errOut = render(t, 301, `x`, output.RenderOptions{Mode: "yaml"})
		assert.Contains(t, errOut, "warning")
		_, _, _, errOut = render(t, 301, `x`, output.RenderOptions{Mode: "raw"})
		assert.Eq(t, "", errOut)
	})
	t.Run("4xx 5xx passthrough", func(t *testing.T) {
		for _, tc := range []struct{ status, want int }{{404, 4}, {500, 5}} {
			code, err, out, errOut := render(t, tc.status, `{"a":1}`, output.RenderOptions{Mode: "table", Pick: mustPick(t, "a")})
			assert.NoError(t, err)
			assert.Eq(t, tc.want, code)
			assert.Eq(t, `{"a":1}`, out)
			assert.Eq(t, "", errOut)
		}
	})
	t.Run("dry-run unaffected", func(t *testing.T) {
		var o, e bytes.Buffer
		code, err := output.Render(&output.Result{Curl: "curl 'x'"}, output.RenderOptions{Mode: "yaml", Pick: mustPick(t, "a")}, &o, &e, &model.Redactor{Secrets: &model.Secrets{}})
		assert.NoError(t, err)
		assert.Eq(t, 0, code)
		assert.Eq(t, "curl 'x'\n", o.String())
	})
	t.Run("without pick output equals Format byte for byte", func(t *testing.T) {
		for _, mode := range []string{"auto", "json", "raw"} {
			for _, tty := range []bool{false, true} {
				for _, body := range []string{"not json", "", "  {\"a\" : 1}\n", `{"a":1}`} {
					for _, status := range []int{200, 204, 201} {
						var want bytes.Buffer
						assert.NoError(t, output.Format(&want, []byte(body), "", mode, tty))
						code, err, out, errOut := render(t, status, body, output.RenderOptions{Mode: mode, TTY: tty})
						assert.NoError(t, err)
						assert.Eq(t, 0, code)
						assert.Eq(t, want.String(), out)
						assert.Eq(t, "", errOut)
					}
				}
			}
		}
	})
}

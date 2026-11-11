package output_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/output"
)

func TestRender(t *testing.T) {
	secrets := &model.Secrets{}
	secrets.Add("secret-value")
	red := &model.Redactor{Secrets: secrets}
	for _, tc := range []struct {
		status, want int
	}{{200, 0}, {204, 0}, {302, 0}, {404, 4}, {499, 4}, {500, 5}, {503, 5}} {
		var out, errOut bytes.Buffer
		res := &output.Result{Status: tc.status, Header: http.Header{"Set-Cookie": {"sid=secret-value"}, "X-Note": {"has secret-value"}}, Body: []byte(`{"a":1}`)}
		code, err := output.Render(res, output.RenderOptions{Mode: "raw", IncludeHeaders: true}, &out, &errOut, red)
		assert.NoError(t, err)
		assert.Eq(t, tc.want, code)
		assert.Eq(t, `{"a":1}`, out.String())
		assert.Contains(t, errOut.String(), "Set-Cookie: ***")
		assert.NotContains(t, errOut.String(), "secret-value")
	}
	var out, errOut bytes.Buffer
	res := &output.Result{Status: 200, Body: []byte(`{"a":1}`)}
	code, err := output.Render(res, output.RenderOptions{Mode: "raw"}, &out, &errOut, red)
	assert.NoError(t, err)
	assert.Eq(t, 0, code)
	assert.Eq(t, "", errOut.String())
	out.Reset()
	code, err = output.Render(&output.Result{Curl: "curl 'x'"}, output.RenderOptions{Mode: "raw", IncludeHeaders: true}, &out, &errOut, red)
	assert.NoError(t, err)
	assert.Eq(t, 0, code)
	assert.Eq(t, "curl 'x'\n", out.String())
	assert.Eq(t, "", errOut.String())
}

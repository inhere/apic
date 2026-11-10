package output_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/output"
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
		assert.Eq(t, tc.want, output.Render(res, "raw", &out, &errOut, red, true, false))
		assert.Eq(t, `{"a":1}`, out.String())
		assert.Contains(t, errOut.String(), "Set-Cookie: ***")
		assert.NotContains(t, errOut.String(), "secret-value")
	}
	var out, errOut bytes.Buffer
	res := &output.Result{Status: 200, Body: []byte(`{"a":1}`)}
	assert.Eq(t, 0, output.Render(res, "raw", &out, &errOut, red, false, false))
	assert.Eq(t, "", errOut.String())
	out.Reset()
	assert.Eq(t, 0, output.Render(&output.Result{Curl: "curl 'x'"}, "raw", &out, &errOut, red, true, false))
	assert.Eq(t, "curl 'x'\n", out.String())
	assert.Eq(t, "", errOut.String())
}

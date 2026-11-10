package model_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func TestSecretsAndRedactor(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	var secrets model.Secrets
	secrets.Add("")
	secrets.Add("abc")
	secrets.Add("long secret+value")
	redactor := &model.Redactor{Secrets: &secrets}
	for _, tc := range []struct{ name, input, want string }{
		{"plaintext", "token=long secret+value", "token=***"},
		{"query escaped", "?token=" + url.QueryEscape("long secret+value"), "?token=***"},
		{"query percent space", "?token=long%20secret%2Bvalue", "?token=***"},
		{"short ignored", "abc", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Eq(t, tc.want, redactor.Redact(tc.input)) })
	}
}

func TestRedactHeadersCopies(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	var secrets model.Secrets
	secrets.Add("demo-secret")
	header := http.Header{"Authorization": {"Bearer abc"}, "Cookie": {"sid=abc"}, "X-Demo": {"demo-secret", "ordinary"}}
	out := (&model.Redactor{Secrets: &secrets}).RedactHeaders(header)
	assert.Eq(t, "***", out.Get("Authorization"))
	assert.Eq(t, "***", out.Get("Cookie"))
	assert.Eq(t, "***", out.Values("X-Demo")[0])
	assert.Eq(t, "ordinary", out.Values("X-Demo")[1])
	out["X-Demo"][1] = "mutated"
	assert.Eq(t, "demo-secret", header.Values("X-Demo")[0])
	assert.Eq(t, "ordinary", header.Values("X-Demo")[1])
}

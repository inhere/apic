package bundle_test

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/bundle"
	"github.com/inhere/apic/internal/model"
)

const siteText = "kind: site\nname: demo\nsite:\n  source: {type: simple}\n  envs:\n    default: {base_url: \"https://a.example.com\"}\n  auths:\n    ci: {type: bearer, token: \"${env:TOK}\"}\n"

func TestEncodeDecode(t *testing.T) {
	site := &model.Site{Source: model.Source{Type: "simple"}, Envs: map[string]model.Env{"default": {BaseURL: "https://a.example.com"}}, Origin: "/secret/path"}
	def := []byte("\xef\xbb\xbf---\r\n# keep me\r\noperations:\r\n  a: {get: /a}")
	data, err := bundle.Encode("demo", site, def, []byte("operations: {}\n"))
	assert.NoError(t, err)
	text := string(data)
	assert.NotContains(t, text, "/secret/path")
	assert.NotContains(t, text, "\r")
	assert.Contains(t, text, "---\nkind: definition\n\n# keep me\noperations:\n  a: {get: /a}\n---\nkind: overlay\n")
	b, err := bundle.Decode(data)
	assert.NoError(t, err)
	assert.Eq(t, "demo", b.Name)
	assert.Eq(t, "simple", b.Site.Source.Type)
	assert.Contains(t, string(b.Definition), "# keep me\noperations:")
	assert.NotContains(t, string(b.Definition), "kind:")
	assert.Eq(t, "operations: {}\n", string(b.Overlay))

	_, err = bundle.Encode("demo", site, []byte("a: 1\n---\nb: 2\n"), nil)
	assert.Err(t, err)
}

func TestDecodeErrors(t *testing.T) {
	for name, text := range map[string]string{
		"empty":          "",
		"no site":        "kind: definition\noperations: {}\n",
		"two defs":       siteText + "---\nkind: definition\na: 1\n---\nkind: definition\na: 1\n",
		"two overlays":   siteText + "---\nkind: overlay\na: 1\n---\nkind: overlay\na: 1\n",
		"unknown kind":   siteText + "---\nkind: x\n",
		"kind not first": "operations: {}\nkind: site\n",
		"unknown key":    "kind: site\nname: x\nextra: 1\nsite: {source: {type: simple}}\n",
	} {
		_, err := bundle.Decode([]byte(text))
		assert.Err(t, err, name)
	}
	b, err := bundle.Decode([]byte(siteText + "---\n"))
	assert.NoError(t, err)
	assert.Nil(t, b.Definition)
}

func TestBuild(t *testing.T) {
	b, err := bundle.Decode([]byte(siteText + "---\nkind: definition\noperations:\n  a: {get: /a}\n"))
	assert.NoError(t, err)
	p, err := bundle.Build(b, "other")
	assert.NoError(t, err)
	assert.Eq(t, "other", p.Name)
	assert.Eq(t, "sites/other.yaml", p.Files[0].Rel)
	assert.Contains(t, string(p.Files[0].Data), "file: defs/other.yaml")
	assert.Eq(t, "defs/other.yaml", p.Files[1].Rel)
	assert.Empty(t, p.Execs)
	_, err = bundle.Build(b, "Bad Name")
	assert.Err(t, err)
	b.Definition = nil
	_, err = bundle.Build(b, "")
	assert.Err(t, err)
	assert.True(t, strings.Contains(err.Error(), "definition"))
}

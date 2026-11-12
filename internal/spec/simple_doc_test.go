package spec

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/skillgen"
)

// simpleDoc is the rendered apic skill, shared by the drift checks below.
func simpleDoc(t *testing.T) string {
	t.Helper()
	data, err := skillgen.RenderSelf("apic", "v1.0.0")
	assert.Require(t, assert.NoError(t, err))
	return string(data)
}

// yamlTags returns the yaml names of the struct's fields.
func yamlTags(t *testing.T, v any) []string {
	t.Helper()
	typ := reflect.TypeOf(v)
	assert.Require(t, assert.True(t, typ.Kind() == reflect.Struct))
	tags := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		assert.Require(t, assert.True(t, tag != "", typ.Field(i).Name+" has no yaml tag"))
		tags = append(tags, strings.Split(tag, ",")[0])
	}
	return tags
}

// TestSimpleDocFields fails when a simple-definition field is added and the
// apic skill template is not updated.
func TestSimpleDocFields(t *testing.T) {
	text := simpleDoc(t)
	fields := yamlTags(t, simpleOperation{})
	fields = append(fields, yamlTags(t, simpleParam{})...)
	for _, name := range fields {
		assert.Contains(t, text, "`"+name+"`", "field "+name+" is missing from the skill template")
	}
	for _, name := range yamlTags(t, simpleBody{}) {
		assert.Contains(t, text, "body."+name, "field body."+name+" is missing from the skill template")
	}
	// The loader's document type is anonymous, so the top-level key is checked
	// by loading the example: it only parses with an `operations` key.
	assert.Contains(t, text, "\noperations:\n")
}

var (
	simpleBlockStart = "<!-- simple-definition-example -->\n```yaml\n"
	simpleOpName     = regexp.MustCompile(`(?m)^  [a-z0-9][a-z0-9-]*:$`)
)

// TestSimpleDocExample loads the example embedded in the skill template with
// the real loader, so the documentation cannot drift from the implementation.
func TestSimpleDocExample(t *testing.T) {
	text := simpleDoc(t)
	start := strings.Index(text, simpleBlockStart)
	assert.Require(t, assert.True(t, start >= 0), "the example marker is missing")
	rest := text[start+len(simpleBlockStart):]
	end := strings.Index(rest, "\n```\n")
	assert.Require(t, assert.True(t, end >= 0), "the example yaml block is not closed")
	yamlText := rest[:end]

	dir := t.TempDir()
	file := "pet.yaml"
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(yamlText), 0600)))

	index, err := (SimpleLoader{}).Load(context.Background(), model.Source{File: file}, dir)
	assert.Require(t, assert.NoError(t, err))
	want := len(simpleOpName.FindAllString(yamlText, -1))
	assert.True(t, want > 0)
	assert.Eq(t, want, len(index.Ops))
	assert.Eq(t, 4, len(index.Ops))

	methods := map[string]string{}
	for _, op := range index.Ops {
		methods[op.Name] = op.Method
	}
	assert.Eq(t, "GET", methods["pet-list"])
	assert.Eq(t, "GET", methods["pet-get"])
	assert.Eq(t, "POST", methods["pet-create"])
	assert.Eq(t, "GET", methods["pet-search"])
}

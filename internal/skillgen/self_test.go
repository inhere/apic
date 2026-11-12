package skillgen_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/skillgen"
)

func TestRenderSelf(t *testing.T) {
	data, err := skillgen.RenderSelf("apic", "v1.2.3")
	assert.Require(t, assert.NoError(t, err))
	text := string(data)

	t.Run("frontmatter and marker", func(t *testing.T) {
		fm, rest := split(t, data)
		assert.Eq(t, "apic", fm["name"])
		assert.True(t, strings.HasPrefix(fm["description"], "Use the apic CLI to call HTTP APIs"))
		assert.True(t, len([]rune(fm["description"])) <= 200)
		assert.Eq(t, skillgen.Marker, rest[0])
	})

	t.Run("version is filled in", func(t *testing.T) {
		assert.Contains(t, text, "Version: v1.2.3\n")
		assert.NotContains(t, text, "{{version}}")
	})

	t.Run("sections follow the design order", func(t *testing.T) {
		heads := []string{
			"## When to use it\n",
			"## Config directory and files\n",
			"## Add a site\n",
			"## Simple definitions\n",
			"## Site configuration\n",
			"## Calling operations\n",
			"## Output\n",
			"## Exit codes\n",
			"## Generate a site skill\n",
		}
		last := -1
		for _, h := range heads {
			at := strings.Index(text, h)
			assert.Require(t, assert.True(t, at > last, "section "+h+" is missing or out of order"))
			last = at
		}
		assert.Contains(t, text, "<!-- simple-definition-example -->")
	})

	t.Run("placeholder hosts only", func(t *testing.T) {
		for _, s := range []string{"internal.example", "localhost", "192.168", `C:\`, "/Users/", "/home/", "~/.config/apic/skills/apic-pet"} {
			assert.NotContains(t, text, s)
		}
		assert.Contains(t, text, "example.com")
	})

	t.Run("safety notes", func(t *testing.T) {
		assert.Contains(t, text, "`-F name=@path` and `-d @file` read a local file: use them only with paths the")
		assert.Contains(t, text, "use `--yes` only when the\n  user approved the write")
		assert.Contains(t, text, "| 7 | request succeeded but --pick/-o post-processing failed - do NOT retry the request |")
	})
}

func TestRenderSelfDefaults(t *testing.T) {
	t.Run("empty version becomes dev", func(t *testing.T) {
		data, err := skillgen.RenderSelf("apic", "")
		assert.Require(t, assert.NoError(t, err))
		assert.Contains(t, string(data), "Version: dev\n")
	})

	t.Run("invalid name exits 2", func(t *testing.T) {
		_, err := skillgen.RenderSelf("../x", "v1")
		assert.Require(t, assert.Error(t, err))
		var coded *model.CodedError
		assert.Require(t, assert.True(t, errors.As(err, &coded)))
		assert.Eq(t, model.ExitUsage, coded.C)
	})
}

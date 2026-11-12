package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
)

// paginateYAML turns a paginate block written at zero indentation into text
// that can sit below a site key; an empty block yields no key at all.
func paginateYAML(block string) string {
	if block == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("paginate:\n")
	for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
		b.WriteString("  " + strings.TrimSpace(line) + "\n")
	}
	return b.String()
}

// indentYAML prefixes every line, to nest a site block below sites.<name>.
func indentYAML(text, prefix string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

// loadPaginate writes one site named pet with the given paginate block, either
// into config.yaml or into sites/pet.yaml, and loads the directory.
func loadPaginate(t *testing.T, from, block string) (*model.Config, error) {
	t.Helper()
	dir := testDir(t)
	body := paginateYAML(block)
	if from == "config.yaml" {
		putFile(t, filepath.Join(dir, "config.yaml"),
			"sites:\n  pet:\n    source: {type: simple}\n"+indentYAML(body, "    "))
	} else {
		putFile(t, filepath.Join(dir, "sites", "pet.yaml"), "source: {type: simple}\n"+body)
	}
	return config.Load(dir)
}

func TestPaginateConfig(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		for _, tc := range []struct {
			name, block, match string
		}{
			{"missing page", "size: per_page", "paginate: page is required"},
			{"size_value zero", "page: page\nsize: per_page\nsize_value: 0", "paginate: size_value must be a positive integer"},
			{"size_value negative", "page: page\nsize: per_page\nsize_value: -5", "paginate: size_value must be a positive integer"},
			{"size_value without size", "page: page\nsize_value: 50", "paginate: size_value requires size"},
			{"start negative", "page: page\nstart: -1", "paginate: start must not be negative"},
			{"items comma", "page: page\nitems: a,b", "paginate: items must be a plain path"},
			{"items group", "page: page\nitems: data.{list,meta}", "paginate: items must be a plain path"},
			{"items star", "page: page\nitems: data.*", "paginate: items must be a plain path"},
			{"total comma", "page: page\ntotal: a,b", "paginate: total must be a plain path"},
			{"total group", "page: page\ntotal: data.{list,total}", "paginate: total must be a plain path"},
			{"total star", "page: page\ntotal: data.*", "paginate: total must be a plain path"},
			{"size_value not an integer", "page: page\nsize: per_page\nsize_value: abc", ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for _, from := range []string{"config.yaml", "sites/pet.yaml"} {
					t.Run(from, func(t *testing.T) {
						_, err := loadPaginate(t, from, tc.block)
						assert.Require(t, assert.NotNil(t, err))
						if tc.match == "" {
							// A YAML type error keeps the loader's own exit code.
							localError(t, err, "pet")
							return
						}
						localError(t, err, `site "pet"`, tc.match)
					})
				}
			})
		}
	})

	t.Run("valid config.yaml", func(t *testing.T) {
		block := "page: page\nsize: per_page\nsize_value: 100\nstart: 0\nitems: data.list\ntotal: data.total"
		cfg, err := loadPaginate(t, "config.yaml", block)
		assert.Require(t, assert.NoErr(t, err))
		site := cfg.Sites["pet"]
		assert.Require(t, assert.NotNil(t, site))
		assert.Require(t, assert.NotNil(t, site.Paginate))
		p := site.Paginate
		assert.Eq(t, "page", p.Page)
		assert.Eq(t, "per_page", p.Size)
		assert.Require(t, assert.NotNil(t, p.SizeValue))
		assert.Eq(t, 100, *p.SizeValue)
		assert.Require(t, assert.NotNil(t, p.Start))
		assert.Eq(t, 0, *p.Start)
		assert.Eq(t, 0, p.StartPage())
		assert.Eq(t, "data.list", p.Items)
		assert.Eq(t, "data.total", p.Total)
	})

	t.Run("valid site file without start", func(t *testing.T) {
		block := "page: page\nsize: per_page\nsize_value: 20"
		cfg, err := loadPaginate(t, "sites/pet.yaml", block)
		assert.Require(t, assert.NoErr(t, err))
		p := cfg.Sites["pet"].Paginate
		assert.Require(t, assert.NotNil(t, p))
		assert.Eq(t, "page", p.Page)
		assert.Require(t, assert.NotNil(t, p.SizeValue))
		assert.Eq(t, 20, *p.SizeValue)
		assert.True(t, p.Start == nil)
		assert.Eq(t, 1, p.StartPage())
		assert.Eq(t, "", p.Items)
		assert.Eq(t, "", p.Total)
	})

	t.Run("valid minimal site file", func(t *testing.T) {
		cfg, err := loadPaginate(t, "sites/pet.yaml", "page: p")
		assert.Require(t, assert.NoErr(t, err))
		p := cfg.Sites["pet"].Paginate
		assert.Require(t, assert.NotNil(t, p))
		assert.Eq(t, "p", p.Page)
		assert.True(t, p.SizeValue == nil)
		assert.Eq(t, 1, p.StartPage())
	})

	t.Run("no paginate", func(t *testing.T) {
		cfg, err := loadPaginate(t, "config.yaml", "")
		assert.Require(t, assert.NoErr(t, err))
		assert.True(t, cfg.Sites["pet"].Paginate == nil)
	})
}

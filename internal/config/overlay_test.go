package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/config"
)

func TestLoadOverlay(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		got, err := config.LoadOverlay(dir, "pet")
		assert.NoErr(t, err)
		assert.Nil(t, got)
	})

	t.Run("loads values and replaces null entries", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "overlays", "pet.yaml")
		putFile(t, file, "operations:\n  GET /users:\n    name: users-list\n    summary: list users\n    body: multipart\n    output: yaml\n    params:\n      q:\n        in: query\n        required: true\n        type: integer\n        desc: page\n        enum: [1, 2]\n        hide: true\n      empty:\n      null-op:\n  GET /empty:\n")

		got, err := config.LoadOverlay(dir, "pet")
		assert.Require(t, assert.NoErr(t, err))
		assert.Eq(t, file, got.File)
		assert.Eq(t, "users-list", *got.Ops["GET /users"].Name)
		assert.Eq(t, "multipart", *got.Ops["GET /users"].Body)
		assert.Eq(t, "yaml", *got.Ops["GET /users"].Output)
		assert.True(t, got.Ops["GET /users"].Params["empty"] != nil)
		assert.True(t, got.Ops["GET /empty"] != nil)
		assert.True(t, got.Ops["GET /empty"].Params == nil)
		param := got.Ops["GET /users"].Params["q"]
		assert.Eq(t, "query", param.In)
		assert.Eq(t, "integer", *param.Type)
		assert.Eq(t, []string{"1", "2"}, param.Enum)
	})

	t.Run("rejects strict decode errors", func(t *testing.T) {
		for _, tc := range []struct {
			name, content string
		}{
			{"unknown top-level key", "file: wrong\n"},
			{"unknown operation key", "operations:\n  op:\n    unknown: true\n"},
			{"unknown parameter key", "operations:\n  op:\n    params:\n      p:\n        unknown: true\n"},
			{"duplicate top-level key", "operations: {}\noperations: {}\n"},
			{"duplicate operation key", "operations:\n  op: {}\n  op: {}\n"},
			{"duplicate parameter key", "operations:\n  op:\n    params:\n      p: {}\n      p: {}\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				putFile(t, filepath.Join(dir, "overlays", "pet.yaml"), tc.content)
				_, err := config.LoadOverlay(dir, "pet")
				localError(t, err)
			})
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tc := range []struct {
			name, content, reason string
		}{
			{"body", "operations:\n  op:\n    body: text\n", "body must be one of"},
			{"output", "operations:\n  op:\n    output: text\n", "output must be one of"},
			{"type", "operations:\n  op:\n    params:\n      p: {type: date}\n", "type must be one of"},
			{"in", "operations:\n  op:\n    params:\n      p: {in: cookie}\n", "in must be one of"},
			{"name syntax", "operations:\n  op:\n    name: bad_name\n", "name must match"},
			{"name help", "operations:\n  op:\n    name: help\n", "name must not be help"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				file := filepath.Join(dir, "overlays", "pet.yaml")
				putFile(t, file, tc.content)
				_, err := config.LoadOverlay(dir, "pet")
				localError(t, err, file, "op", tc.reason)
			})
		}
	})

	t.Run("decode error is one line", func(t *testing.T) {
		dir := t.TempDir()
		putFile(t, filepath.Join(dir, "overlays", "pet.yaml"), "operations: [\n")
		_, err := config.LoadOverlay(dir, "pet")
		localError(t, err)
		assert.False(t, strings.Contains(err.Error(), "\n"))
	})
}

func TestReservedOverlay(t *testing.T) {
	for _, tc := range []struct {
		name, main, siteFile string
	}{
		{
			name: "config site",
			main: "version: 1\nsites:\n  overlay: {source: {type: simple, file: defs/overlay.yaml}}\n  pet: {source: {type: simple, file: defs/pet.yaml}}\n",
		},
		{
			name:     "site file",
			main:     "version: 1\nsites:\n  pet: {source: {type: simple, file: defs/pet.yaml}}\n",
			siteFile: "source: {type: simple, file: defs/overlay.yaml}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			putFile(t, filepath.Join(dir, "config.yaml"), tc.main)
			if tc.siteFile != "" {
				putFile(t, filepath.Join(dir, "sites", "overlay.yaml"), tc.siteFile)
			}
			cfg, err := config.Load(dir)
			assert.Require(t, assert.NoErr(t, err))
			assert.True(t, cfg.Sites["overlay"] == nil)
			assert.True(t, cfg.Sites["pet"] != nil)
			assert.Eq(t, []string{`site "overlay" uses a reserved name and is skipped; rename it`}, cfg.Warnings)
		})
	}
}

func TestOverlayUsesAbsoluteFileInErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "overlays", "pet.yaml")
	putFile(t, file, "operations:\n  op:\n    output: invalid\n")
	_, err := config.LoadOverlay(dir, "pet")
	localError(t, err, file)
	_, statErr := os.Stat(file)
	assert.NoErr(t, statErr)
}

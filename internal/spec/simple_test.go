package spec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
)

func TestSimpleExample(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	data, err := os.ReadFile(filepath.Join("testdata", "jenkins.yaml"))
	assert.Require(t, assert.NoError(t, err))
	file := filepath.Join(dir, "demo.yaml")
	assert.Require(t, assert.NoError(t, os.WriteFile(file, data, 0600)))
	index, err := (spec.SimpleLoader{}).Load(context.Background(), model.Source{File: "demo.yaml"}, dir)
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, 4, len(index.Ops))
	ops := make(map[string]model.Operation)
	for _, op := range index.Ops {
		ops[op.Name] = op
	}
	assert.Eq(t, "GET", ops["job-info"].Method)
	assert.Eq(t, "/job/{name}/api/json", ops["job-info"].Path)
	assert.Eq(t, []model.Param{{Name: "name", In: "path", Required: true, Type: "string"}}, ops["job-info"].Params)
	assert.Eq(t, "POST", ops["job-build"].Method)
	assert.Eq(t, []model.Param{{Name: "BRANCH", In: "query", Type: "string"}, {Name: "ENV", In: "query", Type: "string"}, {Name: "name", In: "path", Required: true, Type: "string"}}, ops["job-build"].Params)
	assert.Eq(t, "raw", ops["job-log"].Output)
	assert.Eq(t, []model.Param{{Name: "name", In: "path", Required: true, Type: "string"}, {Name: "build", In: "path", Required: true, Type: "string"}}, ops["job-log"].Params)
	assert.Eq(t, []model.Param{{Name: "name", In: "query", Required: true, Desc: "新 job 名"}}, ops["create-item"].Params)
	assert.Require(t, assert.NotNil(t, ops["create-item"].Body))
	assert.Eq(t, "xml", ops["create-item"].Body.Type)
}

func TestSimpleVariants(t *testing.T) {
	cases := []struct {
		name, yaml string
		wantErr    bool
	}{
		{"declared-path", "operations:\n  demo:\n    put: /{id}/{id}\n    params: [{name: id, in: path, required: false, type: integer, desc: custom}]\n    body: {}\n", false},
		{"bad-name", "operations:\n  Bad_Name: {get: /demo}\n", true},
		{"no-method", "operations:\n  demo: {summary: test}\n", true},
		{"two-methods", "operations:\n  demo: {get: /demo, post: /demo}\n", true},
		{"bad-yaml", "operations: [", true},
		{"null-extra-method", "operations:\n  demo: {get: /demo, post: null}\n", true},
		{"null-method", "operations:\n  demo: {get: null}\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("APIC_CONFIG_DIR", dir)
			file := filepath.Join(dir, "demo.yaml")
			assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte(tc.yaml), 0600)))
			index, err := (spec.SimpleLoader{}).Load(context.Background(), model.Source{File: file}, dir)
			if tc.wantErr {
				assert.Require(t, assert.Error(t, err))
				var coded *model.CodedError
				assert.Require(t, assert.True(t, errors.As(err, &coded)))
				assert.Eq(t, model.ExitLocal, coded.Code())
				assert.True(t, strings.Contains(err.Error(), file))
				return
			}
			assert.Require(t, assert.NoError(t, err))
			op := index.Ops[0]
			assert.Eq(t, "PUT", op.Method)
			assert.Eq(t, []model.Param{{Name: "id", In: "path", Type: "integer", Desc: "custom"}}, op.Params)
			assert.Require(t, assert.NotNil(t, op.Body))
			assert.Eq(t, "json", op.Body.Type)
		})
	}
}

func TestSimpleEnum(t *testing.T) {
	load := func(t *testing.T, paramYAML string) ([]model.Param, error) {
		t.Helper()
		dir := t.TempDir()
		file := filepath.Join(dir, "demo.yaml")
		content := "operations:\n  demo:\n    get: /demo\n    params:\n      - {name: p, in: query" + paramYAML + "}\n"
		assert.Require(t, assert.NoError(t, os.WriteFile(file, []byte(content), 0600)))
		index, err := (spec.SimpleLoader{}).Load(context.Background(), model.Source{File: "demo.yaml"}, dir)
		if err != nil {
			return nil, err
		}
		return index.Ops[0].Params, nil
	}
	enumOf := func(t *testing.T, paramYAML string) []string {
		t.Helper()
		params, err := load(t, paramYAML)
		assert.Require(t, assert.NoError(t, err))
		return params[0].Enum
	}

	t.Run("string enum", func(t *testing.T) {
		assert.Eq(t, []string{"opened", "closed", "all"}, enumOf(t, ", enum: [opened, closed, all]"))
	})
	t.Run("numbers and booleans become strings", func(t *testing.T) {
		assert.Eq(t, []string{"1", "true"}, enumOf(t, ", enum: [1, true]"))
		assert.Eq(t, []string{"-2", "false"}, enumOf(t, ", enum: [-2, false]"))
	})
	t.Run("float and hex converted by type", func(t *testing.T) {
		assert.Eq(t, []string{"1", "16"}, enumOf(t, ", enum: [1.0, 0x10]"))
		assert.Eq(t, []string{"1.5"}, enumOf(t, ", enum: [1.5]"))
	})
	t.Run("quoted keeps original text", func(t *testing.T) {
		assert.Eq(t, []string{"1.0", "007"}, enumOf(t, `, enum: ["1.0", "007"]`))
	})
	t.Run("nil enum means none", func(t *testing.T) {
		assert.Nil(t, enumOf(t, ", enum: ~"))
		assert.Nil(t, enumOf(t, ", enum: "))
		assert.Nil(t, enumOf(t, ""))
	})
	t.Run("invalid enums fail the load", func(t *testing.T) {
		cases := map[string]string{
			"empty list":       ", enum: []",
			"empty string":     `, enum: [""]`,
			"duplicate":        ", enum: [a, a]",
			"duplicate number": `, enum: [1, "1"]`,
			"map element":      ", enum: [{a: 1}]",
			"slice element":    ", enum: [[1]]",
			"null element":     ", enum: [a, ~]",
		}
		for name, y := range cases {
			t.Run(name, func(t *testing.T) {
				_, err := load(t, y)
				assert.Err(t, err)
				var coded *model.CodedError
				assert.True(t, errors.As(err, &coded))
				assert.Eq(t, model.ExitLocal, coded.Code())
				assert.Contains(t, err.Error(), "operation demo param p: enum ")
			})
		}
		_, err := load(t, ", enum: [{a: 1}]")
		assert.Contains(t, err.Error(), "enum values must be strings, numbers or booleans")
		_, err = load(t, ", enum: []")
		assert.Contains(t, err.Error(), "enum must be a non-empty list of distinct non-empty values")
	})
}

func TestSimpleStripsBOM(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIC_CONFIG_DIR", dir)
	content := "\xEF\xBB\xBFoperations:\n  ping:\n    get: /ping\n"
	assert.Require(t, assert.NoError(t, os.WriteFile(filepath.Join(dir, "demo.yaml"), []byte(content), 0600)))
	index, err := (spec.SimpleLoader{}).Load(context.Background(), model.Source{File: "demo.yaml"}, dir)
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, 1, len(index.Ops))
}

func TestGitLabExampleDefinition(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "gitlab")
	index, err := (spec.SimpleLoader{}).Load(context.Background(), model.Source{File: "defs/gitlab.yaml"}, dir)
	assert.Require(t, assert.NoError(t, err))
	assert.Eq(t, 8, len(index.Ops))
}

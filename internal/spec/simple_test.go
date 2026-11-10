package spec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/spec"
)

func TestSimpleExample(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APIX_CONFIG_DIR", dir)
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
			t.Setenv("APIX_CONFIG_DIR", dir)
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

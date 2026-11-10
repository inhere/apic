package spec

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apix/internal/model"
)

var simpleName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var pathParameter = regexp.MustCompile(`\{([^{}]+)\}`)

type simpleOperation struct {
	Get     *string       `yaml:"get"`
	Post    *string       `yaml:"post"`
	Put     *string       `yaml:"put"`
	Patch   *string       `yaml:"patch"`
	Delete  *string       `yaml:"delete"`
	Head    *string       `yaml:"head"`
	Summary string        `yaml:"summary"`
	Output  string        `yaml:"output"`
	Query   []string      `yaml:"query"`
	Params  []simpleParam `yaml:"params"`
	Body    *simpleBody   `yaml:"body"`
}

type simpleParam struct {
	Name     string `yaml:"name"`
	In       string `yaml:"in"`
	Required bool   `yaml:"required"`
	Type     string `yaml:"type"`
	Desc     string `yaml:"desc"`
}

type simpleBody struct {
	Type string `yaml:"type"`
}

// SimpleLoader loads user-named operations from a simple YAML definition.
type SimpleLoader struct{}

var _ Loader = SimpleLoader{}

// Load reads src.File relative to dir unless it is absolute.
func (SimpleLoader) Load(ctx context.Context, src model.Source, dir string) (*model.Index, error) {
	file := src.File
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	if err := ctx.Err(); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	var doc struct {
		Operations map[string]simpleOperation `yaml:"operations"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	index := &model.Index{}
	names := make([]string, 0, len(doc.Operations))
	for name := range doc.Operations {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !simpleName.MatchString(name) {
			return nil, model.Errorf(model.ExitLocal, "%s: invalid operation name %q (expected kebab-case)", file, name)
		}
		raw := doc.Operations[name]
		op := model.Operation{Name: name, Summary: raw.Summary, Output: raw.Output}
		methods := []struct {
			name string
			path *string
		}{{"GET", raw.Get}, {"POST", raw.Post}, {"PUT", raw.Put}, {"PATCH", raw.Patch}, {"DELETE", raw.Delete}, {"HEAD", raw.Head}}
		count := 0
		for _, method := range methods {
			if method.path != nil {
				op.Method, op.Path = method.name, *method.path
				count++
			}
		}
		if count != 1 {
			return nil, model.Errorf(model.ExitLocal, "%s: operation %q must have exactly one method", file, name)
		}
		for _, p := range raw.Params {
			op.Params = append(op.Params, model.Param{Name: p.Name, In: p.In, Required: p.Required, Type: p.Type, Desc: p.Desc})
		}
		for _, q := range raw.Query {
			op.Params = append(op.Params, model.Param{Name: q, In: "query", Type: "string"})
		}
		for _, match := range pathParameter.FindAllStringSubmatch(op.Path, -1) {
			found := false
			for _, p := range op.Params {
				if p.In == "path" && p.Name == match[1] {
					found = true
					break
				}
			}
			if !found {
				op.Params = append(op.Params, model.Param{Name: match[1], In: "path", Required: true, Type: "string"})
			}
		}
		if raw.Body != nil {
			bodyType := raw.Body.Type
			if strings.TrimSpace(bodyType) == "" {
				bodyType = "json"
			}
			op.Body = &model.BodySpec{Type: bodyType}
		}
		index.Ops = append(index.Ops, op)
	}
	return index, nil
}

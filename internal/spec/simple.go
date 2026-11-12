package spec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
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
	Enum     []any  `yaml:"enum"` // any: YAML numbers and booleans decode to Go types
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
	data = model.StripBOM(data)
	var doc struct {
		Operations map[string]simpleOperation `yaml:"operations"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	var keys struct {
		Operations map[string]map[string]any `yaml:"operations"`
	}
	if err := yaml.Unmarshal(data, &keys); err != nil {
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
			if _, exists := keys.Operations[name][strings.ToLower(method.name)]; exists {
				count++
				if method.path != nil {
					op.Method, op.Path = method.name, *method.path
				}
			}
		}
		if count != 1 || op.Method == "" {
			return nil, model.Errorf(model.ExitLocal, "%s: operation %q must have exactly one method", file, name)
		}
		for _, p := range raw.Params {
			enum, err := simpleEnum(p.Enum)
			if err != nil {
				return nil, model.Errorf(model.ExitLocal, "%s operation %s param %s: %v", file, name, p.Name, err)
			}
			op.Params = append(op.Params, model.Param{Name: p.Name, In: p.In, Required: p.Required, Type: p.Type, Desc: p.Desc, Enum: enum})
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
			} else if bodyType != "json" && bodyType != "form" && bodyType != "multipart" && bodyType != "xml" && bodyType != "raw" {
				return nil, model.Errorf(model.ExitLocal, "%s operation %s: body.type must be one of json, form, multipart, xml, raw", file, name)
			}
			op.Body = &model.BodySpec{Type: bodyType}
		}
		index.Ops = append(index.Ops, op)
	}
	return index, nil
}

// simpleEnum converts YAML enum values to the strings compared at call time.
// nil (absent, "enum:" or "enum: ~") means no enum. Numbers are converted by
// their decoded Go type, so 1.0 becomes "1" and 0x10 becomes "16".
func simpleEnum(raw []any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, item := range raw {
		var text string
		switch v := item.(type) {
		case string:
			text = v
		case bool:
			text = strconv.FormatBool(v)
		case int:
			text = strconv.FormatInt(int64(v), 10)
		case int8:
			text = strconv.FormatInt(int64(v), 10)
		case int16:
			text = strconv.FormatInt(int64(v), 10)
		case int32:
			text = strconv.FormatInt(int64(v), 10)
		case int64:
			text = strconv.FormatInt(v, 10)
		case uint:
			text = strconv.FormatUint(uint64(v), 10)
		case uint8:
			text = strconv.FormatUint(uint64(v), 10)
		case uint16:
			text = strconv.FormatUint(uint64(v), 10)
		case uint32:
			text = strconv.FormatUint(uint64(v), 10)
		case uint64:
			text = strconv.FormatUint(v, 10)
		case float32:
			text = strconv.FormatFloat(float64(v), 'f', -1, 32)
		case float64:
			text = strconv.FormatFloat(v, 'f', -1, 64)
		default: // map, slice, nil
			return nil, errors.New("enum values must be strings, numbers or booleans")
		}
		if text == "" || seen[text] {
			return nil, errors.New("enum must be a non-empty list of distinct non-empty values")
		}
		seen[text] = true
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil, errors.New("enum must be a non-empty list of distinct non-empty values")
	}
	return out, nil
}

package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
)

var overlayNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var overlayValues = map[string][]string{
	"body":   {"json", "form", "multipart", "xml", "raw"},
	"output": {"auto", "json", "raw", "yaml", "table"},
	"type":   {"string", "integer", "number", "boolean", "array"},
	"in":     {"query", "header", "path"},
}

// LoadOverlay loads and validates one site's optional overlay file.
func LoadOverlay(dir, site string) (*model.Overlay, error) {
	file, err := filepath.Abs(filepath.Join(dir, "overlays", site+".yaml"))
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "overlay %s: %v", filepath.Join(dir, "overlays", site+".yaml"), err)
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "overlay %s: %v", file, err)
	}

	ov := &model.Overlay{File: file}
	// Strict decoding rejects unknown and duplicate keys before value validation.
	if err := yaml.UnmarshalWithOptions(data, ov, yaml.Strict()); err != nil {
		return nil, overlayDecodeError(file, err)
	}
	for _, opKey := range sortedOverlayKeys(ov.Ops) {
		op := ov.Ops[opKey]
		if op == nil {
			op = &model.OpOverlay{}
			ov.Ops[opKey] = op
		}
		if err := validateOperationOverlay(file, opKey, op); err != nil {
			return nil, err
		}
	}
	return ov, nil
}

func overlayDecodeError(file string, err error) error {
	line := strings.SplitN(strings.TrimSuffix(err.Error(), "\r"), "\n", 2)[0]
	return model.Errorf(model.ExitLocal, "overlay %s: %s", file, line)
}

func validateOperationOverlay(file, opKey string, op *model.OpOverlay) error {
	for _, field := range []string{"body", "output"} {
		var value string
		switch field {
		case "body":
			if op.Body == nil {
				continue
			}
			value = *op.Body
		case "output":
			if op.Output == nil {
				continue
			}
			value = *op.Output
		}
		if !overlayValueAllowed(field, value) {
			return overlayValueError(file, opKey, "", field+" must be one of "+strings.Join(overlayValues[field], ", "))
		}
	}
	if op.Name != nil {
		if *op.Name == "help" {
			return overlayValueError(file, opKey, "", "name must not be help")
		}
		if !overlayNamePattern.MatchString(*op.Name) {
			return overlayValueError(file, opKey, "", "name must match ^[a-z0-9]+(?:-[a-z0-9]+)*$")
		}
	}
	for _, paramKey := range sortedOverlayKeys(op.Params) {
		param := op.Params[paramKey]
		if param == nil {
			param = &model.ParamOverlay{}
			op.Params[paramKey] = param
		}
		if param.In != "" && !overlayValueAllowed("in", param.In) {
			return overlayValueError(file, opKey, paramKey, "in must be one of "+strings.Join(overlayValues["in"], ", "))
		}
		if param.Type != nil && !overlayValueAllowed("type", *param.Type) {
			return overlayValueError(file, opKey, paramKey, "type must be one of "+strings.Join(overlayValues["type"], ", "))
		}
	}
	return nil
}

func overlayValueError(file, opKey, paramKey, reason string) error {
	key := opKey
	if paramKey != "" {
		key += "." + paramKey
	}
	return model.Errorf(model.ExitLocal, "overlay %s: %s: %s", file, key, reason)
}

func overlayValueAllowed(field, value string) bool {
	for _, allowed := range overlayValues[field] {
		if value == allowed {
			return true
		}
	}
	return false
}

func sortedOverlayKeys[T any](values map[string]*T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

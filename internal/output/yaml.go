package output

import (
	"encoding/json"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
)

// yamlNumber writes a JSON number as its original literal.
type yamlNumber string

func (n yamlNumber) MarshalYAML() ([]byte, error) { return []byte(n), nil }

// ToYAML renders a decoded value as YAML, keeping key order and number literals.
func ToYAML(v any) ([]byte, error) {
	return yaml.Marshal(toYAMLValue(v))
}

func toYAMLValue(v any) any {
	switch t := v.(type) {
	case model.Object:
		ms := make(yaml.MapSlice, 0, len(t.Keys))
		for _, k := range t.Keys {
			ms = append(ms, yaml.MapItem{Key: k, Value: toYAMLValue(t.Vals[k])})
		}
		return ms
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = toYAMLValue(e)
		}
		return out
	case json.Number:
		return yamlNumber(t.String())
	}
	return v
}

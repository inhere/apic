package request

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/inhere/apix/internal/model"
)

// buildBody returns the buffered body and its automatic Content-Type.
func buildBody(op *model.Operation, in Input) ([]byte, string, error) {
	btype := "json"
	var spec *model.BodySpec
	if op != nil && op.Body != nil {
		spec = op.Body
		if spec.Type != "" {
			btype = spec.Type
		}
	}
	hasD, hasF := in.Data != "", len(in.Fields) > 0
	if hasD && hasF {
		return nil, "", errBodyConflict()
	}
	if !hasD && !hasF {
		if spec != nil && spec.Required {
			return nil, "", model.Errorf(model.ExitLocal, "request body is required (use -d or -f)")
		}
		return nil, "", nil
	}
	if hasF && (btype == "xml" || btype == "raw") {
		return nil, "", model.Errorf(model.ExitUsage, "%s body only accepts -d", btype)
	}

	ctype := ""
	switch btype {
	case "json":
		ctype = "application/json"
	case "form":
		ctype = "application/x-www-form-urlencoded"
	case "xml":
		ctype = "application/xml"
	}

	if hasD {
		b, err := readData(in)
		return b, ctype, err
	}
	if btype == "form" {
		form := url.Values{}
		for _, f := range in.Fields {
			k, v, isJSON, err := splitField(f)
			if err != nil {
				return nil, "", err
			}
			if isJSON {
				v = strings.TrimSpace(v)
				var s string
				if json.Unmarshal([]byte(v), &s) == nil {
					v = s
				}
			}
			form.Add(k, v)
		}
		return []byte(form.Encode()), ctype, nil
	}
	root := map[string]any{}
	for _, f := range in.Fields {
		k, v, isJSON, err := splitField(f)
		if err != nil {
			return nil, "", err
		}
		var val any = v
		if isJSON {
			if err := json.Unmarshal([]byte(v), &val); err != nil {
				return nil, "", model.Errorf(model.ExitLocal, "invalid JSON for field %q: %v", k, err)
			}
		}
		if err := setNested(root, k, val); err != nil {
			return nil, "", err
		}
	}
	b, err := json.Marshal(root)
	if err != nil {
		return nil, "", model.Errorf(model.ExitLocal, "encode body: %v", err)
	}
	return b, ctype, nil
}

// splitField parses "k=v" or "k:=json".
func splitField(f string) (k, v string, isJSON bool, err error) {
	i := strings.Index(f, "=")
	if i <= 0 {
		return "", "", false, model.Errorf(model.ExitUsage, "invalid -f %q, want k=v or k:=json", f)
	}
	k, v = f[:i], f[i+1:]
	if strings.HasSuffix(k, ":") {
		k, isJSON = strings.TrimSuffix(k, ":"), true
	}
	if k == "" {
		return "", "", false, model.Errorf(model.ExitUsage, "invalid -f %q, empty key", f)
	}
	return k, v, isJSON, nil
}

// setNested sets root[a][b]... = val for a dotted key.
func setNested(root map[string]any, dotted string, val any) error {
	parts := strings.Split(dotted, ".")
	cur := root
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			if _, exists := cur[p]; exists {
				return model.Errorf(model.ExitLocal, "field %q conflicts with an existing non-object value", dotted)
			}
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = val
	return nil
}

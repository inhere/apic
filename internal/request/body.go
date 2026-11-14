package request

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// bodyType returns the operation's body type; json when unspecified.
func bodyType(op *model.Operation) string {
	if op != nil && op.Body != nil && op.Body.Type != "" {
		return op.Body.Type
	}
	return "json"
}

// fieldText applies the "k:=json" conversion shared by form and multipart:
// trim whitespace and unquote JSON strings; other values are kept as written.
func fieldText(v string, isJSON bool) string {
	if !isJSON {
		return v
	}
	v = strings.TrimSpace(v)
	var s string
	if json.Unmarshal([]byte(v), &s) == nil {
		v = s
	}
	return v
}

// buildBody returns the buffered body, its automatic Content-Type and multipart parts.
func buildBody(op *model.Operation, in Input) ([]byte, string, []Part, error) {
	btype := bodyType(op)
	var spec *model.BodySpec
	if op != nil {
		spec = op.Body
	}
	hasD, hasF, hasForm := in.Data != "", len(in.Fields) > 0, len(in.Forms) > 0
	if hasForm && (hasD || hasF) {
		return nil, "", nil, model.Errorf(model.ExitUsage, "-F cannot be combined with -d or -f")
	}
	if hasForm {
		if op != nil && spec == nil {
			return nil, "", nil, model.Errorf(model.ExitUsage, "operation has no request body; add one with an overlay (body: multipart)")
		}
		if op != nil && btype != "multipart" {
			return nil, "", nil, model.Errorf(model.ExitUsage, "operation body is %s, not multipart; fix it with an overlay (body: multipart)", btype)
		}
		parts := make([]Part, 0, len(in.Forms))
		for _, f := range in.Forms {
			p, err := ParsePart(f)
			if err != nil {
				return nil, "", nil, err
			}
			parts = append(parts, p)
		}
		return nil, "", parts, nil
	}
	if btype == "multipart" && hasF {
		if hasD {
			return nil, "", nil, errBodyConflict()
		}
		parts := make([]Part, 0, len(in.Fields))
		for _, f := range in.Fields {
			k, v, isJSON, err := splitField(f)
			if err != nil {
				return nil, "", nil, err
			}
			p, err := TextPart(k, fieldText(v, isJSON))
			if err != nil {
				return nil, "", nil, err
			}
			parts = append(parts, p)
		}
		return nil, "", parts, nil
	}
	if btype == "multipart" && hasD {
		// Build verifies the caller supplied a Content-Type header.
		b, err := readData(in)
		return b, "", nil, err
	}
	if hasD && hasF {
		return nil, "", nil, errBodyConflict()
	}
	if !hasD && !hasF {
		if spec != nil && spec.Required {
			if btype == "multipart" {
				return nil, "", nil, model.Errorf(model.ExitLocal, "request body is required (use -F, -d or -f)")
			}
			return nil, "", nil, model.Errorf(model.ExitLocal, "request body is required (use -d or -f)")
		}
		return nil, "", nil, nil
	}
	if hasF && (btype == "xml" || btype == "raw") {
		return nil, "", nil, model.Errorf(model.ExitUsage, "%s body only accepts -d", btype)
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
		return b, ctype, nil, err
	}
	if btype == "form" {
		form := url.Values{}
		for _, f := range in.Fields {
			k, v, isJSON, err := splitField(f)
			if err != nil {
				return nil, "", nil, err
			}
			form.Add(k, fieldText(v, isJSON))
		}
		return []byte(form.Encode()), ctype, nil, nil
	}
	root := map[string]any{}
	for _, f := range in.Fields {
		k, v, isJSON, err := splitField(f)
		if err != nil {
			return nil, "", nil, err
		}
		var val any = v
		if isJSON {
			if err := json.Unmarshal([]byte(v), &val); err != nil {
				return nil, "", nil, model.Errorf(model.ExitLocal, "invalid JSON for field %q: %v", k, err)
			}
		} else if val, err = typedField(spec, k, v); err != nil {
			return nil, "", nil, err
		}
		if err := setNested(root, k, val); err != nil {
			return nil, "", nil, err
		}
	}
	b, err := json.Marshal(root)
	if err != nil {
		return nil, "", nil, model.Errorf(model.ExitLocal, "encode body: %v", err)
	}
	return b, ctype, nil, nil
}

// typedField converts a k=v value to the scalar type the body schema declares
// for the top-level field k (integer, number or boolean). Other fields, dotted
// keys and string, array or object types keep the string as written; k:=json
// is the way to send those typed.
func typedField(spec *model.BodySpec, k, v string) (any, error) {
	if spec == nil {
		return v, nil
	}
	typ := ""
	for _, f := range spec.Schema.Fields {
		if f.Name == k {
			typ = f.Type
			break
		}
	}
	bad := func() error {
		return model.Errorf(model.ExitLocal, "field %q: %q is not a valid %s (use %s:=<json> to send it as written)", k, v, typ, k)
	}
	switch typ {
	case "boolean":
		switch v {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, bad()
	case "integer":
		// json.Valid rejects forms ParseInt accepts but JSON does not (+5, 007).
		if _, err := strconv.ParseInt(v, 10, 64); err != nil || !json.Valid([]byte(v)) {
			return nil, bad()
		}
		return json.Number(v), nil
	case "number":
		// ParseFloat rejects quoted strings; json.Valid rejects Inf, NaN, 0x10.
		if _, err := strconv.ParseFloat(v, 64); err != nil || !json.Valid([]byte(v)) {
			return nil, bad()
		}
		return json.Number(v), nil
	}
	return v, nil
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

package spec

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// forbiddenHeaders are headers apic manages itself; overlays may not add them
// as new parameters (compared case-insensitively).
var forbiddenHeaders = []string{
	"host", "content-type", "content-length", "transfer-encoding",
	"authorization", "cookie", "proxy-authorization",
}

// ApplyOverlay returns a new index with ov applied (idx is never mutated), the
// recorded changes, warnings and a structural error. A nil ov yields a copy of idx.
func ApplyOverlay(idx *model.Index, ov *model.Overlay) (*model.Index, []model.OverlayChange, []string, error) {
	out := &model.Index{DefaultBasePath: idx.DefaultBasePath, DefaultBaseURL: idx.DefaultBaseURL}
	out.Ops = make([]model.Operation, len(idx.Ops))
	for i := range idx.Ops {
		out.Ops[i] = copyOperation(idx.Ops[i])
	}
	if ov == nil || len(ov.Ops) == 0 {
		return out, nil, nil, nil
	}

	keys := make([]string, 0, len(ov.Ops))
	for k := range ov.Ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fail := func(where, format string, a ...any) error {
		return model.Errorf(model.ExitLocal, "overlay %s: %s: %s", ov.File, where, fmt.Sprintf(format, a...))
	}

	// Step 1: resolve each key to one operation.
	var warnings []string
	hits := make(map[int]string) // op index -> key
	var order []int              // matched op indexes in key order
	opKey := make(map[int]*model.OpOverlay)
	for _, key := range keys {
		matches := matchOps(out.Ops, key)
		if len(matches) == 0 {
			warnings = append(warnings, fmt.Sprintf("overlay %s: %s matches no operation", ov.File, key))
			continue
		}
		if len(matches) > 1 {
			return nil, nil, nil, fail(key, "matches %d operations", len(matches))
		}
		i := matches[0]
		if other, ok := hits[i]; ok {
			return nil, nil, nil, fail(key, "targets the same operation as %q", other)
		}
		hits[i] = key
		order = append(order, i)
		oo := ov.Ops[key]
		if oo == nil {
			oo = &model.OpOverlay{}
		}
		opKey[i] = oo
	}

	// Steps 2-4: hide, params, body, output. Fields are collected per op and
	// turned into change records once final names are known.
	hidden := make(map[int]bool)
	fields := make(map[int][]string)
	explicit := make(map[int]string)
	for _, i := range order {
		oo, key, op := opKey[i], hits[i], &out.Ops[i]
		if oo.Hide {
			hidden[i] = true
			continue
		}
		if oo.Name != nil {
			if strings.TrimSpace(*oo.Name) == "" {
				return nil, nil, nil, fail(key, "name must not be empty")
			}
			explicit[i] = *oo.Name
			fields[i] = append(fields[i], "name")
		}
		if oo.Summary != nil {
			op.Summary = *oo.Summary
			fields[i] = append(fields[i], "summary")
		}
		if oo.Body != nil {
			if op.Body == nil {
				op.Body = &model.BodySpec{Type: *oo.Body}
			} else {
				op.Body.Type = *oo.Body
			}
			fields[i] = append(fields[i], "body")
		}
		if oo.Output != nil {
			op.Output = *oo.Output
			fields[i] = append(fields[i], "output")
		}
		pkeys := make([]string, 0, len(oo.Params))
		for pk := range oo.Params {
			pkeys = append(pkeys, pk)
		}
		sort.Strings(pkeys)
		for _, pk := range pkeys {
			f, err := applyParam(op, pk, oo.Params[pk], key, fail)
			if err != nil {
				return nil, nil, nil, err
			}
			fields[i] = append(fields[i], f...)
		}
	}

	// Step 5: rename. Explicit names first (must be unique), then every other
	// operation colliding with one gets a -2/-3... suffix.
	owner := make(map[string]string) // explicit name -> key
	for _, i := range order {
		name, ok := explicit[i]
		if !ok {
			continue
		}
		if other, dup := owner[name]; dup {
			return nil, nil, nil, fail(hits[i], "name %q is also set by %q", name, other)
		}
		owner[name] = hits[i]
	}
	reserved := make(map[string]bool, len(out.Ops))
	for name := range owner {
		reserved[name] = true
	}
	for i := range out.Ops {
		if _, ok := explicit[i]; !ok && !hidden[i] {
			reserved[out.Ops[i].Name] = true
		}
	}
	for i := range out.Ops {
		if hidden[i] {
			continue
		}
		if name, ok := explicit[i]; ok {
			out.Ops[i].Name = name
			continue
		}
		base := out.Ops[i].Name
		if _, clash := owner[base]; !clash {
			continue
		}
		for n := 2; ; n++ {
			candidate := base + "-" + strconv.Itoa(n)
			if !reserved[candidate] {
				reserved[candidate] = true
				out.Ops[i].Name = candidate
				warnings = append(warnings, fmt.Sprintf("operation %s %s renamed %q to %q",
					strings.ToUpper(out.Ops[i].Method), out.Ops[i].Path, base, candidate))
				break
			}
		}
	}

	// Step 6: change records in key order, then drop hidden operations.
	var changes []model.OverlayChange
	for _, i := range order {
		for _, f := range fields[i] {
			changes = append(changes, model.OverlayChange{Op: out.Ops[i].Name, Field: f})
		}
	}
	kept := out.Ops[:0]
	for i := range out.Ops {
		if !hidden[i] {
			kept = append(kept, out.Ops[i])
		}
	}
	out.Ops = kept
	return out, changes, warnings, nil
}

// matchOps returns indexes of operations selected by key: "METHOD /path" when
// it contains a space, otherwise the original operation name.
func matchOps(ops []model.Operation, key string) []int {
	var res []int
	method, path, byRoute := strings.Cut(key, " ")
	path = strings.TrimSpace(path)
	for i, op := range ops {
		if byRoute {
			if strings.EqualFold(op.Method, method) && op.Path == path {
				res = append(res, i)
			}
		} else if op.Name == key {
			res = append(res, i)
		}
	}
	return res
}

// applyParam applies one parameter overlay to op and returns the change fields.
func applyParam(op *model.Operation, pk string, po *model.ParamOverlay, opKey string, fail func(string, string, ...any) error) ([]string, error) {
	where := opKey + "." + pk
	if po == nil {
		po = &model.ParamOverlay{}
	}
	// Full-name match first; with `in` set only that location is a candidate.
	var found []int
	for i, p := range op.Params {
		if p.Name == pk {
			found = append(found, i)
		}
	}
	name, in := pk, po.In
	if len(found) > 0 {
		if po.In == "" && len(found) > 1 {
			return nil, fail(where, "matches parameters in several locations, set in")
		}
		if po.In != "" {
			var same []int
			for _, i := range found {
				if op.Params[i].In == po.In {
					same = append(same, i)
				}
			}
			if len(same) == 0 {
				return nil, fail(where, "in %q does not match the parameter's location %q", po.In, op.Params[found[0]].In)
			}
			found = same
		}
	} else if pre, rest, ok := strings.Cut(pk, "."); ok && isParamIn(pre) {
		// Fall back to the in.name form.
		if po.In != "" && po.In != pre {
			return nil, fail(where, "in %q does not match %q", po.In, pre)
		}
		name, in = rest, pre
		for i, p := range op.Params {
			if p.In == pre && p.Name == rest {
				found = append(found, i)
			}
		}
	}

	var fs []string
	if len(found) > 0 {
		p := &op.Params[found[0]]
		if po.Required != nil {
			p.Required = *po.Required
			fs = append(fs, paramField(p, "required"))
		}
		if po.Type != nil {
			p.Type = *po.Type
			fs = append(fs, paramField(p, "type"))
		}
		if po.Enum != nil {
			p.Enum = slices.Clone(po.Enum)
			fs = append(fs, paramField(p, "enum"))
		}
		if po.Desc != nil {
			p.Desc = *po.Desc
			fs = append(fs, paramField(p, "desc"))
		}
		if po.Hide {
			if p.In == "path" {
				return nil, fail(where, "path parameter cannot be hidden")
			}
			p.Hidden = true
			fs = append(fs, paramField(p, "hide"))
		}
		if p.Hidden && p.Required {
			return nil, fail(where, "hidden parameter is still required")
		}
		return fs, nil
	}

	// New parameter: only query and header are allowed.
	if in != "query" && in != "header" {
		return nil, fail(where, "new parameter needs in: query or header")
	}
	if in == "header" {
		if !isHeaderToken(name) {
			return nil, fail(where, "header name %q is not a valid token", name)
		}
		if slices.Contains(forbiddenHeaders, strings.ToLower(name)) {
			return nil, fail(where, "header %q cannot be added", name)
		}
	}
	p := model.Param{Name: name, In: in, Type: "string"}
	if po.Required != nil {
		p.Required = *po.Required
	}
	if po.Type != nil {
		p.Type = *po.Type
	}
	if po.Enum != nil {
		p.Enum = slices.Clone(po.Enum)
	}
	if po.Desc != nil {
		p.Desc = *po.Desc
	}
	p.Hidden = po.Hide
	if p.Hidden && p.Required {
		return nil, fail(where, "hidden parameter is still required")
	}
	op.Params = append(op.Params, p)
	return []string{paramField(&p, "added")}, nil
}

func paramField(p *model.Param, attr string) string {
	return "params." + p.In + "." + p.Name + "." + attr
}

func isParamIn(s string) bool { return s == "path" || s == "query" || s == "header" }

// isHeaderToken reports whether s is an RFC 7230 token (all tchar).
func isHeaderToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

func copyOperation(op model.Operation) model.Operation {
	op.Tags = slices.Clone(op.Tags)
	if op.Params != nil {
		params := make([]model.Param, len(op.Params))
		for i, p := range op.Params {
			p.Enum = slices.Clone(p.Enum)
			params[i] = p
		}
		op.Params = params
	}
	if op.Body != nil {
		b := *op.Body
		b.Schema.Fields = slices.Clone(b.Schema.Fields)
		op.Body = &b
	}
	if op.Responses != nil {
		resp := make(map[string]model.SchemaSummary, len(op.Responses))
		for k, v := range op.Responses {
			v.Fields = slices.Clone(v.Fields)
			resp[k] = v
		}
		op.Responses = resp
	}
	return op
}

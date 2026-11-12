package model

import (
	"fmt"
	"strconv"
	"strings"
)

// Pick is a parsed --pick path: dot separated segments, each a key, a
// non-negative index or "*".
type Pick struct{ segs []string }

// ParsePick parses a pick path; syntax errors carry ExitUsage.
func ParsePick(path string) (Pick, error) {
	if path == "" {
		return Pick{}, Errorf(ExitUsage, "--pick path must not be empty")
	}
	segs := strings.Split(path, ".")
	for i, s := range segs {
		if s == "" {
			return Pick{}, Errorf(ExitUsage, "--pick path %q has an empty segment at position %d", path, i+1)
		}
	}
	return Pick{segs: segs}, nil
}

// String returns the path text.
func (p Pick) String() string { return strings.Join(p.segs, ".") }

// Apply walks v along the path. A missing non-wildcard segment is an
// ExitPostProcess error; after a "*" missing segments yield nil instead.
func (p Pick) Apply(v any) (any, error) {
	return applyPick(v, p.segs, nil)
}

func isIndex(s string) (int, bool) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func pickFail(prefix []string, seg, why string) error {
	at := "root"
	if len(prefix) > 0 {
		at = strings.Join(prefix, ".")
	}
	return Errorf(ExitPostProcess, "--pick segment %q %s (after %s)", seg, why, at)
}

// projectPick handles a last segment containing commas. An object with a
// literal key of the same name wins (compatibility); otherwise the segment is a
// comma separated key list and the result keeps that order, missing keys as nil.
func projectPick(v any, seg string) (any, error) {
	obj, isObj := v.(Object)
	if isObj {
		if val, ok := obj.Vals[seg]; ok {
			return val, nil
		}
	}
	keys := strings.Split(seg, ",")
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			return nil, Errorf(ExitPostProcess, "--pick projection %q has an empty or duplicate key", seg)
		}
		seen[k] = true
	}
	if !isObj {
		return nil, Errorf(ExitPostProcess, "--pick projection %q needs an object; add .* before it for arrays", seg)
	}
	out := Object{Keys: keys, Vals: make(map[string]any, len(keys))}
	for _, k := range keys {
		out.Vals[k] = obj.Vals[k] // absent keys stay nil
	}
	return out, nil
}

func applyPick(v any, segs, prefix []string) (any, error) {
	if len(segs) == 0 {
		return v, nil
	}
	seg := segs[0]
	if len(segs) == 1 && strings.Contains(seg, ",") {
		return projectPick(v, seg)
	}
	next := append(append([]string(nil), prefix...), seg)
	switch t := v.(type) {
	case Object:
		if seg == "*" {
			out := make([]any, 0, len(t.Keys))
			for _, k := range t.Keys {
				r, err := applyPick(t.Vals[k], segs[1:], next)
				if err != nil {
					r = nil
				}
				out = append(out, r)
			}
			return out, nil
		}
		val, ok := t.Vals[seg]
		if !ok {
			if strings.Contains(seg, ",") {
				return nil, pickFail(prefix, seg, "not found; "+projectionHint(append(next, segs[1:]...)))
			}
			return nil, pickFail(prefix, seg, "not found")
		}
		return applyPick(val, segs[1:], next)
	case []any:
		if seg == "*" {
			out := make([]any, 0, len(t))
			for _, e := range t {
				r, err := applyPick(e, segs[1:], next)
				if err != nil {
					r = nil
				}
				out = append(out, r)
			}
			return out, nil
		}
		n, ok := isIndex(seg)
		if !ok {
			return nil, pickFail(prefix, seg, "is not an array index")
		}
		if n >= len(t) {
			return nil, pickFail(prefix, seg, fmt.Sprintf("is out of range (length %d)", len(t)))
		}
		return applyPick(t[n], segs[1:], next)
	}
	return nil, pickFail(prefix, seg, "cannot be applied to a scalar value")
}

// projectionHint explains a comma outside the last segment, such as the
// full-path list "data.username,data.id", and suggests "data.username,id"
// when every listed path shares the same parent.
func projectionHint(segs []string) string {
	const generic = "a projection lists keys in the last segment only, e.g. data.items.*.id,name"
	parts := strings.Split(strings.Join(segs, "."), ",")
	first := parts[0]
	dot := strings.LastIndex(first, ".")
	if dot < 0 {
		return generic
	}
	base := first[:dot]
	keys := []string{first[dot+1:]}
	for _, p := range parts[1:] {
		p = strings.TrimPrefix(p, base+".")
		if p == "" || strings.Contains(p, ".") {
			return generic
		}
		keys = append(keys, p)
	}
	return fmt.Sprintf("a projection lists keys in the last segment only: try %s.%s", base, strings.Join(keys, ","))
}

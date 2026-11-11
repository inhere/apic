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

func applyPick(v any, segs, prefix []string) (any, error) {
	if len(segs) == 0 {
		return v, nil
	}
	seg := segs[0]
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

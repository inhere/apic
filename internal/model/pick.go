package model

import (
	"fmt"
	"strconv"
	"strings"
)

// Pick is a parsed --pick expression: comma separated items that are all
// evaluated from the document root, each one optionally aliased (uid=data.id)
// and optionally ending in a {...} group (data.{username,id}).
type Pick struct {
	src   string // the expression as the user wrote it
	items []pickItem
}

// pickItem is one top-level item or one item inside a group: an optional alias,
// the key/index/"*" segments and the group that was written as its last segment.
type pickItem struct {
	alias string
	segs  []string
	group []pickItem
}

// ParsePick parses a pick path; syntax errors carry ExitUsage. Every comma
// separated item starts at the root and a group in the last segment of an item
// lists paths relative to the item's prefix.
func ParsePick(path string) (Pick, error) {
	if path == "" {
		return Pick{}, Errorf(ExitUsage, "--pick %q: path must not be empty", path)
	}
	items, err := parseItems(path, path)
	if err != nil {
		return Pick{}, err
	}
	p := Pick{src: path, items: items}
	if err := p.check(); err != nil {
		return Pick{}, err
	}
	return p, nil
}

// String returns the expression text as the user wrote it.
func (p Pick) String() string { return p.src }

// IsPlainPath reports whether the expression is a single path without alias,
// group or "*", so it names exactly one key chain from the root.
func (p Pick) IsPlainPath() bool {
	if len(p.items) != 1 {
		return false
	}
	it := p.items[0]
	return it.alias == "" && it.group == nil && !it.starred()
}

// Apply evaluates the expression against a decoded response. A single direct
// item keeps the plain path behaviour (a missing segment is an ExitPostProcess
// error, errors below "*" become nil). Everything else builds an Object in
// written key order with nil for paths that are not there.
func (p Pick) Apply(v any) (any, error) {
	if len(p.items) == 0 {
		return v, nil
	}
	if len(p.items) == 1 && p.items[0].direct() {
		it := p.items[0]
		if it.group == nil {
			return applyPick(v, it.segs, nil)
		}
		return evalItemValue(v, it)
	}
	// A single item keeps the strict behaviour (missing paths are errors); with
	// several items a missing path is null.
	kvs, err := evalLevel(v, p.items, len(p.items) == 1)
	if err != nil {
		return nil, err
	}
	return pickObject(kvs), nil
}

// pickSyntax builds a usage error that names the whole expression.
func pickSyntax(path, format string, a ...any) error {
	return Errorf(ExitUsage, "--pick %q: %s", path, fmt.Sprintf(format, a...))
}

// validAlias reports whether s matches [A-Za-z0-9_-]+.
func validAlias(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// splitTop splits s on sep outside braces and rejects unbalanced braces.
func splitTop(s string, sep byte) ([]string, error) {
	parts := make([]string, 0, 2)
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced braces")
			}
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced braces")
	}
	return append(parts, s[start:]), nil
}

// aliasEnd returns the index of the '=' that separates an item's alias, or -1
// when the item has none. A '=' behind the first '{' belongs to the group.
func aliasEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '=':
			return i
		case '{':
			return -1
		}
	}
	return -1
}

// isGroup reports whether s is exactly one {...} segment.
func isGroup(s string) bool {
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false // a second segment follows the group
			}
		}
	}
	return depth == 0
}

// parseItems parses one comma separated item list: the top level or the inside
// of a group.
func parseItems(path, text string) ([]pickItem, error) {
	parts, err := splitTop(text, ',')
	if err != nil {
		return nil, pickSyntax(path, "%s", err)
	}
	items := make([]pickItem, 0, len(parts))
	for _, part := range parts {
		it, err := parseItem(path, part)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

func parseItem(path, text string) (pickItem, error) {
	if text == "" {
		return pickItem{}, pickSyntax(path, "empty item")
	}
	it := pickItem{}
	if i := aliasEnd(text); i >= 0 {
		it.alias = text[:i]
		if it.alias == "" {
			return pickItem{}, pickSyntax(path, "empty alias")
		}
		if !validAlias(it.alias) {
			return pickItem{}, pickSyntax(path, "invalid alias %q", it.alias)
		}
		text = text[i+1:]
		if text == "" {
			return pickItem{}, pickSyntax(path, "item %q has no path", it.alias)
		}
	}
	segs, err := splitTop(text, '.')
	if err != nil {
		return pickItem{}, pickSyntax(path, "%s", err)
	}
	for i, seg := range segs {
		if seg == "" {
			return pickItem{}, pickSyntax(path, "empty segment")
		}
		if !strings.ContainsAny(seg, "{}") {
			it.segs = append(it.segs, seg)
			continue
		}
		if i != len(segs)-1 {
			return pickItem{}, pickSyntax(path, "a group must be the last segment")
		}
		if !isGroup(seg) {
			return pickItem{}, pickSyntax(path, "malformed group %q", seg)
		}
		inner, err := parseItems(path, seg[1:len(seg)-1])
		if err != nil {
			return pickItem{}, err
		}
		it.group = inner
	}
	return it, nil
}

// starred reports whether the path goes through "*".
func (it pickItem) starred() bool {
	for _, s := range it.segs {
		if s == "*" {
			return true
		}
	}
	return false
}

// direct reports whether a lone item yields one value instead of an object: no
// alias and either no group or a group below "*".
func (it pickItem) direct() bool { return it.alias == "" && (it.group == nil || it.starred()) }

// keyName is the key this item occupies in its object: the alias, otherwise the
// last segment that is neither "*" nor an index. Empty when there is none, which
// needs an alias.
func (it pickItem) keyName() string {
	if it.alias != "" {
		return it.alias
	}
	for i := len(it.segs) - 1; i >= 0; i-- {
		if it.segs[i] == "*" {
			continue
		}
		if _, ok := isIndex(it.segs[i]); ok {
			continue
		}
		return it.segs[i]
	}
	return ""
}

// check validates key names and the keys that are statically known to repeat.
func (p Pick) check() error {
	if len(p.items) == 1 && p.items[0].direct() {
		if p.items[0].group == nil {
			return nil
		}
		_, err := checkLevel(p.src, p.items[0].group)
		return err
	}
	_, err := checkLevel(p.src, p.items)
	return err
}

// checkLevel collects the keys one object level gets and rejects a missing key
// name or a duplicate. An expandable group hoists its keys into this level; an
// aliased group or a group below "*" is an object level of its own.
func checkLevel(path string, items []pickItem) ([]string, error) {
	keys := make([]string, 0, len(items))
	for _, it := range items {
		if it.group != nil && it.alias == "" && !it.starred() {
			sub, err := checkLevel(path, it.group)
			if err != nil {
				return nil, err
			}
			keys = append(keys, sub...)
			continue
		}
		if it.group != nil {
			if _, err := checkLevel(path, it.group); err != nil {
				return nil, err
			}
		}
		k := it.keyName()
		if k == "" {
			return nil, pickSyntax(path, "an item has no key name; write alias=path")
		}
		keys = append(keys, k)
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			return nil, pickSyntax(path, "duplicate key %q", k)
		}
		seen[k] = true
	}
	return keys, nil
}

// pickKeys lists the keys an items list contributes to its object level, with
// the keys of expandable groups hoisted as checkLevel does.
func pickKeys(items []pickItem) []string {
	keys := make([]string, 0, len(items))
	for _, it := range items {
		if it.group != nil && it.alias == "" && !it.starred() {
			keys = append(keys, pickKeys(it.group)...)
			continue
		}
		keys = append(keys, it.keyName())
	}
	return keys
}

type pickKV struct {
	key string
	val any
}

func pickObject(kvs []pickKV) Object {
	obj := Object{Vals: make(map[string]any, len(kvs))}
	for _, kv := range kvs {
		if _, dup := obj.Vals[kv.key]; !dup {
			obj.Keys = append(obj.Keys, kv.key)
		}
		obj.Vals[kv.key] = kv.val
	}
	return obj
}

// groupObjectErr reports a group whose prefix is not an object.
func groupObjectErr(it pickItem) error {
	if len(it.segs) == 0 {
		return Errorf(ExitPostProcess, "--pick group needs an object (at root)")
	}
	return Errorf(ExitPostProcess, "--pick group %q needs an object", it.groupPrefix())
}

// groupPrefix is the path text in front of a group, for error messages.
func (it pickItem) groupPrefix() string { return strings.Join(it.segs, ".") }

// evalLevel evaluates an items list against v, the value the items are relative
// to, and returns ordered key/value pairs. In strict mode a failing item is an
// error; otherwise it produces nil, as inside a group or below "*".
func evalLevel(v any, items []pickItem, strict bool) ([]pickKV, error) {
	out := make([]pickKV, 0, len(items))
	for _, it := range items {
		switch {
		case it.alias != "" || (it.group != nil && it.starred()):
			// One value, either nested under the alias or the element objects.
			val, err := evalItemValue(v, it)
			if err != nil {
				if strict {
					return nil, err
				}
				val = nil
			}
			out = append(out, pickKV{it.keyName(), val})
		case it.group != nil:
			// An expandable group: its keys are hoisted into this object.
			base, err := applyPick(v, it.segs, nil)
			if err == nil && !isObject(base) {
				err = groupObjectErr(it)
			}
			if err != nil {
				if strict {
					return nil, err
				}
				for _, k := range pickKeys(it.group) {
					out = append(out, pickKV{k, nil})
				}
				continue
			}
			sub, err := evalLevel(base, it.group, false)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		default:
			val, err := applyPick(v, it.segs, nil)
			if err != nil {
				if strict {
					return nil, err
				}
				val = nil
			}
			out = append(out, pickKV{it.keyName(), val})
		}
	}
	return out, nil
}

// evalItemValue returns the value of one item that yields a single object or an
// array of objects: an aliased group, or a group below "*".
func evalItemValue(v any, it pickItem) (any, error) {
	base, err := applyPick(v, it.segs, nil)
	if err != nil {
		return nil, err
	}
	if it.group == nil {
		return base, nil
	}
	if it.starred() {
		arr, ok := base.([]any)
		if !ok {
			return nil, Errorf(ExitPostProcess, "--pick group %q needs an array", it.groupPrefix())
		}
		out := make([]any, 0, len(arr))
		for _, e := range arr {
			obj, ok := e.(Object)
			if !ok {
				out = append(out, nil) // not an object: the element itself is nil
				continue
			}
			kvs, err := evalLevel(obj, it.group, false)
			if err != nil {
				return nil, err
			}
			out = append(out, pickObject(kvs))
		}
		return out, nil
	}
	obj, ok := base.(Object)
	if !ok {
		return nil, groupObjectErr(it)
	}
	kvs, err := evalLevel(obj, it.group, false)
	if err != nil {
		return nil, err
	}
	return pickObject(kvs), nil
}

func isObject(v any) bool {
	_, ok := v.(Object)
	return ok
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

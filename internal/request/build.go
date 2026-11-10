// Package request builds, protects and executes HTTP requests for apic.
package request

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// Input is the user-supplied request description.
type Input struct {
	Method  string            // upper-cased by Build; empty means GET
	Path    string            // raw path (raw mode only)
	Params  map[string]string // values passed as --<name>, keyed by parameter name
	P       []string          // raw -p values: name=v, query.name=v, path.name=v, header.name=v
	Data    string            // -d: @file, - (stdin) or literal
	Fields  []string          // -f: k=v or k:=json
	Headers []string          // -H: "Name: value"
	Stdin   io.Reader         // source for -d -; nil means os.Stdin
}

// Prepared is a fully built request whose body is buffered for replay.
type Prepared struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

const (
	locQuery  = "query"
	locPath   = "path"
	locHeader = "header"
)

// Build creates a Prepared request. op is nil in raw-path mode.
func Build(op *model.Operation, in Input, env model.Env, basePath string) (*Prepared, error) {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}

	var path string
	if op == nil {
		path = in.Path
		if err := checkRawPath(path); err != nil {
			return nil, err
		}
		basePath = ""
	} else {
		path = op.Path
	}

	vals, err := collectValues(op, in)
	if err != nil {
		return nil, err
	}
	if op != nil {
		if err := validateParams(op, vals); err != nil {
			return nil, err
		}
	}

	// path parameters
	for k, vs := range vals {
		loc, name := splitKey(k)
		if loc != locPath {
			continue
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(vs[0]))
	}
	if op != nil {
		if i := strings.Index(path, "{"); i >= 0 && strings.Contains(path[i:], "}") {
			return nil, model.Errorf(model.ExitLocal, "missing required path parameter in %s", path)
		}
	}

	full := strings.TrimRight(env.BaseURL, "/") + basePath + path
	u, err := url.Parse(full)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "invalid url: %s", urlErrText(err))
	}
	q := u.Query()
	hdr := http.Header{}
	for _, k := range sortedKeys(vals) {
		loc, name := splitKey(k)
		switch loc {
		case locQuery:
			for _, v := range vals[k] {
				q.Add(name, v)
			}
		case locHeader:
			hdr.Set(name, vals[k][0])
		}
	}
	u.RawQuery = q.Encode()

	body, ctype, err := buildBody(op, in)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		hdr.Set("Content-Type", ctype)
	}
	for _, h := range in.Headers {
		name, value, ok := strings.Cut(h, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, model.Errorf(model.ExitUsage, "invalid header %q, want 'Name: value'", h)
		}
		hdr.Set(name, strings.TrimSpace(value))
	}
	return &Prepared{Method: method, URL: u.String(), Header: hdr, Body: body}, nil
}

// checkRawPath rejects full URLs and protocol-relative paths so credentials only reach base_url.
func checkRawPath(p string) error {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return model.Errorf(model.ExitUsage, "path must start with a single '/': %q", p)
	}
	return nil
}

func urlErrText(err error) string {
	// Do not echo possibly secret URLs from raw errors; keep the cause only.
	if ue, ok := err.(*url.Error); ok {
		return ue.Err.Error()
	}
	return err.Error()
}

func key(loc, name string) string { return loc + "\x00" + name }

func splitKey(k string) (string, string) {
	loc, name, _ := strings.Cut(k, "\x00")
	return loc, name
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isLoc(s string) bool { return s == locQuery || s == locPath || s == locHeader }

// collectValues maps --name values and -p values to (location, name) keys.
func collectValues(op *model.Operation, in Input) (map[string][]string, error) {
	vals := map[string][]string{}
	// locate returns the operation's locations for a name.
	locate := func(name string) []string {
		var locs []string
		if op == nil {
			return nil
		}
		for _, p := range op.Params {
			if p.Name == name {
				locs = append(locs, p.In)
			}
		}
		return locs
	}
	for _, name := range sortedStringKeys(in.Params) {
		v := in.Params[name]
		if op == nil {
			vals[key(locQuery, name)] = append(vals[key(locQuery, name)], v)
			continue
		}
		locs := locate(name)
		switch len(locs) {
		case 0:
			return nil, model.Errorf(model.ExitUsage, "unknown parameter %q", name)
		case 1:
			vals[key(locs[0], name)] = []string{v}
		default:
			return nil, model.Errorf(model.ExitUsage, "parameter %q exists in several locations, use -p <location>.%s=value", name, name)
		}
	}
	for _, raw := range in.P {
		name, v, ok := strings.Cut(raw, "=")
		if !ok || name == "" {
			return nil, model.Errorf(model.ExitUsage, "invalid -p %q, want name=value", raw)
		}
		loc := ""
		if op != nil {
			if locs := locate(name); len(locs) > 0 {
				if len(locs) > 1 {
					return nil, model.Errorf(model.ExitUsage, "parameter %q exists in several locations, use -p <location>.%s=value", name, name)
				}
				loc = locs[0]
			}
		}
		if loc == "" {
			if l, n, found := strings.Cut(name, "."); found && isLoc(l) && n != "" {
				loc, name = l, n
			}
		}
		if loc == "" {
			if op != nil {
				return nil, model.Errorf(model.ExitUsage, "unknown parameter %q", name)
			}
			loc = locQuery
		}
		k := key(loc, name)
		if loc == locQuery {
			vals[k] = append(vals[k], v)
		} else {
			vals[k] = []string{v}
		}
	}
	return vals, nil
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validateParams checks required parameters and value types (ExitLocal on failure).
func validateParams(op *model.Operation, vals map[string][]string) error {
	for _, p := range op.Params {
		vs, has := vals[key(p.In, p.Name)]
		if !has {
			if p.Required || p.In == locPath {
				return model.Errorf(model.ExitLocal, "missing required %s parameter %q", p.In, p.Name)
			}
			continue
		}
		for _, v := range vs {
			if err := checkType(p, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkType(p model.Param, v string) error {
	var err error
	switch p.Type {
	case "integer":
		_, err = strconv.ParseInt(v, 10, 64)
	case "number":
		_, err = strconv.ParseFloat(v, 64)
	case "boolean":
		_, err = strconv.ParseBool(v)
	}
	if err != nil {
		return model.Errorf(model.ExitLocal, "parameter %q: %q is not a valid %s", p.Name, v, p.Type)
	}
	if len(p.Enum) > 0 {
		for _, e := range p.Enum {
			if e == v {
				return nil
			}
		}
		return model.Errorf(model.ExitLocal, "parameter %q: %q not in enum [%s]", p.Name, v, strings.Join(p.Enum, ", "))
	}
	return nil
}

func readFileData(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "read body file: %v", err)
	}
	return b, nil
}

func readData(in Input) ([]byte, error) {
	switch {
	case in.Data == "-":
		r := in.Stdin
		if r == nil {
			r = os.Stdin
		}
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "read stdin: %v", err)
		}
		return b, nil
	case strings.HasPrefix(in.Data, "@"):
		return readFileData(in.Data[1:])
	default:
		return []byte(in.Data), nil
	}
}

func errBodyConflict() error {
	return model.Errorf(model.ExitUsage, "-d and -f cannot be used together")
}

package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

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

	return ParseOverlay(file, data)
}

// ParseOverlay strictly decodes and validates overlay file content; file is
// only used for messages and model.Overlay.File.
func ParseOverlay(file string, data []byte) (*model.Overlay, error) {
	ov := &model.Overlay{File: file}
	data = model.StripBOM(data)
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

// OverlayTemplate renders a starter overlay file for idx. Keys are JSON-encoded
// (valid YAML double-quoted scalars) so spec text can never inject entries;
// the comment is sanitized to one line. A METHOD /path shared by several
// operations (simple definitions) is keyed by operation name instead.
func OverlayTemplate(site string, idx *model.Index) []byte {
	var b strings.Builder
	b.WriteString("# apic overlay for site " + sanitizeOverlayComment(site) + "; edit and keep only what you change.\noperations:")
	routes := make(map[string]int)
	for _, op := range idx.Ops {
		routes[strings.ToUpper(op.Method)+" "+op.Path]++
	}
	if len(idx.Ops) == 0 {
		b.WriteString(" {}")
	}
	b.WriteString("\n")
	for _, op := range idx.Ops {
		key := strings.ToUpper(op.Method) + " " + op.Path
		if routes[key] > 1 {
			key = op.Name
		}
		enc, err := json.Marshal(key)
		if err != nil { // unreachable for a string
			continue
		}
		b.WriteString("  " + string(enc) + ": {}  # " + sanitizeOverlayComment("name: "+op.Name+"; "+op.Summary) + "\n")
	}
	return []byte(b.String())
}

// sanitizeOverlayComment makes s safe for a single YAML comment line: control
// characters and U+2028/U+2029 become spaces, and it is cut to 120 runes.
func sanitizeOverlayComment(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == '\u2028' || r == '\u2029' {
			r = ' '
		}
		if r == utf8.RuneError {
			r = ' '
		}
		out = append(out, r)
		if len(out) == 120 {
			break
		}
	}
	return string(out)
}

// WriteOverlay creates overlays/<site>.yaml exclusively and returns its path.
// An existing overlays entry must be a real directory (no symlink or junction).
func WriteOverlay(dir, site string, data []byte) (string, error) {
	odir := filepath.Join(dir, "overlays")
	path := filepath.Join(odir, site+".yaml")
	info, err := os.Lstat(odir)
	switch {
	case err == nil:
		if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return "", model.Errorf(model.ExitLocal, "overlays path %s is not a directory", odir)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := os.Mkdir(odir, 0700); err != nil {
			return "", model.Errorf(model.ExitLocal, "create %s: %v", odir, err)
		}
	default:
		return "", model.Errorf(model.ExitLocal, "overlays path %s: %v", odir, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return "", model.Errorf(model.ExitLocal, "overlay file %s already exists", path)
	}
	if err != nil {
		return "", model.Errorf(model.ExitLocal, "overlay file %s: %v", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", model.Errorf(model.ExitLocal, "write overlay file %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		return "", model.Errorf(model.ExitLocal, "write overlay file %s: %v", path, err)
	}
	return path, nil
}

package request

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

// Part is one multipart/form-data part: a text field (File empty) or a file.
type Part struct {
	Name     string
	Value    string // text part
	File     string // file part: path; non-empty means file
	Filename string // file part: defaults to filepath.Base(File)
	Type     string // file part: defaults to the builtin table, else application/octet-stream
	Size     int64  // filled by Assemble for -v
}

// MaxMultipartBytes caps the in-memory multipart body.
const MaxMultipartBytes = 64 << 20

var typePattern = regexp.MustCompile(`^[A-Za-z0-9!#$&^_.+-]+/[A-Za-z0-9!#$&^_.+-]+$`)

// Builtin extension table; deliberately not mime.TypeByExtension, which depends on the machine.
var builtinTypes = map[string]string{
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xls":  "application/vnd.ms-excel",
	".csv":  "text/csv",
	".json": "application/json",
	".txt":  "text/plain",
	".xml":  "application/xml",
	".zip":  "application/zip",
	".gz":   "application/gzip",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// checkToken validates a part name or filename: they are written inside quotes without escaping.
func checkToken(what, s string) error {
	if hasControl(s) || strings.ContainsAny(s, `"\`) {
		return model.Errorf(model.ExitUsage, "%s %q must not contain control characters, '\"' or '\\'", what, s)
	}
	return nil
}

func usage(format string, a ...any) error { return model.Errorf(model.ExitUsage, format, a...) }

// TextPart builds a text part for -f; the name is validated like ParsePart's.
func TextPart(name, value string) (Part, error) {
	if name == "" {
		return Part{}, usage("multipart field name must not be empty")
	}
	if err := checkToken("multipart field name", name); err != nil {
		return Part{}, err
	}
	return Part{Name: name, Value: value}, nil
}

// ParsePart parses one -F value: name=text, name=@path[;type=..][;filename=..].
func ParsePart(s string) (Part, error) {
	name, val, ok := strings.Cut(s, "=")
	if !ok {
		return Part{}, usage("invalid -F %q, want name=value or name=@file", s)
	}
	if name == "" {
		return Part{}, usage("invalid -F %q, empty name", s)
	}
	if !strings.HasPrefix(val, "@") {
		return TextPart(name, val)
	}
	if err := checkToken("-F name", name); err != nil {
		return Part{}, err
	}
	rest := val[1:]
	path, rest, err := scanValue(rest, name)
	if err != nil {
		return Part{}, err
	}
	if path == "" {
		return Part{}, usage("-F %s: file path is empty", name)
	}
	if path == "-" {
		return Part{}, usage("-F %s: stdin is not supported; use ./-", name)
	}
	if hasControl(path) {
		return Part{}, usage("-F %s: file path must not contain control characters", name)
	}
	p := Part{Name: name, File: path}
	seen := map[string]bool{}
	for rest != "" {
		// rest starts with ';'
		rest = rest[1:]
		key, after, found := strings.Cut(rest, "=")
		if !found || (key != "type" && key != "filename") {
			return Part{}, usage("-F %s: unknown parameter %q, only type and filename are supported", name, strings.SplitN(rest, ";", 2)[0])
		}
		if seen[key] {
			return Part{}, usage("-F %s: duplicate parameter %q", name, key)
		}
		seen[key] = true
		var v string
		if key == "filename" {
			v, rest, err = scanValue(after, name)
		} else {
			// type is never quoted: it must match type/subtype exactly
			v, rest, err = scanBare(after)
		}
		if err != nil {
			return Part{}, err
		}
		if key == "type" {
			p.Type = v
		} else {
			if v == "" {
				return Part{}, usage("-F %s: filename is empty", name)
			}
			p.Filename = v
		}
	}
	if p.Type != "" && !typePattern.MatchString(p.Type) {
		return Part{}, usage("-F %s: invalid type %q, want type/subtype without parameters", name, p.Type)
	}
	if p.Filename == "" {
		p.Filename = filepath.Base(path)
		if err := checkToken("-F "+name+": default filename", p.Filename); err != nil {
			return Part{}, usage("%s; use ;filename= to set one", err.Error())
		}
	} else if err := checkToken("-F "+name+": filename", p.Filename); err != nil {
		return Part{}, err
	}
	if p.Type == "" {
		p.Type = builtinTypes[strings.ToLower(filepath.Ext(path))]
		if p.Type == "" {
			p.Type = "application/octet-stream"
		}
	}
	return p, nil
}

// scanBare reads an unquoted value up to the next ';'.
func scanBare(s string) (string, string, error) {
	if i := strings.IndexByte(s, ';'); i >= 0 {
		return s[:i], s[i:], nil
	}
	return s, "", nil
}

// scanValue reads either a "quoted" value (\" and \\ escapes) or a bare value up to the next ';'.
// It returns the value and the remainder, which is empty or starts with ';'.
func scanValue(s, name string) (string, string, error) {
	if !strings.HasPrefix(s, `"`) {
		return scanBare(s)
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\'):
			b.WriteByte(s[i+1])
			i++
		case c == '"':
			rest := s[i+1:]
			if rest != "" && rest[0] != ';' {
				return "", "", usage("-F %s: unexpected text after closing quote", name)
			}
			return b.String(), rest, nil
		default:
			b.WriteByte(c)
		}
	}
	return "", "", usage("-F %s: unclosed quote", name)
}

var errNotRegular = errors.New("not a regular file")

// openRegular opens path only if it is a regular file. The pre-Stat avoids blocking on FIFOs,
// and the handle's Stat plus SameFile closes the swap race. /proc pseudo files count as regular
// and are bounded only by the size cap.
func openRegular(path string) (*os.File, os.FileInfo, error) {
	pre, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !pre.Mode().IsRegular() {
		return nil, nil, errNotRegular
	}
	f, err := openFile(path)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() || !os.SameFile(pre, fi) {
		_ = f.Close()
		return nil, nil, errNotRegular
	}
	return f, fi, nil
}

func openErr(p Part, err error) error {
	if errors.Is(err, errNotRegular) {
		return model.Errorf(model.ExitLocal, "-F %s: %s is not a regular file", p.Name, p.File)
	}
	return model.Errorf(model.ExitLocal, "-F %s: %v", p.Name, err)
}

func errTooLarge() error {
	return model.Errorf(model.ExitLocal, "multipart body exceeds 64 MiB; use curl for large uploads")
}

// CheckFiles performs Assemble's file checks without reading content (dry-run).
func CheckFiles(parts []Part) error {
	var total int64
	for _, p := range parts {
		if p.File == "" {
			total += int64(len(p.Value))
		} else {
			f, fi, err := openRegular(p.File)
			if err != nil {
				return openErr(p, err)
			}
			_ = f.Close()
			total += fi.Size()
		}
		if total > MaxMultipartBytes {
			return errTooLarge()
		}
	}
	return nil
}

// capBuf is a buffer that fails once it would exceed its limit.
type capBuf struct {
	buf   bytes.Buffer
	limit int
}

var errCapExceeded = errors.New("cap exceeded")

func (c *capBuf) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.limit {
		return 0, errCapExceeded
	}
	return c.buf.Write(p)
}

// Assemble builds the multipart body in memory and fills Part.Size for file parts.
func Assemble(parts []Part) ([]byte, string, error) {
	cb := &capBuf{limit: MaxMultipartBytes}
	w := multipart.NewWriter(cb)
	for i := range parts {
		p := &parts[i]
		h := textproto.MIMEHeader{}
		// name and filename were validated to contain no '"' or '\', so quoting is direct.
		if p.File == "" {
			h.Set("Content-Disposition", `form-data; name="`+p.Name+`"`)
		} else {
			h.Set("Content-Disposition", `form-data; name="`+p.Name+`"; filename="`+p.Filename+`"`)
			h.Set("Content-Type", p.Type)
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			return nil, "", asnErr(err)
		}
		if p.File == "" {
			if _, err := io.WriteString(pw, p.Value); err != nil {
				return nil, "", asnErr(err)
			}
			continue
		}
		if err := copyFile(pw, p, cb); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", asnErr(err)
	}
	return cb.buf.Bytes(), w.FormDataContentType(), nil
}

func copyFile(w io.Writer, p *Part, cb *capBuf) error {
	f, _, err := openRegular(p.File)
	if err != nil {
		return openErr(*p, err)
	}
	defer f.Close()
	remain := int64(cb.limit - cb.buf.Len())
	n, err := io.Copy(w, io.LimitReader(f, remain+1))
	p.Size = n
	if err != nil {
		if errors.Is(err, errCapExceeded) {
			return errTooLarge()
		}
		// error text omits file content by construction
		return model.Errorf(model.ExitLocal, "-F %s: %v", p.Name, err)
	}
	return nil
}

func asnErr(err error) error {
	if errors.Is(err, errCapExceeded) {
		return errTooLarge()
	}
	return model.Errorf(model.ExitLocal, "assemble multipart body: %v", err)
}

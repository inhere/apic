// Package bundle packs a site into one shareable multi-document YAML file and
// installs such a file into the configuration directory.
package bundle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
)

// Document kinds of a bundle.
const (
	KindSite       = "site"
	KindDefinition = "definition"
	KindOverlay    = "overlay"
)

var kindLine = regexp.MustCompile(`^kind:[ \t]*([A-Za-z]+)[ \t]*(?:#.*)?$`)

// Bundle is a decoded site bundle. Definition and Overlay hold the verbatim
// text without their kind line; nil means the document is absent.
type Bundle struct {
	Name       string
	Site       *model.Site
	Definition []byte
	Overlay    []byte
}

// siteDoc is the site document of a bundle.
type siteDoc struct {
	Kind string      `yaml:"kind"`
	Name string      `yaml:"name"`
	Site *model.Site `yaml:"site"`
}

// normalize drops a BOM, turns CRLF into LF and ends the text with one newline.
func normalize(data []byte) []byte {
	data = bytes.ReplaceAll(model.StripBOM(data), []byte("\r\n"), []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return data
}

// embedText prepares a definition or overlay file for embedding: normalized,
// without a leading "---" line, and free of lines that would split the bundle.
func embedText(what string, data []byte) ([]byte, error) {
	data = normalize(data)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trim := strings.TrimRight(line, " \t")
		if trim == "---" && strings.TrimSpace(strings.Join(lines[:i], "")) == "" {
			lines[i] = ""
			continue
		}
		if trim == "---" || trim == "..." {
			return nil, model.Errorf(model.ExitLocal, "%s contains a YAML document separator line (%s); remove it before export", what, trim)
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// Encode renders the bundle text. definition and overlay may be nil. The site
// is written as it is: references stay unexpanded and Origin is not exported.
func Encode(name string, site *model.Site, definition, overlay []byte) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("# apic site bundle: apic site import <this file>\n")
	data, err := yaml.Marshal(siteDoc{Kind: KindSite, Name: name, Site: site})
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "encode site %s: %v", name, err)
	}
	out.Write(data)
	for _, doc := range []struct {
		kind, what string
		text       []byte
	}{{KindDefinition, "definition", definition}, {KindOverlay, "overlay", overlay}} {
		if doc.text == nil {
			continue
		}
		text, err := embedText(doc.what, doc.text)
		if err != nil {
			return nil, err
		}
		out.WriteString("---\nkind: " + doc.kind + "\n")
		out.Write(text)
	}
	return out.Bytes(), nil
}

// splitDocuments splits normalized text at lines holding only "---".
func splitDocuments(text string) []string {
	var docs []string
	var cur []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimRight(line, " \t") == "---" {
			docs = append(docs, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return append(docs, strings.Join(cur, "\n"))
}

// docKind returns the kind of one document and the document text; the kind
// line must be the first line that is neither blank nor a comment. For
// stripKind the line is removed from the returned text.
func docKind(doc string, stripKind bool) (kind, text string, ok bool) {
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		m := kindLine.FindStringSubmatch(line)
		if m == nil {
			return "", "", false
		}
		if stripKind {
			lines = append(lines[:i:i], lines[i+1:]...)
		}
		return m[1], strings.Join(lines, "\n"), true
	}
	return "", "", false
}

// Decode parses a bundle: exactly one site document, at most one definition
// and one overlay. The site is not validated beyond YAML syntax here; see Build.
func Decode(data []byte) (*Bundle, error) {
	b := &Bundle{}
	var haveSite, haveDef, haveOverlay bool
	for _, doc := range splitDocuments(string(normalize(data))) {
		if strings.TrimSpace(strings.Join(commentFree(doc), "")) == "" {
			continue
		}
		kind, _, ok := docKind(doc, false)
		if !ok {
			return nil, model.Errorf(model.ExitLocal, "bundle: every document must start with a kind: line")
		}
		switch kind {
		case KindSite:
			if haveSite {
				return nil, model.Errorf(model.ExitLocal, "bundle: more than one kind: site document")
			}
			haveSite = true
			var raw struct {
				Kind string         `yaml:"kind"`
				Name string         `yaml:"name"`
				Site map[string]any `yaml:"site"`
			}
			if err := yaml.UnmarshalWithOptions([]byte(doc), &raw, yaml.Strict()); err != nil {
				return nil, model.Errorf(model.ExitLocal, "bundle site document: %s", firstLine(err))
			}
			if raw.Site == nil {
				return nil, model.Errorf(model.ExitLocal, "bundle site document: site is missing")
			}
			siteYAML, err := yaml.Marshal(raw.Site)
			if err != nil {
				return nil, model.Errorf(model.ExitLocal, "bundle site document: %v", err)
			}
			b.Name = raw.Name
			site, err := config.ParseSite("", "bundle", "bundle", siteYAML)
			if err != nil {
				return nil, err
			}
			b.Site = site
		case KindDefinition:
			if haveDef {
				return nil, model.Errorf(model.ExitLocal, "bundle: more than one kind: definition document")
			}
			haveDef = true
			_, text, _ := docKind(doc, true)
			b.Definition = trimDoc(text)
		case KindOverlay:
			if haveOverlay {
				return nil, model.Errorf(model.ExitLocal, "bundle: more than one kind: overlay document")
			}
			haveOverlay = true
			_, text, _ := docKind(doc, true)
			b.Overlay = trimDoc(text)
		default:
			return nil, model.Errorf(model.ExitLocal, "bundle: unknown document kind %q", kind)
		}
	}
	if !haveSite {
		return nil, model.Errorf(model.ExitLocal, "bundle: no kind: site document")
	}
	return b, nil
}

func commentFree(doc string) []string {
	var out []string
	for _, line := range strings.Split(doc, "\n") {
		if t := strings.TrimSpace(line); !strings.HasPrefix(t, "#") {
			out = append(out, t)
		}
	}
	return out
}

// trimDoc drops the blank lines a separator leaves at both ends and ends the
// text with one newline.
func trimDoc(text string) []byte {
	text = strings.Trim(text, "\n")
	return []byte(text + "\n")
}

func firstLine(err error) string {
	return strings.SplitN(strings.TrimSuffix(err.Error(), "\r"), "\n", 2)[0]
}

// ExportResult is a rendered bundle with notes for the user.
type ExportResult struct {
	Data  []byte
	Notes []string
}

// Export renders the bundle of one loaded site. Without allowPlaintext, a site
// with literal credentials is refused. dir is the configuration directory.
func Export(dir, name string, site *model.Site, allowPlaintext bool) (*ExportResult, error) {
	if !allowPlaintext {
		if fields := config.PlaintextCredentials(name, site); len(fields) > 0 {
			return nil, model.Errorf(model.ExitLocal, "site %s has plaintext credentials:\n  %s\nuse ${env:NAME} references or pass --allow-plaintext", name, strings.Join(fields, "\n  "))
		}
	}
	res := &ExportResult{}
	exported := *site
	var definition []byte
	switch {
	case site.Source.Type == "simple":
		data, err := os.ReadFile(site.Source.File)
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "read definition %s: %v", site.Source.File, err)
		}
		definition = data
		exported.Source.File = ""
	case site.Source.File != "":
		exported.Source.File = ""
		res.Notes = append(res.Notes, "the spec file "+site.Source.File+" is not embedded; set source.url or source.file after import")
	}
	var overlay []byte
	data, err := os.ReadFile(filepath.Join(dir, "overlays", name+".yaml"))
	switch {
	case err == nil:
		overlay = data
	case !os.IsNotExist(err):
		return nil, model.Errorf(model.ExitLocal, "read overlay: %v", err)
	}
	res.Data, err = Encode(name, &exported, definition, overlay)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// File is one file an import writes, relative to the configuration directory.
type File struct {
	Rel  string
	Data []byte
}

// Plan is a validated import: the files to write and the commands it enables.
type Plan struct {
	Name     string
	Site     *model.Site
	Files    []File
	Execs    []config.ExecUse
	Warnings []string
}

// rel is the slash-separated path of a managed file below the configuration directory.
func rel(kind, name string) string { return kind + "/" + name + ".yaml" }

// Build validates a decoded bundle for installation as name (empty: the
// bundle's own name) and renders the files. Nothing is written: the definition
// is loaded with the simple loader and the overlay parsed and, for a simple
// site, applied to that index, all from memory or a temporary file.
func Build(b *Bundle, name string) (*Plan, error) {
	if name == "" {
		name = b.Name
	}
	if err := config.ValidateSiteName(name); err != nil {
		return nil, err
	}
	site := *b.Site
	p := &Plan{Name: name}
	var idx *model.Index
	switch site.Source.Type {
	case "simple":
		if b.Definition == nil {
			return nil, model.Errorf(model.ExitLocal, "bundle: a simple site needs a kind: definition document")
		}
		defRel := rel("defs", name)
		var err error
		if idx, err = loadDefinition(b.Definition); err != nil {
			return nil, err
		}
		site.Source.File = defRel
		p.Files = append(p.Files, File{defRel, b.Definition})
	case "openapi":
		if b.Site.Source.File != "" {
			return nil, model.Errorf(model.ExitLocal, "bundle: an openapi site may not carry source.file")
		}
		if b.Definition != nil {
			return nil, model.Errorf(model.ExitLocal, "bundle: a kind: definition document needs source.type simple")
		}
		if site.Source.URL == "" {
			p.Warnings = append(p.Warnings, fmt.Sprintf("site %s has no source.url; set it in sites/%s.yaml", name, name))
		}
	default:
		return nil, model.Errorf(model.ExitLocal, "bundle: unsupported source.type %q", site.Source.Type)
	}
	if b.Overlay != nil {
		ov, err := config.ParseOverlay(rel("overlays", name), b.Overlay)
		if err != nil {
			return nil, err
		}
		if idx != nil {
			var warnings []string
			if _, _, warnings, err = spec.ApplyOverlay(idx, ov); err != nil {
				return nil, err
			}
			p.Warnings = append(p.Warnings, warnings...)
		}
		p.Files = append(p.Files, File{rel("overlays", name), b.Overlay})
	}
	// Written as site add writes it: source.file stays relative to the config dir.
	data, err := yaml.Marshal(&site)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "encode site %s: %v", name, err)
	}
	site.Origin = rel("sites", name)
	p.Site = &site
	p.Files = append([]File{{rel("sites", name), data}}, p.Files...)
	p.Execs = config.ExecCommands(name, &site)
	return p, nil
}

// loadDefinition writes the text to a temporary file and loads it with the
// simple loader.
func loadDefinition(text []byte) (*model.Index, error) {
	tmp, err := os.MkdirTemp("", "apic-bundle-")
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "%v", err)
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "definition.yaml")
	if err := os.WriteFile(file, text, 0600); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%v", err)
	}
	idx, err := spec.SimpleLoader{}.Load(context.Background(), model.Source{Type: "simple", File: file}, tmp)
	if err != nil {
		// The temporary path is of no use to the reader.
		return nil, model.Errorf(model.ExitLocal, "bundle definition: %s", strings.ReplaceAll(err.Error(), file, "definition"))
	}
	return idx, nil
}

// Conflicts lists why the plan cannot be installed over the current state: the
// loaded site (its Origin tells whether config.yaml owns it) and the files that
// exist. Without force every conflict counts; with force only a site defined in
// config.yaml does, since apic never rewrites that file.
func Conflicts(dir string, cfg *model.Config, p *Plan, force bool) []string {
	var out []string
	if existing, ok := cfg.Sites[p.Name]; ok {
		if existing != nil && filepath.Clean(existing.Origin) == filepath.Clean(filepath.Join(dir, "config.yaml")) {
			return []string{fmt.Sprintf("site %q is defined in config.yaml; edit it manually or import under another --name", p.Name)}
		}
		if !force {
			out = append(out, fmt.Sprintf("site %q already exists; use --force to overwrite it or --name to import under another name", p.Name))
		}
	}
	if !force {
		for _, f := range p.Files {
			if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(f.Rel))); err == nil {
				out = append(out, f.Rel+" already exists")
			}
		}
	}
	return out
}

// Install writes the plan's files. Every file is first written to a temporary
// file beside its target, then renamed into place; on a failure the files this
// run touched are restored or removed. It returns the relative paths written.
func Install(dir string, p *Plan) (written []string, err error) {
	type staged struct{ tmp, dest string }
	var stage []staged
	defer func() {
		for _, s := range stage {
			_ = os.Remove(s.tmp) // already renamed or never created: ignore
		}
	}()
	for _, f := range p.Files {
		dest := filepath.Join(dir, filepath.FromSlash(f.Rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return nil, model.Errorf(model.ExitLocal, "%v", err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(dest), ".apic-import-*")
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "%v", err)
		}
		stage = append(stage, staged{tmp.Name(), dest})
		_, werr := tmp.Write(f.Data)
		cerr := tmp.Close()
		if werr != nil || cerr != nil {
			return nil, model.Errorf(model.ExitLocal, "write %s: %v", f.Rel, errors.Join(werr, cerr))
		}
		if err := os.Chmod(tmp.Name(), 0600); err != nil {
			return nil, model.Errorf(model.ExitLocal, "%v", err)
		}
	}
	type undo struct {
		dest   string
		backup []byte // nil: the file did not exist
		had    bool
	}
	var done []undo
	for i, s := range stage {
		u := undo{dest: s.dest}
		if old, rerr := os.ReadFile(s.dest); rerr == nil {
			u.backup, u.had = old, true
		}
		if rerr := os.Rename(s.tmp, s.dest); rerr != nil {
			err = model.Errorf(model.ExitLocal, "write %s: %v", p.Files[i].Rel, rerr)
			for _, d := range done {
				if d.had {
					if werr := os.WriteFile(d.dest, d.backup, 0600); werr != nil {
						err = fmt.Errorf("%w; restore %s: %v", err, d.dest, werr)
					}
				} else if werr := os.Remove(d.dest); werr != nil && !errors.Is(werr, os.ErrNotExist) {
					err = fmt.Errorf("%w; cleanup %s: %v", err, d.dest, werr)
				}
			}
			return nil, err
		}
		done = append(done, u)
		written = append(written, p.Files[i].Rel)
	}
	return written, nil
}

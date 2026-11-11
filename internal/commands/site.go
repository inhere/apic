package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/goccy/go-yaml"
	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/output"
	"github.com/inhere/apic/internal/spec"
	"github.com/inhere/apic/internal/wiring"
)

type siteAddOptions struct{ spec, def, baseURL string }

func siteCommand(g *globalOpts) *gcli.Command {
	add := &siteAddOptions{}
	return &gcli.Command{Name: "site", Desc: "Manage API sites", Subs: []*gcli.Command{
		{Name: "add", Desc: "Add a site; for authenticated specs configure source.auth and auths in its YAML, then run site refresh", Config: func(c *gcli.Command) {
			c.AddArg("name", "Site name", true)
			c.StrOpt(&add.spec, "spec", "", "", "OpenAPI URL or file")
			c.StrOpt(&add.def, "def", "", "", "Simple definition file")
			c.StrOpt(&add.baseURL, "base-url", "", "", "Default base URL")
		}, Func: func(c *gcli.Command, _ []string) error { return addSite(c.Arg("name").String(), add) }},
		{Name: "rm", Desc: "Remove a managed site", Config: siteNameArg, Func: func(c *gcli.Command, _ []string) error { return removeSite(c.Arg("name").String()) }},
		{Name: "ls", Desc: "List sites", Config: func(c *gcli.Command) { g.bindOutput(&c.Flags) }, Func: func(c *gcli.Command, _ []string) error { return listSites(g.Output) }},
		{Name: "show", Desc: "Show a site", Config: siteNameArg, Func: func(c *gcli.Command, _ []string) error { return showSite(c.Arg("name").String()) }},
		{Name: "refresh", Desc: "Refresh a site index", Config: siteNameArg, Func: func(c *gcli.Command, _ []string) error { return refreshSite(c.Arg("name").String()) }},
	}}
}

func siteNameArg(c *gcli.Command) { c.AddArg("name", "Site name", true) }

func localError(err error) error {
	if err == nil {
		return nil
	}
	var coded *model.CodedError
	if errors.As(err, &coded) {
		return coded
	}
	return model.Errorf(model.ExitLocal, "%v", err)
}

func writeString(w io.Writer, s string) error { _, err := io.WriteString(w, s); return localError(err) }

func siteLookup(d *wiring.Deps, name string) (*model.Site, error) {
	site, ok := d.Cfg.Sites[name]
	if !ok {
		return nil, model.Errorf(model.ExitLocal, "site %q does not exist", name)
	}
	return site, nil
}

func loadIndex(d *wiring.Deps, name string, site *model.Site, refresh bool) (*model.Index, []string, error) {
	envName := site.DefaultEnv
	if selected := d.State.Selections[name].Env; selected != "" {
		envName = selected
	}
	if site.Source.URL != "" && site.Envs[envName].Insecure {
		if err := printWarnings(d, []string{"TLS certificate verification disabled for spec fetch"}); err != nil {
			return nil, nil, err
		}
	}
	fetch, err := wiring.FetchOptions(d, name, site)
	if err != nil {
		return nil, nil, err
	}
	index, warnings, err := spec.Index(context.Background(), name, site.Source, d.Dir, spec.IndexOptions{Refresh: refresh, Client: fetch.Client, Apply: fetch.Apply})
	if err != nil {
		return nil, nil, model.Errorf(model.ExitLocal, "%s", d.Redactor.Redact(err.Error()))
	}
	return index, warnings, nil
}

// loadIndexCached reads the site index from the local cache only, however old
// it is: no request is made and no authentication is built.
func loadIndexCached(d *wiring.Deps, name string, site *model.Site) (*model.Index, error) {
	index, _, err := spec.Index(context.Background(), name, site.Source, d.Dir, spec.IndexOptions{CacheOnly: true})
	return index, err
}

// loadSiteIndex is the raw index plus the site's overlay. Warnings combine
// those of loadIndex and of the overlay; structural overlay errors are local
// errors with redacted text.
func loadSiteIndex(d *wiring.Deps, name string, site *model.Site) (*model.Index, []model.OverlayChange, []string, error) {
	raw, warnings, err := loadIndex(d, name, site, false)
	if err != nil {
		return nil, nil, nil, err
	}
	idx, changes, ovWarnings, err := applySiteOverlay(d, name, raw)
	if err != nil {
		return nil, nil, nil, err
	}
	return idx, changes, append(warnings, ovWarnings...), nil
}

// loadSiteIndexCached is the completion variant: cache only, and any overlay
// error is returned (without warnings) so the caller never falls back to the
// unfiltered index.
func loadSiteIndexCached(d *wiring.Deps, name string, site *model.Site) (*model.Index, error) {
	raw, err := loadIndexCached(d, name, site)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	idx, _, _, err := applySiteOverlay(d, name, raw)
	return idx, err
}

func applySiteOverlay(d *wiring.Deps, name string, raw *model.Index) (*model.Index, []model.OverlayChange, []string, error) {
	ov, err := config.LoadOverlay(d.Dir, name)
	if err == nil {
		var idx *model.Index
		var changes []model.OverlayChange
		var warnings []string
		idx, changes, warnings, err = spec.ApplyOverlay(raw, ov)
		if err == nil {
			return idx, changes, warnings, nil
		}
	}
	return nil, nil, nil, model.Errorf(model.ExitLocal, "%s", d.Redactor.Redact(err.Error()))
}

func printWarnings(d *wiring.Deps, warnings []string) error {
	for _, warning := range warnings {
		if err := writeString(os.Stderr, "warning: "+d.Redactor.Redact(warning)+"\n"); err != nil {
			return err
		}
	}
	return nil
}

func addSite(name string, opt *siteAddOptions) (result error) {
	if (opt.spec == "") == (opt.def == "") {
		return model.Errorf(model.ExitUsage, "provide exactly one of --spec or --def")
	}
	if err := config.ValidateSiteName(name); err != nil {
		return err
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	if _, ok := d.Cfg.Sites[name]; ok {
		return model.Errorf(model.ExitLocal, "site %q already exists", name)
	}
	file := filepath.Join(d.Dir, "sites", name+".yaml")
	if _, err := os.Stat(file); err == nil {
		return model.Errorf(model.ExitLocal, "site %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return localError(err)
	}
	src := model.Source{Type: "openapi"}
	if opt.def != "" {
		src.Type = "simple"
		src.File, err = filepath.Abs(opt.def)
	} else if strings.HasPrefix(opt.spec, "http://") || strings.HasPrefix(opt.spec, "https://") {
		src.URL = opt.spec
	} else {
		src.File, err = filepath.Abs(opt.spec)
	}
	if err != nil {
		return localError(err)
	}
	if err := os.MkdirAll(d.Dir, 0700); err != nil {
		return localError(err)
	}
	tmp, err := os.MkdirTemp(d.Dir, ".apic-index-")
	if err != nil {
		return localError(err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			if result != nil {
				result = model.Errorf(model.ExitLocal, "%v; remove temporary index: %v", result, err)
			} else {
				result = model.Errorf(model.ExitLocal, "remove temporary index: %v", err)
			}
		}
	}()
	tempDeps := *d
	tempDeps.Dir = tmp
	site := &model.Site{Source: src}
	idx, warnings, err := loadIndex(&tempDeps, name, site, true)
	if err != nil {
		return err
	}
	base := opt.baseURL
	if base == "" {
		base = idx.DefaultBaseURL
	}
	if base == "" {
		return model.Errorf(model.ExitLocal, "definition has no base URL; provide --base-url")
	}
	site.DefaultEnv = "default"
	site.Envs = map[string]model.Env{"default": {BaseURL: base}}
	data, err := yaml.Marshal(site)
	if err != nil {
		return localError(err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return localError(err)
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return localError(err)
	}
	created := []string{file}
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, path := range created {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				result = model.Errorf(model.ExitLocal, "%v; cleanup %s: %v", result, path, err)
			}
		}
	}()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return localError(errors.Join(writeErr, closeErr))
	}
	// Publish the already-built cache without overwriting any pre-existing files.
	if src.Type == "openapi" {
		// Orphaned caches do not reserve a site name; they are disposable.
		if err := spec.Remove(d.Dir, name); err != nil {
			return localError(err)
		}
		dest := filepath.Join(d.Dir, "cache", "specs")
		if err := os.MkdirAll(dest, 0700); err != nil {
			return localError(err)
		}
		for _, suffix := range []string{".raw", ".index.json", ".meta.json"} {
			data, err := os.ReadFile(filepath.Join(tmp, "cache", "specs", name+suffix))
			if err != nil {
				return localError(err)
			}
			path := filepath.Join(dest, name+suffix)
			cache, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return localError(err)
			}
			created = append(created, path)
			_, writeErr := cache.Write(data)
			closeErr := cache.Close()
			if writeErr != nil || closeErr != nil {
				return localError(errors.Join(writeErr, closeErr))
			}
		}
	}
	d.State.ManagedSites = append(d.State.ManagedSites, name)
	if err := config.SaveState(d.Dir, d.State); err != nil {
		return err
	}
	committed = true
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	return writeString(os.Stdout, fmt.Sprintf("%s\t%d\t%s\n", name, len(idx.Ops), base))
}

func removeSite(name string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site := d.Cfg.Sites[name]
	if site != nil && filepath.Clean(site.Origin) == filepath.Clean(filepath.Join(d.Dir, "config.yaml")) {
		return model.Errorf(model.ExitLocal, "site %q is in config.yaml; edit it manually", name)
	}
	if !slices.Contains(d.State.ManagedSites, name) {
		return model.Errorf(model.ExitLocal, "site %q is not managed or does not exist; edit it manually", name)
	}
	if err := spec.Remove(d.Dir, name); err != nil {
		return localError(err)
	}
	tokenDir := filepath.Join(d.Dir, "cache", "tokens")
	files, err := os.ReadDir(tokenDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return localError(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), name+".") {
			if err := os.Remove(filepath.Join(tokenDir, file.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return localError(err)
			}
		}
	}
	if err := os.Remove(filepath.Join(d.Dir, "sites", name+".yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return localError(err)
	}
	d.State.ManagedSites = slices.DeleteFunc(d.State.ManagedSites, func(s string) bool { return s == name })
	return config.SaveState(d.Dir, d.State)
}

type siteRow struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	File       string `json:"file"`
	DefaultEnv string `json:"default_env"`
}

func listSites(format string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(d.Cfg.Sites))
	for name := range d.Cfg.Sites {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]siteRow, 0, len(names))
	for _, name := range names {
		s := d.Cfg.Sites[name]
		rows = append(rows, siteRow{name, s.Source.Type, s.Origin, s.DefaultEnv})
	}
	if format == "json" {
		return writeJSON(rows)
	}
	if format != "" {
		return model.Errorf(model.ExitUsage, "unsupported output format %q", format)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tSOURCE\tFILE\tDEFAULT ENV"); err != nil {
		return localError(err)
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Source, r.File, r.DefaultEnv); err != nil {
			return localError(err)
		}
	}
	return localError(w.Flush())
}

func writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return localError(err)
	}
	if err := output.Format(os.Stdout, data, "application/json", "json", false); err != nil {
		return localError(err)
	}
	return writeString(os.Stdout, "\n")
}

func maskCredential(value string, secrets *model.Secrets, short *[]string) string {
	if value == "" {
		return value
	}
	if !strings.Contains(value, "${env:") {
		secrets.Add(value)
		if len(value) < 4 {
			*short = append(*short, value)
		}
	}
	ref := regexp.MustCompile(`\$\{env:[^}]+\}`)
	matches := ref.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return "***"
	}
	var masked strings.Builder
	start := 0
	for _, match := range matches {
		if match[0] > start {
			masked.WriteString("***")
		}
		masked.WriteString(value[match[0]:match[1]])
		start = match[1]
	}
	if start < len(value) {
		masked.WriteString("***")
	}
	return masked.String()
}

func maskBody(value any, secrets *model.Secrets, short *[]string) any {
	switch v := value.(type) {
	case string:
		return maskCredential(v, secrets, short)
	case map[string]any:
		copy := make(map[string]any, len(v))
		for key, child := range v {
			copy[key] = maskBody(child, secrets, short)
		}
		return copy
	case []any:
		copy := make([]any, len(v))
		for i, child := range v {
			copy[i] = maskBody(child, secrets, short)
		}
		return copy
	default:
		if value == nil {
			return nil
		}
		return "***"
	}
}

// maskExec hides argv: only command[0] is shown, followed by "***". Env values
// are masked like other credential fields. The input is not modified.
func maskExec(x model.ExecSpec, secrets *model.Secrets, short *[]string) model.ExecSpec {
	if len(x.Command) > 0 {
		if strings.Contains(x.Command[0], "${env:") {
			x.Command = []string{"***"}
		} else {
			x.Command = []string{x.Command[0], "***"}
		}
	}
	if x.Env != nil {
		env := make(map[string]string, len(x.Env))
		for key, value := range x.Env {
			env[key] = maskCredential(value, secrets, short)
		}
		x.Env = env
	}
	return x
}

func maskedSite(site *model.Site, secrets *model.Secrets, short *[]string) *model.Site {
	copy := *site
	copy.Auths = make(map[string]model.AuthSpec, len(site.Auths))
	for name, auth := range site.Auths {
		auth.Token = maskCredential(auth.Token, secrets, short)
		auth.Username = maskCredential(auth.Username, secrets, short)
		auth.Password = maskCredential(auth.Password, secrets, short)
		auth.Value = maskCredential(auth.Value, secrets, short)
		if auth.Login.Body != nil {
			auth.Login.Body = maskBody(auth.Login.Body, secrets, short).(map[string]any)
		}
		auth.Exec = maskExec(auth.Exec, secrets, short)
		copy.Auths[name] = auth
	}
	return &copy
}

// maskShortScalars preserves YAML syntax while masking short values that the
// shared registry intentionally declines to register.
func maskShortScalars(value any, short []string) any {
	switch v := value.(type) {
	case string:
		ref := regexp.MustCompile(`\$\{env:[^}]+\}`)
		matches := ref.FindAllStringIndex(v, -1)
		mask := func(s string) string {
			for _, secret := range short {
				s = strings.ReplaceAll(s, secret, "***")
			}
			return s
		}
		var out strings.Builder
		start := 0
		for _, match := range matches {
			out.WriteString(mask(v[start:match[0]]))
			out.WriteString(v[match[0]:match[1]])
			start = match[1]
		}
		out.WriteString(mask(v[start:]))
		return out.String()
	case map[string]any:
		for key, child := range v {
			v[key] = maskShortScalars(child, short)
		}
	case []any:
		for i, child := range v {
			v[i] = maskShortScalars(child, short)
		}
	}
	return value
}

func writeYAML(d *wiring.Deps, value any) error {
	var short []string
	switch v := value.(type) {
	case *model.Site:
		value = maskedSite(v, d.Secrets, &short)
	case *model.Config:
		copy := *v
		copy.Sites = make(map[string]*model.Site, len(v.Sites))
		for name, site := range v.Sites {
			copy.Sites[name] = maskedSite(site, d.Secrets, &short)
		}
		value = &copy
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return localError(err)
	}
	ref := regexp.MustCompile(`\$\{env:([^}]+)\}`)
	text := string(data)
	matches := ref.FindAllStringSubmatchIndex(text, -1)
	for _, match := range matches {
		if v, ok := d.Env.Lookup(text[match[2]:match[3]]); ok {
			d.Secrets.Add(v)
			if len(v) > 0 && len(v) < 4 {
				short = append(short, v)
			}
		}
	}
	if len(short) > 0 {
		var document any
		if err := yaml.Unmarshal(data, &document); err != nil {
			return localError(err)
		}
		data, err = yaml.Marshal(maskShortScalars(document, short))
		if err != nil {
			return localError(err)
		}
		text = string(data)
		matches = ref.FindAllStringSubmatchIndex(text, -1)
	}
	// Keep references verbatim, even if a secret happens to occur in a name.
	var out strings.Builder
	start := 0
	for _, match := range matches {
		out.WriteString(d.Redactor.Redact(text[start:match[0]]))
		out.WriteString(text[match[0]:match[1]])
		start = match[1]
	}
	out.WriteString(d.Redactor.Redact(text[start:]))
	return writeString(os.Stdout, out.String())
}
func showSite(name string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	return writeYAML(d, site)
}
func refreshSite(name string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	idx, warnings, err := loadIndex(d, name, site, true)
	if err != nil {
		return err
	}
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	if refreshFailed(warnings) {
		// An explicit refresh that fell back to the stale cache did not succeed.
		return model.Errorf(model.ExitLocal, "refresh of site %q failed; the cached index is stale", name)
	}
	return writeString(os.Stdout, fmt.Sprintf("%s\t%d\n", name, len(idx.Ops)))
}

// refreshFailed reports whether spec.Index fell back to the stale cache.
func refreshFailed(warnings []string) bool {
	for _, w := range warnings {
		if strings.HasPrefix(w, "refresh ") && strings.Contains(w, " failed: ") {
			return true
		}
	}
	return false
}

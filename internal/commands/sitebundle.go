package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/bundle"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
	"github.com/inhere/apic/internal/wiring"
)

type siteExportOptions struct {
	out            string
	allowPlaintext bool
}

type siteImportOptions struct {
	name       string
	force, yes bool
}

func siteExportCommand() *gcli.Command {
	opt := &siteExportOptions{}
	return &gcli.Command{Name: "export", Desc: "Write a site, its definition and overlay as one shareable YAML bundle", Config: func(c *gcli.Command) {
		c.AddArg("name", "Site name", true)
		c.StrOpt(&opt.out, "out", "", "", "Write to this file instead of stdout; an existing file is not overwritten")
		c.BoolOpt(&opt.allowPlaintext, "allow-plaintext", "", false, "Export even if credentials are written in plaintext")
	}, Func: func(c *gcli.Command, _ []string) error { return exportSite(c.Arg("name").String(), opt) }}
}

func siteImportCommand() *gcli.Command {
	opt := &siteImportOptions{}
	return &gcli.Command{Name: "import", Desc: "Install a site bundle from a file, an http(s) URL or - for stdin", Config: func(c *gcli.Command) {
		c.AddArg("source", "Bundle file, URL or -", true)
		c.StrOpt(&opt.name, "name", "", "", "Install under this site name instead of the bundle's")
		c.BoolOpt(&opt.force, "force", "", false, "Overwrite an existing site and its files under sites/, defs/, overlays/")
		c.BoolOpt(&opt.yes, "yes", "y", false, "Accept the commands the bundle would run")
	}, Func: func(c *gcli.Command, _ []string) error { return importSite(c.Arg("source").String(), opt) }}
}

func exportSite(name string, opt *siteExportOptions) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	res, err := bundle.Export(d.Dir, name, site, opt.allowPlaintext)
	if err != nil {
		return err
	}
	for _, note := range res.Notes {
		if err := writeString(os.Stderr, "note: "+note+"\n"); err != nil {
			return err
		}
	}
	if opt.out == "" {
		return writeString(os.Stdout, string(res.Data))
	}
	f, err := os.OpenFile(opt.out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return model.Errorf(model.ExitLocal, "%s already exists; remove it or choose another --out", opt.out)
	}
	if err != nil {
		return localError(err)
	}
	_, werr := f.Write(res.Data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(opt.out)
		return localError(werr)
	}
	return nil
}

// readBundleSource reads a bundle from a file, stdin ("-") or an http(s) URL,
// at most spec.MaxDownload bytes.
func readBundleSource(src string) ([]byte, error) {
	switch {
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		return spec.FetchText(context.Background(), src, nil)
	case src == "-":
		return readLimited(os.Stdin, "stdin")
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, localError(err)
	}
	defer f.Close()
	return readLimited(f, src)
}

func readLimited(r io.Reader, what string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, spec.MaxDownload+1))
	if err != nil {
		return nil, localError(err)
	}
	if len(data) > spec.MaxDownload {
		return nil, model.Errorf(model.ExitLocal, "%s is larger than %d bytes", what, spec.MaxDownload)
	}
	return data, nil
}

// importSite installs a bundle. A bundle that would run commands is refused
// with exit 2 (usage) unless --yes: the commands are listed on stderr first.
func importSite(src string, opt *siteImportOptions) error {
	data, err := readBundleSource(src)
	if err != nil {
		return err
	}
	b, err := bundle.Decode(data)
	if err != nil {
		return err
	}
	plan, err := bundle.Build(b, opt.name)
	if err != nil {
		return err
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	if conflicts := bundle.Conflicts(d.Dir, d.Cfg, plan, opt.force); len(conflicts) > 0 {
		return model.Errorf(model.ExitLocal, "cannot import site %q:\n  %s", plan.Name, strings.Join(conflicts, "\n  "))
	}
	if len(plan.Execs) > 0 {
		if err := writeString(os.Stderr, "this bundle would run these commands when its credentials are used:\n"); err != nil {
			return err
		}
		for _, use := range plan.Execs {
			if err := writeString(os.Stderr, "  "+use.Label+": "+use.Command+"\n"); err != nil {
				return err
			}
		}
		if !opt.yes {
			return model.Errorf(model.ExitUsage, "review the commands above and rerun with --yes to import")
		}
	}
	written, err := bundle.Install(d.Dir, plan)
	if err != nil {
		return err
	}
	// A replaced site may leave a stale index of its old definition behind.
	if err := spec.Remove(d.Dir, plan.Name); err != nil {
		return localError(err)
	}
	if !slices.Contains(d.State.ManagedSites, plan.Name) {
		d.State.ManagedSites = append(d.State.ManagedSites, plan.Name)
		if err := config.SaveState(d.Dir, d.State); err != nil {
			return err
		}
	}
	warnings := append(config.Check(&model.Config{Sites: map[string]*model.Site{plan.Name: plan.Site}}), plan.Warnings...)
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	return writeString(os.Stdout, fmt.Sprintf("imported site %s: %s\n", plan.Name, strings.Join(written, " ")))
}

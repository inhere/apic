package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
	"github.com/inhere/apic/internal/wiring"
)

// checkOverlays validates the overlay of every configured site that has an
// overlay file, against its cached index. failed is true on a structural error
// or a missing cache; unmatched keys are only messages.
func checkOverlays(d *wiring.Deps) (msgs []string, failed bool) {
	names := make([]string, 0, len(d.Cfg.Sites))
	for name := range d.Cfg.Sites {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(d.Dir, "overlays", name+".yaml")); err != nil {
			continue
		}
		idx, err := loadIndexCached(d, name, d.Cfg.Sites[name])
		if err != nil || idx == nil {
			// Not a structural error: report it without failing the check.
			msgs = append(msgs, "site "+name+" overlay: no cached index; run apic site refresh "+name)
			continue
		}
		ov, err := config.LoadOverlay(d.Dir, name)
		if err == nil {
			var ovWarnings []string
			_, _, ovWarnings, err = spec.ApplyOverlay(idx, ov)
			msgs = append(msgs, ovWarnings...)
		}
		if err != nil {
			msgs = append(msgs, "site "+name+" overlay: "+err.Error())
			failed = true
		}
	}
	return msgs, failed
}

func configCommand() *gcli.Command {
	return &gcli.Command{Name: "config", Desc: "Inspect configuration", Subs: []*gcli.Command{
		{Name: "path", Desc: "Show configuration directory", Func: func(*gcli.Command, []string) error {
			dir, err := config.Dir()
			if err != nil {
				return localError(err)
			}
			return writeString(os.Stdout, dir+"\n")
		}},
		{Name: "show", Desc: "Show merged configuration", Func: func(*gcli.Command, []string) error {
			d, err := wiring.Load()
			if err != nil {
				return err
			}
			return writeYAML(d, d.Cfg)
		}},
		{Name: "check", Desc: "Check configuration", Func: func(*gcli.Command, []string) error {
			d, err := wiring.Load()
			if err != nil {
				return err
			}
			warnings := config.Check(d.Cfg)
			ovMsgs, ovFailed := checkOverlays(d)
			warnings = append(warnings, ovMsgs...)
			if len(warnings) == 0 {
				return writeString(os.Stdout, "ok\n")
			}
			for _, warning := range warnings {
				if _, err := fmt.Fprintln(os.Stdout, d.Redactor.Redact(warning)); err != nil {
					return localError(err)
				}
			}
			if ovFailed {
				// Details are already on stdout; the exit code tells scripts.
				return model.Errorf(model.ExitLocal, "overlay check failed")
			}
			return nil
		}},
	}}
}

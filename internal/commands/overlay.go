package commands

import (
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
	"github.com/inhere/apic/internal/wiring"
)

func overlayCommand() *gcli.Command {
	var toStdout bool
	return &gcli.Command{Name: "overlay", Desc: "Manage site overlays", Subs: []*gcli.Command{
		{Name: "init", Desc: "Write a starter overlay file for a site", Config: func(c *gcli.Command) {
			c.AddArg("site", "Site name", true)
			c.BoolOpt(&toStdout, "stdout", "", false, "Print the template instead of writing the file")
		}, Func: func(c *gcli.Command, _ []string) error { return overlayInit(c.Arg("site").String(), toStdout) }},
	}}
}

func overlayInit(name string, toStdout bool) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	// The raw index: the template must list every operation, hidden or renamed.
	idx, warnings, err := loadIndex(d, name, site, false, "")
	if err != nil {
		return err
	}
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	data := config.OverlayTemplate(name, idx)
	if err := checkOverlayTemplate(name, idx, data); err != nil {
		return err
	}
	if toStdout {
		_, err := os.Stdout.Write(data)
		return localError(err)
	}
	path, err := config.WriteOverlay(d.Dir, name, data)
	if err != nil {
		return err
	}
	return writeString(os.Stdout, path+"\n")
}

// checkOverlayTemplate decodes the generated text strictly and applies it to
// the raw index: it must hold exactly one empty entry per operation and change
// nothing, so spec text cannot smuggle entries into the file.
func checkOverlayTemplate(name string, idx *model.Index, data []byte) error {
	bad := func(format string, a ...any) error {
		return model.Errorf(model.ExitLocal, "generated overlay for site %s failed self-check: "+format, append([]any{name}, a...)...)
	}
	ov, err := config.ParseOverlay(name+".yaml", data)
	if err != nil {
		return bad("%v", err)
	}
	if len(ov.Ops) != len(idx.Ops) {
		return bad("%d entries for %d operations", len(ov.Ops), len(idx.Ops))
	}
	_, changes, warnings, err := spec.ApplyOverlay(idx, ov)
	if err != nil {
		return bad("%v", err)
	}
	if len(changes) != 0 || len(warnings) != 0 {
		return bad("%d changes, %d warnings", len(changes), len(warnings))
	}
	return nil
}

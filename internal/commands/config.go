package commands

import (
	"fmt"
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/wiring"
)

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
			if len(warnings) == 0 {
				return writeString(os.Stdout, "ok\n")
			}
			for _, warning := range warnings {
				if _, err := fmt.Fprintln(os.Stdout, d.Redactor.Redact(warning)); err != nil {
					return localError(err)
				}
			}
			return nil
		}},
	}}
}

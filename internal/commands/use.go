package commands

import (
	"fmt"
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/wiring"
)

// bindSelection registers --env and --auth on a static command, sharing the global fields.
func (g *globalOpts) bindSelection(c *gcli.Command) {
	c.StrOpt(&g.Env, "env", "", "", "Environment")
	c.StrOpt(&g.Auth, "auth", "", "", "Authentication")
}

func useCommand(g *globalOpts) *gcli.Command {
	clear := false
	return &gcli.Command{Name: "use", Desc: "Set or show the current environment and authentication of a site", Config: func(c *gcli.Command) {
		c.AddArg("site", "Site name", true)
		g.bindSelection(c)
		c.BoolOpt(&clear, "clear", "", false, "Clear the saved environment and authentication of the site")
	}, Func: func(c *gcli.Command, _ []string) error { return useSite(c.Arg("site").String(), g, clear) }}
}

func useSite(name string, g *globalOpts, clear bool) error {
	if clear && (g.Env != "" || g.Auth != "") {
		return model.Errorf(model.ExitUsage, "--clear cannot be combined with --env or --auth")
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		if !clear {
			return err
		}
		return clearOrphanSelection(d, name, err)
	}
	if clear {
		delete(d.State.Selections, name)
		if err := config.SaveState(d.Dir, d.State); err != nil {
			return err
		}
		return printSelection(d, name, site, d.State.Selections[name])
	}
	saved := d.State.Selections[name]
	if g.Env == "" && g.Auth == "" {
		return printSelection(d, name, site, saved)
	}
	next := saved
	if g.Env != "" {
		if _, ok := site.Envs[g.Env]; !ok {
			return model.Errorf(model.ExitLocal, "site %q has no environment %q", name, g.Env)
		}
		next.Env = g.Env
	}
	if g.Auth != "" {
		if _, ok := site.Auths[g.Auth]; !ok {
			return model.Errorf(model.ExitLocal, "site %q has no auth %q", name, g.Auth)
		}
		next.Auth = g.Auth
	}
	d.State.Selections[name] = next
	if err := config.SaveState(d.Dir, d.State); err != nil {
		return err
	}
	if g.Auth != "" {
		// The note follows the environment as it is after saving.
		env := firstOf(next.Env, site.DefaultEnv)
		if declared := site.Envs[env].Auth; declared != "" && declared != next.Auth {
			note := fmt.Sprintf("note: env %q declares auth %q; the saved auth applies only to environments without one\n", env, declared)
			if err := writeString(os.Stderr, d.Redactor.Redact(note)); err != nil {
				return err
			}
		}
	}
	return printSelection(d, name, site, next)
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return "(none)"
}

// clearOrphanSelection drops a selection left behind by a site that is no
// longer configured. Without a residue, lookupErr (the missing site) is reported.
func clearOrphanSelection(d *wiring.Deps, name string, lookupErr error) error {
	if _, ok := d.State.Selections[name]; !ok {
		return lookupErr
	}
	delete(d.State.Selections, name)
	if err := config.SaveState(d.Dir, d.State); err != nil {
		return err
	}
	return writeString(os.Stdout, "cleared\n")
}

func printSelection(d *wiring.Deps, name string, site *model.Site, saved config.Selection) error {
	env := firstOf(saved.Env, site.DefaultEnv)
	auth := "(none)"
	if n, source := site.EffectiveAuth(env, saved.Auth, ""); n != "" {
		auth = fmt.Sprintf("%s (%s)", n, source)
	}
	return writeString(os.Stdout, d.Redactor.Redact(fmt.Sprintf("site: %s\nenv: %s\nauth: %s\n", name, env, auth)))
}

package commands

import (
	"fmt"
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apix/internal/config"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/wiring"
)

// bindSelection registers --env and --auth on a static command, sharing the global fields.
func (g *globalOpts) bindSelection(c *gcli.Command) {
	c.StrOpt(&g.Env, "env", "", "", "Environment")
	c.StrOpt(&g.Auth, "auth", "", "", "Authentication")
}

func useCommand(g *globalOpts) *gcli.Command {
	return &gcli.Command{Name: "use", Desc: "Set or show the current environment and authentication of a site", Config: func(c *gcli.Command) {
		c.AddArg("site", "Site name", true)
		g.bindSelection(c)
	}, Func: func(c *gcli.Command, _ []string) error { return useSite(c.Arg("site").String(), g) }}
}

func useSite(name string, g *globalOpts) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	saved := d.State.Selections[name]
	if g.Env == "" && g.Auth == "" {
		return printSelection(d, name, firstOf(saved.Env, site.DefaultEnv), firstOf(saved.Auth, site.DefaultAuth))
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
	return printSelection(d, name, firstOf(next.Env, site.DefaultEnv), firstOf(next.Auth, site.DefaultAuth))
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return "(none)"
}

func printSelection(d *wiring.Deps, site, env, auth string) error {
	return writeString(os.Stdout, d.Redactor.Redact(fmt.Sprintf("site: %s\nenv: %s\nauth: %s\n", site, env, auth)))
}

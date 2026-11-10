package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apix/internal/auth"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/wiring"
)

func authCommand(g *globalOpts) *gcli.Command {
	sub := func(name, desc string, run func(*authTarget) error) *gcli.Command {
		return &gcli.Command{Name: name, Desc: desc, Config: func(c *gcli.Command) {
			c.AddArg("site", "Site name", true)
			g.bindSelection(c)
			g.bindVerbose(c)
		}, Func: func(c *gcli.Command, _ []string) error {
			t, err := newAuthTarget(c.Arg("site").String(), g)
			if err != nil {
				return err
			}
			return t.wrap(run(t))
		}}
	}
	return &gcli.Command{Name: "auth", Desc: "Inspect and manage authentication", Subs: []*gcli.Command{
		{Name: "ls", Desc: "List the authentications of a site", Config: func(c *gcli.Command) {
			c.AddArg("site", "Site name", true)
		}, Func: func(c *gcli.Command, _ []string) error { return listAuths(c.Arg("site").String()) }},
		sub("login", "Log in again, replacing the cached token", (*authTarget).login),
		sub("logout", "Drop the cached token", (*authTarget).logout),
		sub("status", "Show whether a token is cached and when it expires", (*authTarget).status),
	}}
}

// bindVerbose registers -v so a login request can be traced (secrets masked).
func (g *globalOpts) bindVerbose(c *gcli.Command) {
	c.BoolOpt(&g.Verbose, "verbose", "v", false, "Print login request details to stderr (secrets masked)")
}

func listAuths(name string) error {
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	current := d.State.Selections[name].Auth
	if current == "" {
		current = site.DefaultAuth
	}
	names := make([]string, 0, len(site.Auths))
	for n := range site.Auths {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tCURRENT")
	for _, n := range names {
		mark := ""
		if n == current {
			mark = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", n, site.Auths[n].Type, mark)
	}
	if err := w.Flush(); err != nil {
		return localError(err)
	}
	return writeString(os.Stdout, d.Redactor.Redact(b.String()))
}

// authTarget is the resolved provider for the auth login/logout/status commands.
type authTarget struct {
	d        *wiring.Deps
	name     string
	sel      wiring.Selection
	spec     model.AuthSpec
	provider *auth.Provider
	timeout  time.Duration
}

func newAuthTarget(name string, g *globalOpts) (*authTarget, error) {
	d, err := wiring.Load()
	if err != nil {
		return nil, err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return nil, err
	}
	timeout, err := parseTimeout(g.Timeout)
	if err != nil {
		return nil, err
	}
	sel, err := wiring.Resolve(d, name, site, g.Env, g.Auth)
	if err != nil {
		return nil, err
	}
	t := &authTarget{d: d, name: name, sel: sel, timeout: timeout}
	if sel.Auth == "" {
		return nil, model.Errorf(model.ExitLocal, "site %q has no auth selected; use --auth or set default_auth", name)
	}
	var verbose io.Writer
	if g.Verbose {
		verbose = os.Stderr
	}
	t.provider, t.spec, err = wiring.NewAuth(d, name, site, sel, verbose)
	if err != nil {
		return nil, redactError(d.Redactor, err)
	}
	return t, nil
}

// wrap masks secrets in an error returned by a subcommand.
func (t *authTarget) wrap(err error) error {
	if err == nil {
		return nil
	}
	return redactError(t.d.Redactor, err)
}

func (t *authTarget) label() string {
	return fmt.Sprintf("%s env=%s auth=%s type=%s", t.name, t.sel.Env, t.sel.Auth, t.spec.Type)
}

func (t *authTarget) print(s string) error {
	return writeString(os.Stdout, t.d.Redactor.Redact(s))
}

func (t *authTarget) login() error {
	ctx := context.Background()
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	if err := t.provider.Refresh(ctx); err != nil {
		if errors.Is(err, model.ErrNoRefresh) {
			return t.print(t.label() + ": no login needed\n")
		}
		return err
	}
	return t.print(t.label() + ": logged in\n")
}

func (t *authTarget) logout() error {
	if err := t.provider.Logout(); err != nil {
		return err
	}
	return t.print(t.label() + ": logged out\n")
}

func (t *authTarget) status() error {
	if t.spec.Type != "login" {
		return t.print(t.label() + ": no login state\n")
	}
	in, exp := t.provider.Status()
	if !in {
		return t.print(t.label() + ": logged out\n")
	}
	if exp.IsZero() {
		return t.print(t.label() + ": logged in, no known expiry\n")
	}
	return t.print(fmt.Sprintf("%s: logged in, expires %s\n", t.label(), exp.Local().Format(time.RFC3339)))
}

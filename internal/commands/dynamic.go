package commands

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/request"
	"github.com/inhere/apic/internal/wiring"
)

// siteContext is everything the generated commands of one site need. It only
// translates the operation index into gcli commands; requests are made by run.
type siteContext struct {
	d    *wiring.Deps
	name string
	site *model.Site
	idx  *model.Index
	g    *globalOpts
	so   *siteOpts
}

// siteDynamicCommand builds the <site> command: a raw path call by itself and
// one subcommand per operation.
func siteDynamicCommand(sc *siteContext) *gcli.Command {
	c := &gcli.Command{
		Name: sc.name,
		Desc: fmt.Sprintf("Call site %s by operation name or raw path", sc.name),
		Config: func(c *gcli.Command) {
			c.AddArg("path", "Raw request path starting with a single '/'", false)
			sc.g.bind(c.SharedOpts())
			sc.so.bind(c.SharedOpts())
		},
		Func: func(c *gcli.Command, _ []string) error { return sc.rawCall(c.Arg("path").String()) },
	}
	for i := range sc.idx.Ops {
		c.Add(operationCommand(sc, &sc.idx.Ops[i]))
	}
	return c
}

// rawCall handles `apic <site> [-X METHOD] <path>`.
func (sc *siteContext) rawCall(path string) error {
	so := sc.so
	if path == "" {
		if so.Method != "" || so.Data != "" || len(so.Fields) > 0 || len(so.Param) > 0 {
			return model.Errorf(model.ExitUsage, "a raw call needs a path starting with '/'")
		}
		return sc.listOperations()
	}
	if !strings.HasPrefix(path, "/") {
		return model.Errorf(model.ExitUsage, "unknown operation %q for site %q", path, sc.name)
	}
	return sc.run(nil, request.Input{Method: so.Method, Path: path})
}

func (sc *siteContext) listOperations() error {
	if o := sc.g.Output; o != "" && o != "json" {
		return model.Errorf(model.ExitUsage, "unsupported output format %q for an operation list", o)
	}
	return printOps(sc.idx, &opsOptions{output: sc.g.Output})
}

// paramFlag records a parameter option value and whether it was given.
type paramFlag struct {
	val string
	set bool
}

func (p *paramFlag) String() string { return p.val }
func (p *paramFlag) Set(s string) error {
	p.val, p.set = s, true
	return nil
}

var optNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]+$`)

// paramOptions returns the parameters that get a --<name> option. A parameter is
// left to -p when its name clashes with a reserved option, appears in more than
// one location, or is not a valid option name.
func paramOptions(op *model.Operation) []model.Param {
	count := map[string]int{}
	for _, p := range op.Params {
		count[p.Name]++
	}
	var out []model.Param
	for _, p := range op.Params {
		if count[p.Name] > 1 || reservedOptNames[p.Name] || !optNamePattern.MatchString(p.Name) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func paramDesc(p model.Param) string {
	desc := fmt.Sprintf("[%s] %s", p.In, p.Desc)
	if p.Required {
		desc += " (required)"
	}
	if len(p.Enum) > 0 {
		desc += " (one of: " + strings.Join(p.Enum, ", ") + ")"
	}
	return strings.TrimSpace(desc)
}

// operationCommand builds the subcommand for one operation; its parameter
// options are registered at runtime and read back by name.
func operationCommand(sc *siteContext, op *model.Operation) *gcli.Command {
	flags := map[string]*paramFlag{}
	return &gcli.Command{
		Name: op.Name,
		Desc: fmt.Sprintf("%s %s %s", op.Method, op.Path, op.Summary),
		Config: func(c *gcli.Command) {
			for _, p := range paramOptions(op) {
				pf := &paramFlag{}
				flags[p.Name] = pf
				c.VarOpt(pf, p.Name, "", paramDesc(p))
			}
		},
		Func: func(c *gcli.Command, _ []string) error {
			if sc.so.Method != "" {
				return model.Errorf(model.ExitUsage, "-X is only valid for raw path calls")
			}
			in := request.Input{Method: op.Method, Params: map[string]string{}}
			for name, pf := range flags {
				if pf.set {
					in.Params[name] = pf.val
				}
			}
			return sc.run(op, in)
		},
	}
}

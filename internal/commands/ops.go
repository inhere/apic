package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/wiring"
)

type opsOptions struct{ tag, match, output string }

func opsCommand(g *globalOpts) *gcli.Command {
	opt := &opsOptions{}
	return &gcli.Command{Name: "ops", Desc: "List operations", Config: func(c *gcli.Command) {
		c.AddArg("site", "Site name", true)
		c.StrOpt(&opt.tag, "tag", "", "", "Filter by tag")
		c.StrOpt(&opt.match, "match", "", "", "Case-insensitive text filter")
		g.bindOutput(&c.Flags)
	}, Func: func(c *gcli.Command, _ []string) error {
		opt.output = g.Output
		return listOps(c.Arg("site").String(), opt)
	}}
}

type operationRow struct {
	Name    string   `json:"name"`
	Method  string   `json:"method"`
	Path    string   `json:"path"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

func listOps(name string, opt *opsOptions) error {
	if opt.output != "" && opt.output != "json" {
		return model.Errorf(model.ExitUsage, "unsupported output format %q", opt.output)
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	idx, warnings, err := loadIndex(d, name, site, false)
	if err != nil {
		return err
	}
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	return printOps(idx, opt)
}

// printOps lists the operations of an index, applying the filters in opt.
func printOps(idx *model.Index, opt *opsOptions) error {
	rows := make([]operationRow, 0)
	for _, op := range idx.Ops {
		if opt.tag != "" {
			found := false
			for _, tag := range op.Tags {
				if tag == opt.tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if opt.match != "" && !strings.Contains(strings.ToLower(op.Name+"\n"+op.Path+"\n"+op.Summary), strings.ToLower(opt.match)) {
			continue
		}
		rows = append(rows, operationRow{op.Name, op.Method, op.Path, op.Summary, op.Tags})
	}
	if opt.output == "json" {
		return writeJSON(rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tMETHOD\tPATH\tSUMMARY"); err != nil {
		return localError(err)
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Method, r.Path, r.Summary); err != nil {
			return localError(err)
		}
	}
	return localError(w.Flush())
}

func descCommand(g *globalOpts) *gcli.Command {
	return &gcli.Command{Name: "desc", Desc: "Describe an operation", Config: func(c *gcli.Command) {
		c.AddArg("site", "Site name", true)
		c.AddArg("op", "Operation name", true)
		g.bindOutput(&c.Flags)
	}, Func: func(c *gcli.Command, _ []string) error {
		return describeOp(c.Arg("site").String(), c.Arg("op").String(), g.Output)
	}}
}
func describeOp(name, operation, format string) error {
	if format != "" && format != "json" {
		return model.Errorf(model.ExitUsage, "unsupported output format %q", format)
	}
	d, err := wiring.Load()
	if err != nil {
		return err
	}
	site, err := siteLookup(d, name)
	if err != nil {
		return err
	}
	idx, warnings, err := loadIndex(d, name, site, false)
	if err != nil {
		return err
	}
	if err := printWarnings(d, warnings); err != nil {
		return err
	}
	var op *model.Operation
	for i := range idx.Ops {
		if idx.Ops[i].Name == operation {
			op = &idx.Ops[i]
			break
		}
	}
	if op == nil {
		return model.Errorf(model.ExitUsage, "unknown operation %q for site %q", operation, name)
	}
	if format == "json" {
		return writeJSON(op)
	}
	basePath := idx.DefaultBasePath
	envName := site.DefaultEnv
	if selected := d.State.Selections[name].Env; selected != "" {
		envName = selected
	}
	if env := site.Envs[envName]; env.BasePath != "" {
		basePath = env.BasePath
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(w, "%s %s%s\n\nPARAMETERS\nNAME\tIN\tTYPE\tREQUIRED\tENUM\tDESCRIPTION\n", op.Method, strings.TrimRight(basePath, "/"), op.Path); err != nil {
		return localError(err)
	}
	for _, p := range op.Params {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\t%s\n", p.Name, p.In, p.Type, p.Required, strings.Join(p.Enum, ","), p.Desc); err != nil {
			return localError(err)
		}
	}
	if op.Body != nil {
		if _, err := fmt.Fprintf(w, "\nBODY %s\n", op.Body.Type); err != nil {
			return localError(err)
		}
		if err := printFields(w, op.Body.Schema.Fields); err != nil {
			return err
		}
	}
	codes := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		if _, err := fmt.Fprintf(w, "\nRESPONSE %s %s\n", code, op.Responses[code].Type); err != nil {
			return localError(err)
		}
		if err := printFields(w, op.Responses[code].Fields); err != nil {
			return err
		}
	}
	return localError(w.Flush())
}
func printFields(w *tabwriter.Writer, fields []model.Field) error {
	for _, f := range fields {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%t\n", f.Name, f.Type, f.Required); err != nil {
			return localError(err)
		}
	}
	return nil
}

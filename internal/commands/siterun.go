package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/output"
	"github.com/inhere/apic/internal/request"
	"github.com/inhere/apic/internal/wiring"
	"golang.org/x/term"
)

// run executes one call: resolve the selection, assemble the call, execute it
// and render the response. It only sequences the steps.
func (sc *siteContext) run(op *model.Operation, in request.Input) error {
	g, so := sc.g, sc.so
	timeout, err := parseTimeout(g.Timeout)
	if err != nil {
		return err
	}
	mode, err := outputMode(sc.d.Cfg.Defaults.Output, g.Output, op)
	if err != nil {
		return err
	}
	var pick *model.Pick
	if g.Pick != "" {
		pk, perr := model.ParsePick(g.Pick)
		if perr != nil {
			return perr
		}
		pick = &pk
	}
	in.P, in.Data, in.Fields, in.Headers = so.Param, so.Data, so.Fields, g.Headers
	in.Forms = so.Forms
	// The pagination checks run before the authentication is resolved, so a usage
	// error never runs a ${exec:} reference or an exec auth command.
	plan, err := sc.planPages(op, &in)
	if err != nil {
		return err
	}
	sel, err := wiring.Resolve(sc.d, sc.name, sc.site, g.Env, g.Auth)
	if err != nil {
		return err
	}
	flags := wiring.CallFlags{DryRun: g.DryRun, Yes: g.Yes, TTY: interactive(), Timeout: timeout, Stderr: os.Stderr, Prompt: promptYesNo}
	if g.Verbose {
		flags.Verbose = os.Stderr
	}
	call, missing, err := wiring.NewCall(sc.d, sc.name, sc.site, sc.idx, op, in, sel, flags)
	if err != nil {
		return sc.redactErr(err)
	}
	for _, name := range missing {
		if err := writeString(os.Stderr, fmt.Sprintf("warning: dry-run: %s is not set; a real call would fail\n", name)); err != nil {
			return err
		}
	}
	if plan != nil {
		return sc.runPages(*call, plan, mode, pick)
	}
	resp, err := request.Execute(context.Background(), *call)
	if err != nil {
		return sc.redactErr(err)
	}
	code, err := sc.render(resp, mode, pick)
	if err != nil {
		return err
	}
	return exitFor(resp, code)
}

// render shows one response (a single call or the merged pages) and returns the
// exit code Render assigned to it.
func (sc *siteContext) render(resp *request.Response, mode string, pick *model.Pick) (int, error) {
	res := &output.Result{Status: resp.Status, Header: resp.Header, Body: resp.Body, Curl: resp.Curl}
	code, err := output.Render(res, output.RenderOptions{Mode: mode, Pick: pick, IncludeHeaders: sc.g.Include, TTY: term.IsTerminal(int(os.Stdout.Fd()))}, os.Stdout, os.Stderr, sc.d.Redactor)
	if err != nil {
		return code, sc.redactErr(err)
	}
	return code, nil
}

// exitFor turns a rendered result into the error that carries the exit code.
func exitFor(resp *request.Response, code int) error {
	if code == model.ExitOK {
		return nil
	}
	if resp.Status == 0 {
		return model.Errorf(code, "write output failed")
	}
	return model.Errorf(code, "HTTP %d %s", resp.Status, http.StatusText(resp.Status))
}

// pagePlan is the pagination decision taken before authentication is resolved.
type pagePlan struct {
	spec     model.Paginate
	first    int
	max      int
	explicit bool
}

// planPages decides whether to walk pages and writes the start page and the size
// into the request input, so an operation that declares page as required still
// passes the parameter checks. It runs before any authentication is resolved.
func (sc *siteContext) planPages(op *model.Operation, in *request.Input) (*pagePlan, error) {
	g := sc.g
	// --max-pages implies --all-pages.
	if !g.AllPages && g.MaxPages == "" {
		return nil, nil
	}
	spec := sc.site.Paginate
	if spec == nil || spec.Page == "" {
		return nil, model.Errorf(model.ExitUsage, "--all-pages: site %q has no paginate configuration", sc.name)
	}
	method := in.Method
	if op != nil {
		method = op.Method
	}
	// An empty method means GET, as in request.Build.
	if m := strings.ToUpper(strings.TrimSpace(method)); m != "" && m != http.MethodGet {
		return nil, model.Errorf(model.ExitUsage, "--all-pages only supports GET requests")
	}
	plan := &pagePlan{spec: *spec, first: spec.StartPage(), max: request.DefaultMaxPages}
	if g.MaxPages != "" {
		n, err := strconv.Atoi(g.MaxPages)
		if err != nil || n <= 0 {
			return nil, model.Errorf(model.ExitUsage, "--max-pages must be a positive integer, got %q", g.MaxPages)
		}
		plan.max, plan.explicit = n, true
	}
	pageVal, gavePage := pageValue(op, *in, spec.Page)
	sizeVal, gaveSize := "", false
	if spec.Size != "" {
		sizeVal, gaveSize = pageValue(op, *in, spec.Size)
	}
	if gavePage {
		n, err := strconv.Atoi(pageVal)
		if err != nil {
			return nil, model.Errorf(model.ExitUsage, "--all-pages: page parameter %q: %q is not an integer", spec.Page, pageVal)
		}
		plan.first = n
	}
	if gaveSize {
		if _, err := strconv.Atoi(sizeVal); err != nil {
			return nil, model.Errorf(model.ExitUsage, "--all-pages: size parameter %q: %q is not an integer", spec.Size, sizeVal)
		}
	}
	if !gavePage {
		in.P = append(in.P, "query."+spec.Page+"="+strconv.Itoa(plan.first))
	}
	if !gaveSize && spec.SizeValue != nil {
		in.P = append(in.P, "query."+spec.Size+"="+strconv.Itoa(*spec.SizeValue))
	}
	return plan, nil
}

// pageValue returns the value the user gave for the query parameter name and
// whether it was given. The sources and their order follow request.Build: the
// generated --<name> option first, then every -p value in order, so the last one
// wins.
func pageValue(op *model.Operation, in request.Input, name string) (string, bool) {
	val, has := "", false
	if inQuery(op, name) {
		if v, ok := in.Params[name]; ok {
			val, has = v, true
		}
	}
	for _, raw := range in.P {
		n, v, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		if n == "query."+name || (n == name && inQuery(op, name)) {
			val, has = v, true
		}
	}
	return val, has
}

// inQuery reports whether a bare -p name=value lands in the query: every name
// does for a raw path call, while an operation must declare it there and nowhere
// else (request.Build rejects the other cases).
func inQuery(op *model.Operation, name string) bool {
	if op == nil {
		return true
	}
	found := false
	for _, p := range op.Params {
		if p.Name != name {
			continue
		}
		if p.In != "query" {
			return false
		}
		found = true
	}
	return found
}

// runPages walks the pages, prints the notes of the run and renders the merged
// array through the same output path as a single call.
func (sc *siteContext) runPages(call request.Call, plan *pagePlan, mode string, pick *model.Pick) error {
	res, err := request.Paginate(context.Background(), call, request.PageOptions{
		Spec: plan.spec, First: plan.first, Max: plan.max, Explicit: plan.explicit,
	})
	if err != nil {
		return sc.redactErr(err)
	}
	var notes []string
	if sc.g.DryRun {
		notes = append(notes, "note: dry-run: --all-pages would request further pages")
	}
	if res.Truncated {
		notes = append(notes, fmt.Sprintf("warning: stopped after %d pages (--max-pages); more may exist", res.Pages))
	}
	if res.FailedAt > 1 {
		notes = append(notes, fmt.Sprintf("note: page %d returned HTTP %d; %d earlier pages discarded", res.FailedAt, res.Resp.Status, res.Discarded))
	}
	for _, n := range notes {
		if err := writeString(os.Stderr, n+"\n"); err != nil {
			return err
		}
	}
	code, err := sc.render(res.Resp, mode, pick)
	if err != nil {
		return err
	}
	if res.FailedAt > 0 && code == model.ExitOK {
		// A 3xx page maps to 0, but a run that stopped early is not a complete result.
		code = model.ExitPostProcess
	}
	return exitFor(res.Resp, code)
}

// redactErr keeps the exit code and masks secrets in the message.
func (sc *siteContext) redactErr(err error) error { return redactError(sc.d.Redactor, err) }

func redactError(red *model.Redactor, err error) error {
	var coded *model.CodedError
	if errors.As(err, &coded) {
		return model.Errorf(coded.C, "%s", red.Redact(coded.Error()))
	}
	return model.Errorf(model.ExitLocal, "%s", red.Redact(err.Error()))
}

// outputMode picks -o, then the operation's declared output, then the config default.
func outputMode(configured, flag string, op *model.Operation) (string, error) {
	mode := flag
	if mode == "" && op != nil {
		mode = op.Output
	}
	if mode == "" {
		mode = configured
	}
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "json", "raw", "yaml", "table":
		return mode, nil
	}
	return "", model.Errorf(model.ExitUsage, "unsupported output format %q (want auto, json, raw, yaml or table)", mode)
}

// interactive reports whether both stdin and stderr are terminals.
func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// promptYesNo asks on stderr and reads one line from stdin.
func promptYesNo(msg string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", msg)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

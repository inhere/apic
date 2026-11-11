package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	sel, err := wiring.Resolve(sc.d, sc.name, sc.site, g.Env, g.Auth)
	if err != nil {
		return err
	}
	in.P, in.Data, in.Fields, in.Headers = so.Param, so.Data, so.Fields, g.Headers
	flags := wiring.CallFlags{DryRun: g.DryRun, Yes: g.Yes, TTY: interactive(), Timeout: timeout, Stderr: os.Stderr, Prompt: promptYesNo}
	if g.Verbose {
		flags.Verbose = os.Stderr
	}
	call, err := wiring.NewCall(sc.d, sc.name, sc.site, sc.idx, op, in, sel, flags)
	if err != nil {
		return sc.redactErr(err)
	}
	resp, err := request.Execute(context.Background(), *call)
	if err != nil {
		return sc.redactErr(err)
	}
	res := &output.Result{Status: resp.Status, Header: resp.Header, Body: resp.Body, Curl: resp.Curl}
	code, rerr := output.Render(res, output.RenderOptions{Mode: mode, Pick: pick, IncludeHeaders: g.Include, TTY: term.IsTerminal(int(os.Stdout.Fd()))}, os.Stdout, os.Stderr, sc.d.Redactor)
	if rerr != nil {
		return sc.redactErr(rerr)
	}
	if code != model.ExitOK {
		if resp.Status == 0 {
			return model.Errorf(code, "write output failed")
		}
		return model.Errorf(code, "HTTP %d %s", resp.Status, http.StatusText(resp.Status))
	}
	return nil
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

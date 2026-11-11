package commands

import (
	"strconv"
	"time"

	"github.com/gookit/gcli/v3/gflag"
	"github.com/inhere/apic/internal/model"
)

// globalOpts holds the global options. The same fields are bound on the app
// flags and on the <site> command's shared options, so the options work before
// the site name, after it and after the operation name.
type globalOpts struct {
	Env, Auth, Output string
	Pick              string
	Include, Verbose  bool
	DryRun, Yes       bool
	Timeout           string
	Headers           []string
}

// siteOpts holds the options owned by the <site> command (-X -d -f -p).
type siteOpts struct {
	Method, Data  string
	Fields, Param []string
}

// reservedOptNames are option names a generated parameter option must not use.
var reservedOptNames = map[string]bool{
	"env": true, "auth": true, "output": true, "include": true, "verbose": true,
	"dry-run": true, "yes": true, "timeout": true, "header": true, "pick": true,
	"help": true, "version": true, "gen-completion": true, "in-completion": true, "api": true,
	"X": true, "d": true, "f": true, "p": true, "o": true, "i": true, "v": true, "H": true, "h": true, "V": true,
}

// bind registers the global options on fs.
func (g *globalOpts) bind(fs *gflag.Flags) {
	fs.StrOpt(&g.Env, "env", "", "", "Environment for this request only")
	fs.StrOpt(&g.Auth, "auth", "", "", "Authentication for this request only")
	fs.StrOpt(&g.Output, "output", "o", "", "Output format: auto, json, raw, yaml or table")
	fs.StrOpt(&g.Pick, "pick", "", "", "Pick a value from a 2xx JSON response by path, e.g. data.items.*.id")
	fs.BoolOpt(&g.Include, "include", "i", false, "Print status line and response headers to stderr")
	fs.BoolOpt(&g.Verbose, "verbose", "v", false, "Print request line and headers to stderr (secrets masked)")
	fs.BoolOpt(&g.DryRun, "dry-run", "", false, "Print the equivalent curl command without sending")
	fs.BoolOpt(&g.Yes, "yes", "", false, "Confirm a write on a protected environment")
	fs.StrOpt(&g.Timeout, "timeout", "", "", "Request timeout, e.g. 10s (plain numbers are seconds)")
	gflag.Opt(fs, &g.Headers, "header", "H", nil, "Extra request header 'Name: value' (repeatable)")
}

// bindOutput registers only -o on a static command, sharing the global field.
func (g *globalOpts) bindOutput(fs *gflag.Flags) {
	fs.StrOpt(&g.Output, "output", "o", "", "Output format")
	// Static commands only list operations or descriptions, which are JSON only.
	fs.Opt("output").Choices = []string{"json"}
}

// bind registers the site-owned options on fs.
func (s *siteOpts) bind(fs *gflag.Flags) {
	fs.StrVar(&s.Method, &gflag.CliOpt{Name: "X", Desc: "HTTP method for a raw path call (default GET)"})
	fs.StrVar(&s.Data, &gflag.CliOpt{Name: "d", Desc: "Request body: @file, - for stdin, or literal"})
	gflag.BindVar(fs, &s.Fields, &gflag.CliOpt{Name: "f", Desc: "Body field k=v or k:=json (repeatable)"})
	gflag.BindVar(fs, &s.Param, &gflag.CliOpt{Name: "p", Desc: "Parameter name=value or <in>.name=value (repeatable)"})
}

// globalSpec lists the global options for Prescan.
func globalSpec() GlobalSpec {
	return GlobalSpec{Flags: map[string]bool{
		"--env": true, "--auth": true, "--output": true, "-o": true,
		"--include": false, "-i": false, "--verbose": false, "-v": false,
		"--dry-run": false, "--yes": false, "--timeout": true, "--pick": true,
		"--header": true, "-H": true,
	}}
}

// parseTimeout accepts a Go duration or a plain number of seconds.
func parseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return d, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n >= 0 {
		return time.Duration(n * float64(time.Second)), nil
	}
	return 0, model.Errorf(model.ExitUsage, "invalid --timeout %q", s)
}

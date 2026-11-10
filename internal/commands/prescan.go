package commands

import (
	"slices"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// GlobalSpec declares the global option names (with their dashes) and whether
// each one takes a value.
type GlobalSpec struct {
	Flags map[string]bool // name -> takes a value
}

// gcliBuiltins are the options gcli itself understands before the command name.
var gcliBuiltins = map[string]bool{"-h": false, "--help": false, "-V": false, "--version": false, "--gen-completion": true, "--in-completion": false}

// scanResult is what the argument scan learns before gcli runs.
type scanResult struct {
	api      string // value of --api, when given
	hasAPI   bool
	first    string // first positional argument (before any rewrite)
	firstAt  int    // index of the first positional in the args without --api; len(rest) when none
	rest     []string
	hasFirst bool
}

// scan walks the leading global options up to the first positional argument.
// Unknown options and --api misuse are usage errors.
func scan(args []string, globals GlobalSpec) (*scanResult, error) {
	res := &scanResult{}
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		name, _, hasEq := strings.Cut(arg, "=")
		if name == "--api" {
			if res.hasAPI {
				return nil, model.Errorf(model.ExitUsage, "--api given more than once")
			}
			switch {
			case hasEq:
				res.api = arg[len("--api="):]
				i++
			case i+1 < len(args):
				res.api = args[i+1]
				i += 2
			default:
				return nil, model.Errorf(model.ExitUsage, "--api requires a site name")
			}
			if res.api == "" || strings.HasPrefix(res.api, "-") {
				return nil, model.Errorf(model.ExitUsage, "--api requires a site name")
			}
			res.hasAPI = true
			continue
		}
		takes, ok := globals.Flags[name]
		if !ok {
			takes, ok = gcliBuiltins[name]
		}
		if !ok {
			return nil, model.Errorf(model.ExitUsage, "unknown option %q", name)
		}
		res.rest = append(res.rest, arg)
		i++
		if takes && !hasEq {
			if i >= len(args) {
				return nil, model.Errorf(model.ExitUsage, "option %s requires a value", name)
			}
			res.rest = append(res.rest, args[i])
			i++
		}
	}
	res.firstAt = len(res.rest)
	if i < len(args) && args[i] != "--" {
		res.first, res.hasFirst = args[i], true
	}
	res.rest = append(res.rest, args[i:]...)
	return res, nil
}

// Prescan validates the leading global options and resolves the target site.
//
// An unknown option before the first positional argument, or an unknown --api
// site, is an ExitUsage error. --api X or --api=X is removed and X is placed
// right after all the global options. The returned site is empty when the first
// positional argument is not a configured site; the arguments are then passed
// to gcli unchanged.
func Prescan(args []string, globals GlobalSpec, sites []string) (site string, rewritten []string, err error) {
	res, err := scan(args, globals)
	if err != nil {
		return "", nil, err
	}
	if res.hasAPI {
		if !slices.Contains(sites, res.api) {
			return "", nil, model.Errorf(model.ExitUsage, "unknown site %q for --api", res.api)
		}
		out := make([]string, 0, len(res.rest)+1)
		out = append(out, res.rest[:res.firstAt]...)
		out = append(out, res.api)
		out = append(out, res.rest[res.firstAt:]...)
		return res.api, out, nil
	}
	if res.hasFirst && slices.Contains(sites, res.first) {
		site = res.first
	}
	return site, args, nil
}

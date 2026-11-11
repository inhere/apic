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

// completionFlag is the gcli option that marks a runtime completion request.
const completionFlag = "--in-completion"

// completionOutputs are the -o values offered at the top level.
var completionOutputs = []string{"auto", "json", "raw", "yaml", "table"}

// completionPlan is what the completion prescan decided.
//
//   - handled: out holds the final candidates, gcli is not called;
//   - silent: nothing is printed and gcli is not called;
//   - otherwise gcli runs with args (starting with --in-completion); site is the
//     targeted site, empty when every site gets a placeholder command.
type completionPlan struct {
	handled, silent bool
	out             []string
	site            string
	args            []string
}

// isCompletionRun reports whether args is a runtime completion request.
func isCompletionRun(args []string) bool { return slices.Contains(args, completionFlag) }

// completionWords returns the words after --in-completion. A bare "=" that
// follows an option is what bash produces for --opt=value (COMP_WORDBREAKS), so
// "X = cur" is folded into "X cur" and a trailing "X =" into "X ”".
func completionWords(args []string) []string {
	at := slices.Index(args, completionFlag)
	if at < 0 {
		return nil
	}
	in := args[at+1:]
	out := make([]string, 0, len(in))
	for i, w := range in {
		if w == "=" && len(out) > 0 && strings.HasPrefix(out[len(out)-1], "-") {
			if i == len(in)-1 {
				out = append(out, "")
			}
			continue
		}
		out = append(out, w)
	}
	return out
}

// matchPrefix returns the sorted, de-duplicated items starting with prefix.
func matchPrefix(items []string, prefix string) []string {
	var out []string
	for _, it := range items {
		if it != "" && strings.HasPrefix(it, prefix) {
			out = append(out, it)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// topLevelOptionNames lists the global option names (long and short) the
// prescan completes itself.
func topLevelOptionNames(globals GlobalSpec) []string {
	names := []string{"--api", "--help", "-h", "--version", "-V", "--gen-completion"}
	for name := range globals.Flags {
		names = append(names, name)
	}
	return names
}

// PrescanCompletion is Prescan for a completion request. The last word is the
// one being typed: it is neither validated nor used to pick the site. Any
// problem makes the plan silent; nothing is ever reported as an error.
func PrescanCompletion(args []string, globals GlobalSpec, sites []string) completionPlan {
	words := completionWords(args)
	if len(words) == 0 {
		return completionPlan{args: []string{completionFlag}}
	}
	cur, prev := words[len(words)-1], words[:len(words)-1]
	var (
		lead    []string // leading global options, without --api
		api     string
		hasAPI  bool
		pending string // a leading value option still waiting for its value (cur)
		i       int
	)
	for i < len(prev) {
		arg := prev[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		name, val, hasEq := strings.Cut(arg, "=")
		if name == "--api" {
			if hasAPI {
				return completionPlan{silent: true}
			}
			hasAPI = true
			i++
			switch {
			case hasEq:
				api = val
			case i < len(prev):
				api = prev[i]
				i++
			default:
				pending = name
			}
			continue
		}
		takes, ok := globals.Flags[name]
		if !ok {
			// Unknown options, and help/version/gen-completion, which gcli would act on.
			return completionPlan{silent: true}
		}
		lead = append(lead, arg)
		i++
		if takes && !hasEq {
			if i >= len(prev) {
				pending = name
				break
			}
			lead = append(lead, prev[i])
			i++
		}
	}
	switch pending {
	case "":
	case "--api":
		return completionPlan{handled: true, out: matchPrefix(sites, cur)}
	case "-o", "--output":
		return completionPlan{handled: true, out: matchPrefix(completionOutputs, cur)}
	default: // --env/--auth need a site; other values have no candidates.
		return completionPlan{handled: true}
	}
	hasFirst := i < len(prev) && prev[i] != "--"
	rest := append(slices.Clone(prev[i:]), cur)
	plan := completionPlan{}
	switch {
	case hasAPI:
		if !slices.Contains(sites, api) {
			return completionPlan{silent: true}
		}
		plan.site = api
		plan.args = append(append(append([]string{completionFlag}, lead...), api), rest...)
		return plan
	case !hasFirst && strings.HasPrefix(cur, "-"):
		return completionPlan{handled: true, out: matchPrefix(topLevelOptionNames(globals), cur)}
	case hasFirst && slices.Contains(sites, prev[i]):
		plan.site = prev[i]
	}
	plan.args = append(append([]string{completionFlag}, lead...), rest...)
	return plan
}

package commands

import (
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/wiring"
)

// BuildInfo contains values supplied at build time.
type BuildInfo struct {
	Version, GitCommit, BuildDate string
}

// managementCommands are the reserved top-level command names.
var managementCommands = []string{"site", "ops", "desc", "use", "auth", "link", "config", "help", "skill", "overlay"}

// Run dispatches the command line and returns its exit code.
//
// Flow: prescan the global options and the target site, load the configuration,
// register the site's commands when a site is targeted, then let gcli run.
func Run(info BuildInfo, args []string) int {
	if args == nil {
		args = os.Args[1:]
	}
	g := &globalOpts{}
	spec := globalSpec()
	if isCompletionRun(args) {
		return runCompletion(info, g, spec, args)
	}
	d, loadErr := wiring.Load()
	if loadErr != nil {
		return runWithoutConfig(info, g, spec, args, loadErr)
	}
	if err := printWarnings(d, d.Cfg.Warnings); err != nil {
		return fail(d.Redactor, err)
	}
	sites := make([]string, 0, len(d.Cfg.Sites))
	for name := range d.Cfg.Sites {
		sites = append(sites, name)
	}
	sort.Strings(sites)
	site, rewritten, env, err := Prescan(args, spec, sites)
	if err != nil {
		return fail(d.Redactor, err)
	}
	app := newApp(info, g, d.Redactor)
	if site != "" {
		sc := &siteContext{d: d, name: site, site: d.Cfg.Sites[site], g: g, so: &siteOpts{}}
		idx, _, warnings, err := loadSiteIndex(d, site, sc.site, env)
		if err != nil {
			return fail(d.Redactor, err)
		}
		if err := printWarnings(d, warnings); err != nil {
			return fail(d.Redactor, err)
		}
		sc.idx = idx
		app.Add(siteDynamicCommand(sc))
	}
	return app.Run(rewritten)
}

// runCompletion answers a runtime completion request. It never prints
// warnings or errors, never touches the network and never builds an
// authentication: the site index comes from the cache only, and a site that is
// not targeted only gets a placeholder command.
func runCompletion(info BuildInfo, g *globalOpts, spec GlobalSpec, args []string) int {
	d, err := wiring.Load()
	var sites []string
	if err == nil {
		for name := range d.Cfg.Sites {
			sites = append(sites, name)
		}
		sort.Strings(sites)
	}
	plan := PrescanCompletion(args, spec, sites)
	switch {
	case plan.silent:
		return 0
	case plan.handled:
		for _, line := range plan.out {
			if err := writeString(os.Stdout, line+"\n"); err != nil {
				return 0
			}
		}
		return 0
	}
	app := newApp(info, g, nil)
	switch {
	case d == nil:
	case plan.site != "":
		sc := &siteContext{d: d, name: plan.site, site: d.Cfg.Sites[plan.site], g: g, so: &siteOpts{}, completion: true}
		// Any overlay error yields an empty index, never the unfiltered one.
		idx, err := loadSiteIndexCached(d, plan.site, sc.site)
		if err != nil || idx == nil {
			idx = &model.Index{}
		}
		sc.idx = idx
		app.Add(siteDynamicCommand(sc))
	default:
		for _, name := range sites {
			app.Add(placeholderCommand(name))
		}
	}
	app.Run(plan.args)
	return 0
}

// runWithoutConfig handles a configuration that failed to load. Management
// commands (config path, site add ...) still run; a call that targets a site
// reports the configuration error with its exit code.
func runWithoutConfig(info BuildInfo, g *globalOpts, spec GlobalSpec, args []string, loadErr error) int {
	res, err := scan(args, spec)
	if err != nil {
		return fail(nil, err)
	}
	candidate := res.first
	if res.hasAPI {
		candidate = res.api
	}
	if candidate != "" && !slices.Contains(managementCommands, candidate) {
		// The target cannot be checked against a broken configuration; report that.
		return fail(nil, loadErr)
	}
	return newApp(info, g, nil).Run(args)
}

// fail prints an error on stderr and returns its exit code.
func fail(red *model.Redactor, err error) int {
	fmt.Fprintln(os.Stderr, "error: "+red.Redact(err.Error()))
	if coded, ok := err.(*model.CodedError); ok {
		return coded.C
	}
	return model.ExitLocal
}

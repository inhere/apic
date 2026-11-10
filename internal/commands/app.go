package commands

import (
	"fmt"
	"os"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/gcli/v3/gevent"
	"github.com/inhere/apix/internal/model"
)

// newApp builds the application with the static commands. red masks secrets in
// any error gcli prints.
func newApp(info BuildInfo, g *globalOpts, red *model.Redactor) *gcli.App {
	return gcli.NewApp(func(app *gcli.App) {
		app.Name = "apix"
		app.Desc = "Call APIs using commands generated from API definitions"
		app.Version = fmt.Sprintf("%s (commit: %s, built: %s)", info.Version, info.GitCommit, info.BuildDate)
		// Replace gcli's default handler, which prints errors on stdout; stdout is
		// reserved for response bodies.
		app.On(gevent.OnAppRunError, func(ctx *gcli.HookCtx) bool {
			if err, ok := ctx.Get("err").(error); ok {
				fmt.Fprintln(os.Stderr, "error: "+red.Redact(err.Error()))
			}
			return false
		})
		g.bind(app.Flags())
		app.Add(siteCommand(g), opsCommand(g), descCommand(g), configCommand(), useCommand(g), authCommand(g), linkCommand(g))
	})
}

package commands

import (
	"fmt"

	"github.com/gookit/gcli/v3"
)

func newApp(info BuildInfo) *gcli.App {
	return gcli.NewApp(func(app *gcli.App) {
		app.Name = "apix"
		app.Desc = "Call APIs using commands generated from API definitions"
		app.Version = fmt.Sprintf("%s (commit: %s, built: %s)", info.Version, info.GitCommit, info.BuildDate)
	})
}

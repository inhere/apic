package commands

import "os"

// BuildInfo contains values supplied at build time.
type BuildInfo struct {
	Version, GitCommit, BuildDate string
}

// Run dispatches the command line and returns its exit code.
func Run(info BuildInfo, args []string) int {
	if args == nil {
		args = os.Args[1:]
	}
	rewritten := prescan(args)
	app := newApp(info)
	return app.Run(rewritten)
}

// prescan leaves arguments unchanged until the dynamic command work in T08.
func prescan(args []string) []string {
	return args
}

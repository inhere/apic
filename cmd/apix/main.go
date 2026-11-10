package main

import (
	"os"

	"github.com/inhere/apix/internal/commands"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	info := commands.BuildInfo{Version: Version, GitCommit: GitCommit, BuildDate: BuildDate}
	os.Exit(commands.Run(info, nil))
}

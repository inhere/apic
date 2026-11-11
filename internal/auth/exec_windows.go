package auth

import (
	"os/exec"
	"path/filepath"
)

func configureCmd(*exec.Cmd) {}

// checkResolved applies the batch-file argument check to the file Windows will
// actually run: a path without an extension is completed through PATHEXT by
// os/exec at start time, so resolve it the same way before checking.
func checkResolved(path string, args []string) error {
	if filepath.Ext(path) == "" {
		if resolved, err := exec.LookPath(path); err == nil {
			path = resolved
		}
	}
	return checkBatchArgs(path, args)
}

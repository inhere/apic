package model

import "fmt"

// ExecPlaceholder stands in for a ${exec:} reference in dry-run output.
const ExecPlaceholder = "<exec>"

// ExecFailure is returned by a command runner: an exec auth command or a
// ${exec:} reference. Code<0 means the command could not start or timed out.
// Reason is already cleaned and redacted, and never holds argv.
type ExecFailure struct {
	Code   int
	Reason string
}

func (e *ExecFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.Code >= 0 {
		return fmt.Sprintf("exit %d: %s", e.Code, e.Reason)
	}
	return e.Reason
}

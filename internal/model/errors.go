package model

import (
	"fmt"

	"github.com/gookit/goutil/errorx"
)

const (
	ExitOK = iota
	ExitLocal
	ExitUsage
	ExitNetwork
	ExitHTTP4xx
	ExitHTTP5xx
	ExitProtected
	ExitPostProcess
)

// CodedError carries an exit code and the underlying error.
// gcli prints errors verbatim, so messages must already be redacted before construction.
type CodedError struct {
	C   int
	Err error
}

var _ errorx.ErrorCoder = (*CodedError)(nil)

func (e *CodedError) Code() int     { return e.C }
func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

// Errorf constructs a coded error from an already redacted message.
func Errorf(code int, format string, a ...any) error {
	return &CodedError{C: code, Err: fmt.Errorf(format, a...)}
}

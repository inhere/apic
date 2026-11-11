//go:build !unix

package request

import "os"

func openFile(path string) (*os.File, error) { return os.Open(path) }

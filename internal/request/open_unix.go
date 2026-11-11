//go:build unix

package request

import (
	"os"
	"syscall"
)

// openFile opens without blocking so a path swapped for a FIFO after the pre-Stat cannot hang apic.
func openFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

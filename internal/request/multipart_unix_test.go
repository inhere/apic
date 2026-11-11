//go:build unix

package request

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

// within fails the test when fn takes longer than 2s; the FIFO has no writer, so a blocking open would hang.
func within(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("operation blocked on a special file")
	}
}

func TestMultipartSpecialFilesUnix(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	assert.NoErr(t, syscall.Mkfifo(fifo, 0o600))
	for name, path := range map[string]string{"fifo": fifo, "devzero": "/dev/zero"} {
		t.Run(name, func(t *testing.T) {
			srv, got := mpServer(t, func(int) int { return 200 })
			pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"f=@" + path + ";filename=x"}})
			assert.NoErr(t, err)
			within(t, func() {
				_, err = Execute(context.Background(), call(t, srv, pr, nil, nil))
			})
			assert.Eq(t, model.ExitLocal, code(err))
			assert.Contains(t, err.Error(), "is not a regular file")
			c := call(t, srv, pr, nil, nil)
			c.DryRun = true
			within(t, func() { _, err = Execute(context.Background(), c) })
			assert.Eq(t, model.ExitLocal, code(err))
			within(t, func() { err = CheckFiles(pr.Parts) })
			assert.Eq(t, model.ExitLocal, code(err))
			assert.Eq(t, 0, len(got()))
		})
	}
}

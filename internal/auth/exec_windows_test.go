//go:build windows

package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

// TestRunCommandRejectsBatchMetacharacters checks that RunCommand applies the
// batch-file argument check to the file Windows actually runs.
func TestRunCommandRejectsBatchMetacharacters(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "echo-args.cmd")
	assert.NoErr(t, os.WriteFile(script, []byte("@echo off\r\necho ran\r\n"), 0o600))

	_, err := RunCommand(context.Background(), []string{"./echo-args.cmd", "a&b"}, dir, os.Environ(), 5*time.Second, nil)
	failure := asExecFailure(t, err)
	assert.Eq(t, -1, failure.Code)
	assert.Eq(t, "argument 1 contains characters not allowed for a batch file", failure.Reason)
	assert.NotContains(t, failure.Reason, "a&b")
}

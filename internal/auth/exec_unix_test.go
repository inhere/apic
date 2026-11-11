//go:build !windows

package auth

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

func pidGone(pid int) bool {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.Require(t, assert.NoErr(t, err))
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	assert.Require(t, assert.NoErr(t, err))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

func TestExecProcessGroup(t *testing.T) {
	f := newExecFixture(t)
	t.Run("timeout kills the grandchild", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "pid")
		s := f.spec("grand="+pidFile, "sleep=10s")
		s.Exec.Timeout = "800ms"
		_, err := apply(t, f.provider(t, s))
		assert.Err(t, err)
		assert.Contains(t, err.Error(), "timed out")
		pid := readPid(t, pidFile)
		assert.True(t, pidGone(pid), "grandchild still alive")
	})
	t.Run("request cancel kills the grandchild", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "pid")
		s := f.spec("grand="+pidFile, "sleep=10s")
		ctx, cancel := ctxTimeout(800 * time.Millisecond)
		defer cancel()
		assert.Err(t, f.provider(t, s).Apply(ctx, newReq(t), false))
		assert.True(t, pidGone(readPid(t, pidFile)))
	})
	t.Run("grandchild holding the pipes does not hang a successful command", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "pid")
		start := time.Now()
		r, err := apply(t, f.provider(t, f.spec("grand="+pidFile, "token=tok-grand-1")))
		readPid(t, pidFile) // registers cleanup
		assert.NoErr(t, err)
		assert.Eq(t, "Bearer tok-grand-1", r.Header.Get("Authorization"))
		assert.True(t, time.Since(start) < 6*time.Second)
	})
}

func ctxTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

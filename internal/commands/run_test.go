package commands_test

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apix/internal/commands"
)

func TestRun(t *testing.T) {
	t.Setenv("APIX_CONFIG_DIR", t.TempDir())
	t.Run("version", func(t *testing.T) {
		assert.Eq(t, 0, commands.Run(commands.BuildInfo{Version: "t"}, []string{"--version"}))
	})
	t.Run("unknown command", func(t *testing.T) {
		assert.True(t, commands.Run(commands.BuildInfo{Version: "t"}, []string{"no-such-cmd"}) != 0)
	})
}

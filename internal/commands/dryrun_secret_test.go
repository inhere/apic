package commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// A dry run must keep masking a cached login token that the auth applies in a
// custom header: only ${exec:} placeholders are left visible.
func TestDryRunMasksCachedTokenInCustomHeader(t *testing.T) {
	_, dir := setup(t)
	file := filepath.Join(dir, "config.yaml")
	raw, err := os.ReadFile(file)
	assert.Require(t, assert.NoErr(t, err))
	cfg := strings.Replace(string(raw), `apply: {header: Authorization, value: "Bearer {token}"}`, `apply: {header: X-Token, value: "{token}"}`, 1)
	assert.Require(t, assert.NoErr(t, os.WriteFile(file, []byte(cfg), 0o600)))

	code, _, errOut := runIO(t, "pet", "list-users", "--auth", "lg")
	assert.Require(t, assert.Eq(t, 0, code, errOut))
	code, out, errOut := runIO(t, "pet", "list-users", "--auth", "lg", "--dry-run", "-v")
	assert.Eq(t, 0, code, errOut)
	assert.Contains(t, out, "X-Token")
	assert.NotContains(t, out+errOut, loginToken)
}

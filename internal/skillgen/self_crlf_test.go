package skillgen

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// A Windows checkout with core.autocrlf turns the embedded template into CRLF;
// the rendered skill must still be LF only.
func TestRenderSelfCRLFTemplate(t *testing.T) {
	orig := selfTemplate
	t.Cleanup(func() { selfTemplate = orig })
	selfTemplate = strings.ReplaceAll(orig, "\n", "\r\n")

	out, err := RenderSelf("apic", "v1.2.3")
	assert.Require(t, assert.NoErr(t, err))
	assert.False(t, strings.Contains(string(out), "\r"))
	lf, err := func() ([]byte, error) { selfTemplate = orig; return RenderSelf("apic", "v1.2.3") }()
	assert.Require(t, assert.NoErr(t, err))
	assert.Eq(t, string(lf), string(out))
}

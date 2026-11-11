package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func mustDecode(t *testing.T, s string) any {
	t.Helper()
	v, err := model.Decode([]byte(s))
	assert.NoError(t, err)
	return v
}

func TestYAML(t *testing.T) {
	t.Run("order and big number", func(t *testing.T) {
		out, err := ToYAML(mustDecode(t, `{"z":12345678901234567891,"a":1.50,"m":"<>&","l":[1,{"k":true}],"n":null}`))
		assert.NoError(t, err)
		s := string(out)
		assert.Contains(t, s, "z: 12345678901234567891\n")
		assert.Contains(t, s, "a: 1.50\n")
		assert.Contains(t, s, "<>&")
		assert.True(t, strings.Index(s, "z:") < strings.Index(s, "a:"))
		assert.True(t, strings.Index(s, "a:") < strings.Index(s, "m:"))
	})
	t.Run("top level array", func(t *testing.T) {
		out, err := ToYAML(mustDecode(t, `[1,2]`))
		assert.NoError(t, err)
		assert.Eq(t, "- 1\n- 2\n", string(out))
	})
}

func TestTable(t *testing.T) {
	t.Run("array of objects union columns", func(t *testing.T) {
		var b bytes.Buffer
		ok, err := Table(&b, mustDecode(t, `[{"id":1,"name":"a"},{"id":2,"extra":{"k":[1]},"name":"bb"}]`), false)
		assert.NoError(t, err)
		assert.True(t, ok)
		lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		assert.Eq(t, 3, len(lines))
		assert.Eq(t, []string{"id", "name", "extra"}, strings.Fields(lines[0]))
		assert.Contains(t, lines[2], `{"k":[1]}`)
	})
	t.Run("single object", func(t *testing.T) {
		var b bytes.Buffer
		ok, err := Table(&b, mustDecode(t, `{"b":1,"a":"x"}`), false)
		assert.NoError(t, err)
		assert.True(t, ok)
		lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		assert.Eq(t, []string{"KEY", "VALUE"}, strings.Fields(lines[0]))
		assert.Eq(t, []string{"b", "1"}, strings.Fields(lines[1]))
		assert.Eq(t, []string{"a", "x"}, strings.Fields(lines[2]))
	})
	t.Run("no table form", func(t *testing.T) {
		for _, in := range []string{`1`, `"s"`, `[]`, `[1,2]`, `[{"a":1},2]`, `[{}]`, `null`} {
			var b bytes.Buffer
			ok, err := Table(&b, mustDecode(t, in), false)
			assert.NoError(t, err)
			assert.False(t, ok, in)
			assert.Eq(t, 0, b.Len())
		}
	})
	t.Run("tty truncates and strips control", func(t *testing.T) {
		long := strings.Repeat("界", 80)
		var b bytes.Buffer
		ok, err := Table(&b, mustDecode(t, `[{"v":"\u001b[31mred\u001b[0m\u0007`+long+`"}]`), true)
		assert.NoError(t, err)
		assert.True(t, ok)
		out := b.String()
		assert.NotContains(t, out, "\x1b")
		assert.NotContains(t, out, "\x07")
		line := strings.Split(out, "\n")[1]
		assert.Eq(t, 60, len([]rune(strings.TrimRight(line, " "))))
		assert.True(t, strings.HasPrefix(line, "red界"))
	})
	t.Run("non tty keeps length but flattens whitespace", func(t *testing.T) {
		long := strings.Repeat("x", 100)
		var b bytes.Buffer
		ok, err := Table(&b, mustDecode(t, `[{"v":"a\nb\tc`+long+`"}]`), false)
		assert.NoError(t, err)
		assert.True(t, ok)
		lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		assert.Eq(t, 2, len(lines))
		assert.Contains(t, lines[1], "a b c"+long)
		assert.NotContains(t, lines[1], "\t")
	})
}

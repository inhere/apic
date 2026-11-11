package model

import (
	"bytes"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func pickJSON(t *testing.T, body, path string) (string, error) {
	t.Helper()
	v, err := Decode([]byte(body))
	assert.NoError(t, err)
	p, err := ParsePick(path)
	assert.NoError(t, err)
	got, err := p.Apply(v)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	assert.NoError(t, EncodeJSON(&out, got, false))
	return out.String(), nil
}

func TestPick(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		for _, p := range []string{"", ".", "a..b", ".a", "a."} {
			_, err := ParsePick(p)
			assert.Err(t, err, p)
			assert.Eq(t, ExitUsage, err.(*CodedError).Code())
		}
		p, err := ParsePick("a.0.*")
		assert.NoError(t, err)
		assert.Eq(t, "a.0.*", p.String())
	})
	t.Run("plain path and index", func(t *testing.T) {
		got, err := pickJSON(t, `{"a":{"b":[10,{"c":"x"}]}}`, "a.b.1.c")
		assert.NoError(t, err)
		assert.Eq(t, `"x"`, got)
		got, err = pickJSON(t, `[{"n":1},{"n":2}]`, "1.n")
		assert.NoError(t, err)
		assert.Eq(t, `2`, got)
	})
	t.Run("star over array", func(t *testing.T) {
		got, err := pickJSON(t, `{"d":[{"id":1},{"id":2}]}`, "d.*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2]`, got)
	})
	t.Run("star over empty array", func(t *testing.T) {
		got, err := pickJSON(t, `{"d":[]}`, "d.*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[]`, got)
	})
	t.Run("star missing field fills null", func(t *testing.T) {
		got, err := pickJSON(t, `[{"id":1},{"x":2},{"id":3}]`, "*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[1,null,3]`, got)
		got, err = pickJSON(t, `[{"a":{"b":1}},{"a":5},{"a":{"c":1}}]`, "*.a.b")
		assert.NoError(t, err)
		assert.Eq(t, `[1,null,null]`, got)
	})
	t.Run("star over object follows key order", func(t *testing.T) {
		got, err := pickJSON(t, `{"z":1,"a":2,"m":3}`, "*")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2,3]`, got)
		got, err = pickJSON(t, `{"z":{"k":1},"a":{"k":2}}`, "*.k")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2]`, got)
	})
	t.Run("errors carry exit 7 and segment", func(t *testing.T) {
		for _, tc := range []struct{ body, path, seg string }{
			{`{"a":1}`, "b", `"b"`},
			{`{"a":[1,2]}`, "a.2", `"2"`},
			{`{"a":[1,2]}`, "a.x", `"x"`},
			{`{"a":1}`, "a.b", `"b"`},
			{`{"a":{"b":1}}`, "a.c", `"c"`},
		} {
			_, err := pickJSON(t, tc.body, tc.path)
			assert.Err(t, err, tc.path)
			assert.Eq(t, ExitPostProcess, err.(*CodedError).Code())
			assert.Contains(t, err.Error(), tc.seg)
		}
		_, err := pickJSON(t, `{"a":{"b":1}}`, "a.c")
		assert.Contains(t, err.Error(), "a")
	})
	t.Run("exit code value", func(t *testing.T) {
		assert.Eq(t, 7, ExitPostProcess)
	})
}

package model

import (
	"bytes"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestDecode(t *testing.T) {
	t.Run("round trip keeps order numbers and html", func(t *testing.T) {
		in := `{"z":12345678901234567891,"a":1.50,"m":"<>&","n":null,"l":[true,{"y":1,"x":2}],"e":{},"f":[]}`
		v, err := Decode([]byte(in))
		assert.NoError(t, err)
		var out bytes.Buffer
		assert.NoError(t, EncodeJSON(&out, v, false))
		assert.Eq(t, in, out.String())
		obj := v.(Object)
		assert.Eq(t, []string{"z", "a", "m", "n", "l", "e", "f"}, obj.Keys)
	})
	t.Run("indent two spaces", func(t *testing.T) {
		v, err := Decode([]byte(`{"a":[1,2],"b":{}}`))
		assert.NoError(t, err)
		var out bytes.Buffer
		assert.NoError(t, EncodeJSON(&out, v, true))
		assert.Eq(t, "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": {}\n}", out.String())
	})
	t.Run("scalars", func(t *testing.T) {
		v, err := Decode([]byte(` "x" `))
		assert.NoError(t, err)
		assert.Eq(t, "x", v)
	})
	t.Run("duplicate key keeps first position", func(t *testing.T) {
		v, err := Decode([]byte(`{"a":1,"b":2,"a":3}`))
		assert.NoError(t, err)
		var out bytes.Buffer
		assert.NoError(t, EncodeJSON(&out, v, false))
		assert.Eq(t, `{"a":3,"b":2}`, out.String())
	})
	t.Run("rejects bad input", func(t *testing.T) {
		for _, in := range []string{``, `   `, `{"a":1} x`, `{"a":1}{"b":2}`, `[1,2`, `{"a":}`, `not json`, `{"a":1}}`} {
			_, err := Decode([]byte(in))
			assert.Err(t, err, in)
		}
	})
}

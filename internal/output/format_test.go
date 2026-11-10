package output

import (
	"bytes"
	"io"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestFormat(t *testing.T) {
	for _, tc := range []struct {
		name, body, mode, want string
		tty                    bool
	}{
		{"auto tty", `{"a":1}`, "auto", "{\n  \"a\": 1\n}", true},
		{"auto pipe", `{"a":1}`, "auto", `{"a":1}`, false},
		{"json", `{"a":1}`, "json", "{\n  \"a\": 1\n}", false},
		{"invalid", "not json", "json", "not json", true},
		{"raw", `{"a":1}`, "raw", `{"a":1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			assert.NoError(t, Format(&out, []byte(tc.body), "application/json", tc.mode, tc.tty))
			assert.Eq(t, tc.want, out.String())
		})
	}
	t.Run("short write", func(t *testing.T) {
		assert.Eq(t, io.ErrShortWrite, Format(shortWriter{}, []byte("body"), "", "raw", false))
	})
}

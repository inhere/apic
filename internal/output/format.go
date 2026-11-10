// Package output formats response bodies for display.
package output

import (
	"bytes"
	"encoding/json"
	"io"
)

// Format writes body, pretty-printing valid JSON when the selected mode requests it.
func Format(w io.Writer, body []byte, contentType string, mode string, tty bool) error {
	if mode == "json" || (mode == "auto" && tty) {
		var formatted bytes.Buffer
		if json.Indent(&formatted, body, "", "  ") == nil {
			body = formatted.Bytes()
		}
	}
	_, err := w.Write(body)
	return err
}

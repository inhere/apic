package output

import (
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/inhere/apic/internal/model"
)

// Result is the output layer's view of a finished request; it keeps this
// package independent of internal/request.
type Result struct {
	Status int
	Header http.Header
	Body   []byte
	Curl   string // non-empty in dry-run mode
}

// Render writes a result and returns the process exit code.
//
// Dry-run results print the already redacted curl command and exit 0. Otherwise
// the body goes to stdout unchanged in meaning (4xx/5xx included), and with
// includeHeaders the status line and redacted headers go to stderr.
func Render(res *Result, mode string, stdout, stderr io.Writer, red *model.Redactor, includeHeaders bool, tty bool) int {
	if res.Curl != "" {
		if _, err := io.WriteString(stdout, res.Curl+"\n"); err != nil {
			return model.ExitLocal
		}
		return model.ExitOK
	}
	if includeHeaders {
		if !writeHeaders(stderr, res, red) {
			return model.ExitLocal
		}
	}
	if err := Format(stdout, res.Body, res.Header.Get("Content-Type"), mode, tty); err != nil {
		return model.ExitLocal
	}
	return ExitCode(res.Status)
}

// ExitCode maps an HTTP status to the exit code contract: 4xx is 4, 5xx is 5, others 0.
func ExitCode(status int) int {
	switch {
	case status >= 500:
		return model.ExitHTTP5xx
	case status >= 400:
		return model.ExitHTTP4xx
	default:
		return model.ExitOK
	}
}

func writeHeaders(w io.Writer, res *Result, red *model.Redactor) bool {
	if _, err := fmt.Fprintf(w, "HTTP %d %s\n", res.Status, http.StatusText(res.Status)); err != nil {
		return false
	}
	hdr := red.RedactHeaders(res.Header)
	names := make([]string, 0, len(hdr))
	for k := range hdr {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		for _, v := range hdr[k] {
			if _, err := fmt.Fprintf(w, "%s: %s\n", k, v); err != nil {
				return false
			}
		}
	}
	return true
}

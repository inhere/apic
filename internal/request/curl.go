package request

import (
	"net/http"
	"sort"
	"strings"

	"github.com/inhere/apic/internal/model"
)

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Curl renders a single-line curl command; everything is redacted before rendering.
func Curl(req *http.Request, body []byte, env model.Env, r *model.Redactor) string {
	var b strings.Builder
	b.WriteString("curl -X " + req.Method)
	if env.Insecure {
		b.WriteString(" -k")
	}
	if env.CAFile != "" {
		b.WriteString(" --cacert " + shQuote(env.CAFile))
	}
	hdr := r.RedactHeaders(req.Header)
	names := make([]string, 0, len(hdr))
	for k := range hdr {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		for _, v := range hdr[k] {
			b.WriteString(" -H " + shQuote(k+": "+v))
		}
	}
	if len(body) > 0 {
		b.WriteString(" --data-raw " + shQuote(r.Redact(string(body))))
	}
	b.WriteString(" " + shQuote(r.Redact(req.URL.String())))
	return b.String()
}

package request

import (
	"net/http"
	"sort"
	"strings"

	"github.com/inhere/apic/internal/model"
)

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Curl renders a single-line curl command; everything is redacted before rendering.
func Curl(req *http.Request, body []byte, parts []Part, env model.Env, r *model.Redactor) string {
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
		if len(parts) > 0 && strings.EqualFold(k, "Content-Type") {
			continue // curl generates the boundary itself
		}
		for _, v := range hdr[k] {
			b.WriteString(" -H " + shQuote(k+": "+v))
		}
	}
	for _, p := range parts {
		if p.File == "" {
			b.WriteString(" --form-string " + shQuote(r.Redact(p.Name)+"="+r.Redact(p.Value)))
			continue
		}
		path := formQuote(r.Redact(p.File))
		b.WriteString(" -F " + shQuote(r.Redact(p.Name)+`=@"`+path+`";type=`+p.Type+`;filename="`+r.Redact(p.Filename)+`"`))
	}
	if len(parts) == 0 && len(body) > 0 {
		b.WriteString(" --data-raw " + shQuote(r.Redact(string(body))))
	}
	b.WriteString(" " + shQuote(r.Redact(req.URL.String())))
	return b.String()
}

// formQuote escapes a path for curl's quoted -F form.
func formQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

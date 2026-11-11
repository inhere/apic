package output

import (
	"bytes"
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

// RenderOptions selects how a result is displayed.
type RenderOptions struct {
	Mode           string // auto, json, raw, yaml, table ("" means auto)
	Pick           *model.Pick
	IncludeHeaders bool
	TTY            bool
}

// Render writes a result and returns the process exit code.
//
// Dry-run results print the already redacted curl command and exit 0. Without
// a pick and with mode auto/json/raw the body goes through Format unchanged.
// A 2xx response with a pick or yaml/table mode is post-processed into a buffer
// first; on failure nothing reaches stdout and (ExitPostProcess, err) is
// returned. 3xx and 4xx/5xx bodies are printed as received.
func Render(res *Result, opt RenderOptions, stdout, stderr io.Writer, red *model.Redactor) (int, error) {
	if res.Curl != "" {
		if _, err := io.WriteString(stdout, res.Curl+"\n"); err != nil {
			return model.ExitLocal, writeErr(err)
		}
		return model.ExitOK, nil
	}
	if opt.IncludeHeaders {
		if !writeHeaders(stderr, res, red) {
			return model.ExitLocal, model.Errorf(model.ExitLocal, "write headers failed")
		}
	}
	mode := opt.Mode
	if mode == "" {
		mode = "auto"
	}
	post := opt.Pick != nil || mode == "yaml" || mode == "table"
	if post && res.Status >= 200 && res.Status < 300 {
		var buf bytes.Buffer
		if err := postProcess(&buf, res, opt, mode, stderr); err != nil {
			return model.ExitPostProcess, model.Errorf(model.ExitPostProcess, "HTTP %d succeeded; %s", res.Status, err)
		}
		if _, err := stdout.Write(buf.Bytes()); err != nil {
			return model.ExitLocal, writeErr(err)
		}
		return model.ExitOK, nil
	}
	if post && res.Status >= 300 && res.Status < 400 {
		if _, err := fmt.Fprintf(stderr, "warning: HTTP %d is not a 2xx response; --pick/-o %s not applied\n", res.Status, mode); err != nil {
			return model.ExitLocal, writeErr(err)
		}
	}
	// Non-post-processed responses (and 3xx/4xx/5xx) keep the phase-one path;
	// yaml/table fall back to the raw body there.
	fmtMode := mode
	if fmtMode == "yaml" || fmtMode == "table" {
		fmtMode = "raw"
	}
	if err := Format(stdout, res.Body, res.Header.Get("Content-Type"), fmtMode, opt.TTY); err != nil {
		return model.ExitLocal, writeErr(err)
	}
	return ExitCode(res.Status), nil
}

func writeErr(err error) error {
	return model.Errorf(model.ExitLocal, "write output failed: %v", err)
}

// postProcess decodes the body, applies the pick and formats the outcome.
func postProcess(buf *bytes.Buffer, res *Result, opt RenderOptions, mode string, stderr io.Writer) error {
	what := "-o " + mode
	if opt.Pick != nil {
		what = "--pick"
	}
	if len(bytes.TrimSpace(res.Body)) == 0 {
		return fmt.Errorf("%s failed: response body is empty", what)
	}
	v, err := model.Decode(res.Body)
	if err != nil {
		return fmt.Errorf("%s failed: response is not JSON: %v", what, err)
	}
	if opt.Pick != nil {
		if v, err = opt.Pick.Apply(v); err != nil {
			return fmt.Errorf("--pick failed: %v", err)
		}
	}
	switch mode {
	case "yaml":
		out, err := ToYAML(v)
		if err != nil {
			return fmt.Errorf("-o yaml failed: %v", err)
		}
		buf.Write(out)
		return nil
	case "table":
		ok, err := Table(buf, v, opt.TTY)
		if err != nil {
			return fmt.Errorf("-o table failed: %v", err)
		}
		if ok {
			return nil
		}
		if _, err := fmt.Fprintln(stderr, "note: value has no table form; printing JSON"); err != nil {
			return err
		}
		return writeJSONValue(buf, v, true)
	case "json":
		return writeJSONValue(buf, v, true)
	}
	// auto / raw: strings are printed bare.
	if s, isStr := v.(string); isStr && opt.Pick != nil {
		buf.WriteString(s + "\n")
		return nil
	}
	return writeJSONValue(buf, v, mode == "auto" && opt.TTY)
}

func writeJSONValue(buf *bytes.Buffer, v any, indent bool) error {
	if err := model.EncodeJSON(buf, v, indent); err != nil {
		return err
	}
	buf.WriteByte('\n')
	return nil
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

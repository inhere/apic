package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

const maxCellRunes = 60

var ansiRe = regexp.MustCompile("\x1b(?:\\[[0-9;?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|[@-Z\\\\-_])")

// Table renders an array of objects (one column per top-level key, in first
// appearance order) or a single object (key and value columns). ok=false means
// v has no table form and the caller should fall back to JSON; nothing is
// written in that case.
func Table(w io.Writer, v any, tty bool) (ok bool, err error) {
	var rows [][]string
	switch t := v.(type) {
	case model.Object:
		rows = append(rows, []string{"KEY", "VALUE"})
		for _, k := range t.Keys {
			rows = append(rows, []string{cell(k, tty), cell(t.Vals[k], tty)})
		}
	case []any:
		if len(t) == 0 {
			return false, nil
		}
		var cols []string
		seen := map[string]bool{}
		for _, e := range t {
			o, isObj := e.(model.Object)
			if !isObj {
				return false, nil
			}
			for _, k := range o.Keys {
				if !seen[k] {
					seen[k] = true
					cols = append(cols, k)
				}
			}
		}
		if len(cols) == 0 {
			return false, nil
		}
		head := make([]string, len(cols))
		for i, c := range cols {
			head[i] = cell(c, tty)
		}
		rows = append(rows, head)
		for _, e := range t {
			o := e.(model.Object)
			row := make([]string, len(cols))
			for i, c := range cols {
				if val, has := o.Vals[c]; has {
					row[i] = cell(val, tty)
				}
			}
			rows = append(rows, row)
		}
	default:
		return false, nil
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return true, err
	}
	_, err = w.Write(buf.Bytes())
	return true, err
}

func cell(v any, tty bool) string {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case json.Number:
		s = t.String()
	case nil:
		s = "null"
	case bool:
		s = fmt.Sprint(t)
	default:
		var b bytes.Buffer
		if err := model.EncodeJSON(&b, v, false); err != nil {
			s = "?"
		} else {
			s = b.String()
		}
	}
	return sanitizeCell(s, tty)
}

func sanitizeCell(s string, tty bool) string {
	if !tty {
		return strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\t' {
				return ' '
			}
			return r
		}, s)
	}
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if rs := []rune(s); len(rs) > maxCellRunes {
		s = string(rs[:maxCellRunes])
	}
	return s
}

package request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// PageOptions describes one pagination run.
type PageOptions struct {
	Spec     model.Paginate
	First    int  // starting page number, already resolved by the caller
	Max      int  // page limit, >0 (falls back to DefaultMaxPages)
	Explicit bool // the limit came from --max-pages
}

// PageResult is the outcome of Paginate. Resp is the last page on success and
// the failing page on a non-2xx status.
type PageResult struct {
	Resp      *Response // success: last page's status/headers, Body is the merged array
	FailedAt  int       // 1-based number of the non-2xx page, 0 when none
	Discarded int       // pages dropped in front of the failed page
	Truncated bool      // stopped at an explicit --max-pages limit
	Pages     int       // pages requested by this run
}

// DefaultMaxPages limits the loop when the caller passes no limit; the caller
// uses it as PageOptions.Max. It is a variable so tests can shrink it.
var DefaultMaxPages = 100

// Paginate walks the list pages of one call: every page uses a copy of
// c.Prepared whose URL carries the page parameter, so the caller's request is
// left untouched. Pages after the first run quietly (no repeated insecure
// warning). A dry run only builds the first page and returns its Response.
func Paginate(ctx context.Context, c Call, opt PageOptions) (*PageResult, error) {
	if c.Prepared == nil || !strings.EqualFold(strings.TrimSpace(c.Prepared.Method), http.MethodGet) {
		return nil, model.Errorf(model.ExitUsage, "--all-pages only supports GET requests")
	}
	if opt.Spec.Page == "" {
		return nil, model.Errorf(model.ExitUsage, "--all-pages: paginate.page is empty")
	}
	max := opt.Max
	if max <= 0 {
		max = DefaultMaxPages
	}

	var (
		merged    = []any{}
		prev      []byte // encoded list of the previous page, for the repeat check
		refLen    int    // item count of the first page, the short-page reference
		total     = -1   // unset until the first page carries it
		last      *Response
		pages     int
		truncated bool
	)
	for {
		page := opt.First + pages
		prep, err := pagePrepared(c.Prepared, opt.Spec.Page, page)
		if err != nil {
			return nil, err
		}
		call := c
		call.Prepared = prep
		call.QuietInsecure = pages > 0

		resp, err := Execute(ctx, call)
		if err != nil {
			return nil, err
		}
		pages++
		if c.DryRun {
			// The caller prints the "would request further pages" note.
			return &PageResult{Resp: resp, Pages: 1}, nil
		}
		if resp.Status < 200 || resp.Status > 299 {
			return &PageResult{Resp: resp, FailedAt: pages, Discarded: pages - 1, Pages: pages}, nil
		}

		list, err := pageItems(resp.Body, opt.Spec.Items, page)
		if err != nil {
			return nil, err
		}
		if pages == 1 && opt.Spec.Total != "" {
			if total, err = pageTotal(resp.Body, opt.Spec.Total, page); err != nil {
				return nil, err
			}
		}
		cur, err := encodeJSON(list)
		if err != nil {
			return nil, err
		}
		// A server ignoring the page parameter returns the same list twice;
		// treating it as the last page would silently duplicate rows.
		if pages > 1 && len(list) > 0 && bytes.Equal(prev, cur) {
			return nil, model.Errorf(model.ExitPostProcess,
				"--all-pages failed: page %d repeats page %d; check paginate.page", page, page-1)
		}
		prev = cur
		merged = append(merged, list...)
		if pages == 1 {
			refLen = len(list)
		}
		last = resp

		if len(list) == 0 {
			break
		}
		if total >= 0 {
			if len(merged) >= total {
				break
			}
		} else if pages > 1 && len(list) < refLen {
			// The first page's length is the reference: servers often cap the
			// requested size, so a short later page is the real end.
			break
		}
		if pages >= max {
			if opt.Explicit {
				truncated = true
				break
			}
			return nil, model.Errorf(model.ExitPostProcess,
				"--all-pages: reached the default limit of %d pages; pass --max-pages to raise or accept it", max)
		}
	}

	body, err := encodeJSON(merged)
	if err != nil {
		return nil, err
	}
	last.Body = body
	return &PageResult{Resp: last, Truncated: truncated, Pages: pages}, nil
}

// pagePrepared returns a copy of p with the page parameter overwritten, so
// each page starts from the caller's original URL.
func pagePrepared(p *Prepared, name string, page int) (*Prepared, error) {
	cp := *p
	cp.Header = p.Header.Clone()
	u, err := url.Parse(p.URL)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "--all-pages: invalid url: %s", urlErrText(err))
	}
	q := u.Query()
	q.Set(name, strconv.Itoa(page))
	u.RawQuery = q.Encode()
	cp.URL = u.String()
	return &cp, nil
}

// pageItems returns the item list of one page; an empty items path means the
// response body itself is the array.
func pageItems(body []byte, items string, page int) ([]any, error) {
	doc, err := model.Decode(body)
	if err != nil {
		return nil, pageErr(page, "response is not JSON")
	}
	if items == "" {
		list, ok := doc.([]any)
		if !ok {
			return nil, pageErr(page, "response is not an array")
		}
		return list, nil
	}
	pick, err := model.ParsePick(items)
	if err != nil {
		// The config loader already rejects such paths; keep the page context.
		return nil, pageErr(page, "items %q: %s", items, err)
	}
	v, err := pick.Apply(doc)
	if err != nil {
		return nil, pageErr(page, "items %q is not an array", items)
	}
	list, ok := v.([]any)
	if !ok {
		return nil, pageErr(page, "items %q is not an array", items)
	}
	return list, nil
}

// pageTotal reads the total row count; it must be a non-negative integer.
func pageTotal(body []byte, totalPath string, page int) (int, error) {
	doc, err := model.Decode(body)
	if err != nil {
		return 0, pageErr(page, "response is not JSON")
	}
	pick, err := model.ParsePick(totalPath)
	if err != nil {
		return 0, pageErr(page, "total %q: %s", totalPath, err)
	}
	v, err := pick.Apply(doc)
	if err != nil {
		return 0, pageErr(page, "total %q is not a non-negative integer", totalPath)
	}
	num, ok := v.(json.Number)
	if !ok {
		return 0, pageErr(page, "total %q is not a non-negative integer", totalPath)
	}
	// "3" parses directly; "3.0" or "3e0" have no fraction part either.
	if n, err := strconv.ParseInt(num.String(), 10, 64); err == nil {
		if n < 0 {
			return 0, pageErr(page, "total %q is not a non-negative integer", totalPath)
		}
		return int(n), nil
	}
	f, err := strconv.ParseFloat(num.String(), 64)
	if err != nil || f < 0 || math.Trunc(f) != f || f > math.MaxInt32 {
		return 0, pageErr(page, "total %q is not a non-negative integer", totalPath)
	}
	return int(f), nil
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := model.EncodeJSON(&buf, v, false); err != nil {
		return nil, model.Errorf(model.ExitLocal, "--all-pages: encode result: %v", err)
	}
	return buf.Bytes(), nil
}

func pageErr(page int, format string, a ...any) error {
	return model.Errorf(model.ExitPostProcess, "--all-pages failed: page %d: %s", page, fmt.Sprintf(format, a...))
}

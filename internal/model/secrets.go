package model

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Secrets holds values that must not be printed, including expanded variables.
type Secrets struct {
	mu     sync.RWMutex
	values map[string]struct{}
}

// Add registers a value; short and empty values are ignored to avoid false positives.
func (s *Secrets) Add(v string) {
	if s == nil || len(v) < 4 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]struct{})
	}
	s.values[v] = struct{}{}
}

func (s *Secrets) snapshot() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make([]string, 0, len(s.values)*2)
	for v := range s.values {
		values = append(values, v)
		if escaped := url.QueryEscape(v); escaped != v {
			values = append(values, escaped, strings.ReplaceAll(escaped, "+", "%20"))
		}
	}
	// Replace longer variants first so overlapping secrets cannot expose suffixes.
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

// Redactor masks credentials in text and headers.
type Redactor struct {
	Secrets *Secrets
}

// Redact replaces registered plaintext and URL-query-encoded secret values.
func (r *Redactor) Redact(text string) string {
	if r == nil {
		return text
	}
	for _, value := range r.Secrets.snapshot() {
		text = strings.ReplaceAll(text, value, "***")
	}
	return text
}

// RedactHeaders returns a deep copy with sensitive headers and known secrets masked.
func (r *Redactor) RedactHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for key, vals := range h {
		copyVals := make([]string, len(vals))
		for i, value := range vals {
			switch {
			case strings.EqualFold(key, "Authorization"), strings.EqualFold(key, "Proxy-Authorization"),
				strings.EqualFold(key, "Cookie"), strings.EqualFold(key, "Set-Cookie"):
				copyVals[i] = "***"
			default:
				copyVals[i] = r.Redact(value)
			}
		}
		out[key] = copyVals
	}
	return out
}

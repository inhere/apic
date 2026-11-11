package skillgen

import (
	"strings"
	"unicode"
)

// markdownSpecials are escaped so spec text cannot form Markdown structure.
const markdownSpecials = "\\`*_[]<>|#"

// Sanitize turns untrusted spec text into a single line of inert Markdown text:
// control characters and newlines become spaces, HTML comment delimiters are
// removed, Markdown specials are escaped, and the result is cut to max runes
// (before escaping, so an escape is never split).
func Sanitize(s string, max int) string {
	var b strings.Builder
	for _, r := range PlainText(s, max) {
		if strings.ContainsRune(markdownSpecials, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// PlainText is Sanitize without Markdown escaping, for values the YAML encoder
// quotes itself (the frontmatter description).
func PlainText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, s)
	for strings.Contains(s, "<!--") || strings.Contains(s, "-->") {
		s = strings.ReplaceAll(s, "<!--", "")
		s = strings.ReplaceAll(s, "-->", "")
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); max >= 0 && len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

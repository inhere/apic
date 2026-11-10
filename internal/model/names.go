package model

import (
	"regexp"
	"strings"
)

var siteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidSiteName reports whether name is safe and valid for a site.
func ValidSiteName(name string) bool {
	return siteNamePattern.MatchString(name) && !strings.HasSuffix(name, "-") && !strings.Contains(name, "--")
}

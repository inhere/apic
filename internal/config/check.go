package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inhere/apix/internal/model"
)

// Check reports credentials written directly into the configuration.
func Check(cfg *model.Config) []string {
	if cfg == nil {
		return nil
	}
	var warnings []string
	for name, site := range cfg.Sites {
		if site == nil {
			continue
		}
		for authName, auth := range site.Auths {
			prefix := fmt.Sprintf("site %s auth %s", name, authName)
			for field, value := range map[string]string{
				"token": auth.Token, "password": auth.Password, "value": auth.Value,
			} {
				if value != "" && !isReference(value) {
					warnings = append(warnings, prefix+" "+field+": plaintext credential")
				}
			}
			checkBody(auth.Login.Body, prefix+" login.body", &warnings)
		}
	}
	sort.Strings(warnings)
	return warnings
}

func checkBody(value any, path string, warnings *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			label := path + "." + key
			lower := strings.ToLower(key)
			if text, ok := child.(string); ok && text != "" &&
				(strings.Contains(lower, "pass") || strings.Contains(lower, "secret") ||
					strings.Contains(lower, "token") || strings.Contains(lower, "key")) && !isReference(text) {
				*warnings = append(*warnings, label+": plaintext credential")
			}
			checkBody(child, label, warnings)
		}
	case []any:
		for i, child := range v {
			checkBody(child, fmt.Sprintf("%s[%d]", path, i), warnings)
		}
	}
}

func isReference(value string) bool {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return false
	}
	inner := value[2 : len(value)-1]
	return inner != "" && !strings.ContainsAny(inner, "{}")
}

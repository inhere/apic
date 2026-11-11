package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/inhere/apic/internal/model"
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
		warnings = append(warnings, checkAuthNames(name, site)...)
		for authName, auth := range site.Auths {
			prefix := fmt.Sprintf("site %s auth %s", name, authName)
			for field, value := range map[string]string{
				"token": auth.Token, "password": auth.Password, "value": auth.Value,
			} {
				if value != "" && !isReference(value) {
					warnings = append(warnings, prefix+" "+field+": plaintext credential")
				}
			}
			for key, value := range auth.Exec.Env {
				if value != "" && !isReference(value) {
					warnings = append(warnings, prefix+" exec.env."+key+": plaintext credential")
				}
			}
			if auth.Type == "exec" {
				command := ""
				if len(auth.Exec.Command) > 0 {
					command = auth.Exec.Command[0]
				}
				if strings.Contains(command, "${env:") {
					command = "***"
				}
				warnings = append(warnings, prefix+": runs "+command)
			}
			checkBody(auth.Login.Body, prefix+" login.body", &warnings)
		}
	}
	sort.Strings(warnings)
	return warnings
}

// checkAuthNames hints at default_auth and envs.*.auth naming an undefined
// auth. Envs are visited in name order; Check sorts the final list anyway.
func checkAuthNames(name string, site *model.Site) []string {
	var warnings []string
	if site.DefaultAuth != "" {
		if _, ok := site.Auths[site.DefaultAuth]; !ok {
			warnings = append(warnings, fmt.Sprintf("site %s: default_auth %q is not defined", name, site.DefaultAuth))
		}
	}
	envNames := make([]string, 0, len(site.Envs))
	for envName := range site.Envs {
		envNames = append(envNames, envName)
	}
	sort.Strings(envNames)
	for _, envName := range envNames {
		auth := site.Envs[envName].Auth
		if auth == "" {
			continue
		}
		if _, ok := site.Auths[auth]; !ok {
			warnings = append(warnings, fmt.Sprintf("site %s env %s: auth %q is not defined", name, envName, auth))
		}
	}
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

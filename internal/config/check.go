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
				warnings = append(warnings, checkExec(prefix+" "+field, value, true)...)
			}
			for key, value := range auth.Exec.Env {
				if value != "" && !isReference(value) {
					warnings = append(warnings, prefix+" exec.env."+key+": plaintext credential")
				}
				warnings = append(warnings, checkExec(prefix+" exec.env."+key, value, true)...)
			}
			for _, field := range execPlainFields(&auth) {
				warnings = append(warnings, checkExec(prefix+" "+field.label, field.value, false)...)
			}
			if auth.Type == "exec" {
				command := ""
				if len(auth.Exec.Command) > 0 {
					command = auth.Exec.Command[0]
				}
				if strings.Contains(command, "${env:") || strings.Contains(command, "${exec:") {
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
			if text, ok := child.(string); ok {
				// Every login body string is a credential value.
				*warnings = append(*warnings, checkExec(label, text, true)...)
			}
			checkBody(child, label, warnings)
		}
	case []any:
		for i, child := range v {
			checkBody(child, fmt.Sprintf("%s[%d]", path, i), warnings)
		}
	}
}

// checkExec hints at one ${exec:} reference: it runs in a credential value, is
// misplaced elsewhere and is malformed when unclosed. The command text is never
// part of the message, same as a resolution failure.
func checkExec(label, value string, credential bool) []string {
	if !strings.Contains(value, "${exec:") {
		return nil
	}
	switch {
	case malformedExec(value):
		return []string{label + ": malformed ${exec:} reference"}
	case !credential:
		return []string{label + ": ${exec:} is only allowed in credential values"}
	default:
		return []string{label + ": runs ${exec:}"}
	}
}

// execPlainFields lists the non-credential values of auth with their labels: a
// ${exec:} in any of them is a position error.
func execPlainFields(auth *model.AuthSpec) []struct{ label, value string } {
	fields := []struct{ label, value string }{
		{"type", auth.Type}, {"username", auth.Username}, {"in", auth.In}, {"name", auth.Name},
		{"login.method", auth.Login.Method}, {"login.path", auth.Login.Path},
		{"login.token.from", auth.Login.Token.From}, {"login.token.path", auth.Login.Token.Path},
		{"login.expires.from", auth.Login.Expires.From}, {"login.expires.path", auth.Login.Expires.Path},
		{"login.apply.header", auth.Login.Apply.Header}, {"login.apply.query", auth.Login.Apply.Query},
		{"login.apply.value", auth.Login.Apply.Value}, {"apply.header", auth.Apply.Header},
		{"apply.query", auth.Apply.Query}, {"apply.value", auth.Apply.Value},
		{"exec.timeout", auth.Exec.Timeout}, {"exec.format", auth.Exec.Format}, {"exec.ttl", auth.Exec.TTL},
		{"exec.token.path", auth.Exec.Token.Path}, {"exec.expires.path", auth.Exec.Expires.Path},
	}
	for i, arg := range auth.Exec.Command {
		fields = append(fields, struct{ label, value string }{fmt.Sprintf("exec.command[%d]", i), arg})
	}
	for i, method := range auth.AllowMethods {
		fields = append(fields, struct{ label, value string }{fmt.Sprintf("allow_methods[%d]", i), method})
	}
	return fields
}

// isReference reports a value resolved at load time, alone or mixed with text.
func isReference(value string) bool {
	return strings.Contains(value, "${env:") || strings.Contains(value, "${exec:")
}

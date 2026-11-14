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
			for _, field := range credentialFields(&auth) {
				warnings = append(warnings, checkExec(prefix+" "+field.label, field.value, true)...)
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
		warnings = append(warnings, PlaintextCredentials(name, site)...)
	}
	sort.Strings(warnings)
	return warnings
}

// PlaintextCredentials reports the credential fields of one site that hold a
// literal value instead of an ${env:} / ${exec:} reference, as the "plaintext
// credential" warnings that Check prints. It is shared by site export.
func PlaintextCredentials(name string, site *model.Site) []string {
	var warnings []string
	if site == nil {
		return nil
	}
	for authName, auth := range site.Auths {
		prefix := fmt.Sprintf("site %s auth %s", name, authName)
		for _, field := range credentialFields(&auth) {
			if field.value != "" && !isReference(field.value) {
				warnings = append(warnings, prefix+" "+field.label+": plaintext credential")
			}
		}
		plaintextBody(auth.Login.Body, prefix+" login.body", &warnings)
	}
	sort.Strings(warnings)
	return warnings
}

// credentialFields lists the scalar credential values of auth (token, password,
// value and exec.env entries, the latter in key order) with their labels.
func credentialFields(auth *model.AuthSpec) []struct{ label, value string } {
	fields := []struct{ label, value string }{
		{"token", auth.Token}, {"password", auth.Password}, {"value", auth.Value},
	}
	keys := make([]string, 0, len(auth.Exec.Env))
	for key := range auth.Exec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fields = append(fields, struct{ label, value string }{"exec.env." + key, auth.Exec.Env[key]})
	}
	return fields
}

// ExecUse is one command a site would run: where it is written and its text.
type ExecUse struct{ Label, Command string }

// ExecCommands lists every command that loading credentials of the site runs:
// each ${exec:...} reference in a credential value and each type exec command.
func ExecCommands(name string, site *model.Site) []ExecUse {
	if site == nil {
		return nil
	}
	authNames := make([]string, 0, len(site.Auths))
	for authName := range site.Auths {
		authNames = append(authNames, authName)
	}
	sort.Strings(authNames)
	var uses []ExecUse
	for _, authName := range authNames {
		auth := site.Auths[authName]
		prefix := fmt.Sprintf("site %s auth %s", name, authName)
		for _, field := range credentialFields(&auth) {
			uses = append(uses, execRefs(prefix+" "+field.label, field.value)...)
		}
		bodyExecs(auth.Login.Body, prefix+" login.body", &uses)
		if auth.Type == "exec" && len(auth.Exec.Command) > 0 {
			uses = append(uses, ExecUse{prefix + " exec.command", strings.Join(auth.Exec.Command, " ")})
		}
	}
	return uses
}

func execRefs(label, value string) []ExecUse {
	var uses []ExecUse
	for _, match := range valueRef.FindAllStringSubmatch(value, -1) {
		if strings.HasPrefix(match[0], "${exec:") {
			uses = append(uses, ExecUse{label, strings.TrimSpace(match[2])})
		}
	}
	return uses
}

func bodyExecs(value any, path string, uses *[]ExecUse) {
	switch v := value.(type) {
	case string:
		*uses = append(*uses, execRefs(path, v)...)
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			bodyExecs(v[key], path+"."+key, uses)
		}
	case []any:
		for i, child := range v {
			bodyExecs(child, fmt.Sprintf("%s[%d]", path, i), uses)
		}
	}
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
			if text, ok := child.(string); ok {
				// Every login body string is a credential value.
				*warnings = append(*warnings, checkExec(label, text, true)...)
			}
			checkBody(child, label, warnings)
		}
	case []any:
		for i, child := range v {
			label := fmt.Sprintf("%s[%d]", path, i)
			if text, ok := child.(string); ok {
				*warnings = append(*warnings, checkExec(label, text, true)...)
			}
			checkBody(child, label, warnings)
		}
	}
}

// plaintextBody reports login body strings that are written literally under a
// credential-looking key.
func plaintextBody(value any, path string, warnings *[]string) {
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
			plaintextBody(child, label, warnings)
		}
	case []any:
		for i, child := range v {
			plaintextBody(child, fmt.Sprintf("%s[%d]", path, i), warnings)
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

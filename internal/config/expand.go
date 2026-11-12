package config

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

// valueRef matches ${env:NAME} and ${exec:command} in one scan, so a replacement
// is never scanned again; ${env:} keeps requiring a non-empty name.
var valueRef = regexp.MustCompile(`\$\{(?:env:([^}]+)|exec:([^}]*))\}`)

// execPlaceholder stands in for ${exec:...} in dry-run output.
const execPlaceholder = "<exec>"

// Expand replaces environment references and registers each resolved secret.
func Expand(s string, env *Env, sec *model.Secrets) (string, error) {
	return expand(s, env, sec, nil)
}

// expand is Expand with an optional tolerant mode: when missing is non-nil, an
// unset variable keeps its ${env:NAME} text, is not registered as a secret and
// is recorded in missing instead of failing. Each string is replaced once.
// A ${exec:} reference stays as written: only credential values run it.
func expand(s string, env *Env, sec *model.Secrets, missing map[string]struct{}) (string, error) {
	return replaceRefs(s, func(ref string) (string, error) {
		if strings.HasPrefix(ref, "${exec:") {
			return ref, nil
		}
		return expandEnvRef(ref, env, sec, missing)
	})
}

// expandCredential is expand for a credential value, where ${exec:} is allowed:
// strict mode runs it and dry-run (missing non-nil) writes the placeholder.
func expandCredential(s string, env *Env, sec *model.Secrets, missing map[string]struct{}, auth, label string) (string, error) {
	if malformedExec(s) {
		return "", malformedExecError(auth, label)
	}
	out, err := replaceRefs(s, func(ref string) (string, error) {
		content, ok := strings.CutPrefix(ref, "${exec:")
		if !ok {
			return expandEnvRef(ref, env, sec, missing)
		}
		if missing != nil {
			return execPlaceholder, nil
		}
		return env.execRef(strings.TrimSuffix(content, "}"), ref, sec, auth, label)
	})
	if err != nil {
		return "", err
	}
	if malformedExec(out) {
		// An expanded variable may hold an unclosed reference itself.
		return "", malformedExecError(auth, label)
	}
	return out, nil
}

// replaceRefs replaces every reference in one pass; the results are not scanned
// again, so neither an expanded variable nor a command output is re-expanded.
func replaceRefs(s string, fn func(ref string) (string, error)) (string, error) {
	var refErr error
	result := valueRef.ReplaceAllStringFunc(s, func(ref string) string {
		if refErr != nil {
			return ref
		}
		value, err := fn(ref)
		if err != nil {
			refErr = err
			return ref
		}
		return value
	})
	if refErr != nil {
		return "", refErr
	}
	return result, nil
}

// expandEnvRef resolves one ${env:NAME} reference.
func expandEnvRef(ref string, env *Env, sec *model.Secrets, missing map[string]struct{}) (string, error) {
	name := ref[len("${env:") : len(ref)-1]
	value, ok := env.Lookup(name)
	if !ok {
		if missing != nil {
			missing[name] = struct{}{}
			return ref, nil
		}
		return ref, model.Errorf(model.ExitLocal, "missing environment variable %q", name)
	}
	sec.Add(value)
	return value, nil
}

// execRef runs one ${exec:...} reference, deduplicated by its full text. Only
// trailing line breaks are stripped from the output: a password may legitimately
// start or end with spaces. A failure is not cached.
func (e *Env) execRef(content, ref string, sec *model.Secrets, auth, label string) (string, error) {
	argv, err := execArgv(content, auth, label)
	if err != nil {
		return ref, err
	}
	if e != nil {
		if value, ok := e.cachedExec(ref); ok {
			sec.Add(value)
			return value, nil
		}
	}
	if e == nil || e.Exec == nil {
		return ref, model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} is not available", auth, label)
	}
	out, runErr := e.Exec(argv)
	if runErr != nil {
		return ref, execFailureError(auth, label, runErr)
	}
	value := strings.TrimRight(out, "\r\n")
	if err := checkExecOutput(value, auth, label); err != nil {
		return ref, err
	}
	e.rememberExec(ref, value)
	sec.Add(value)
	return value, nil
}

// execArgv splits a reference content by whitespace; empty content or a nested
// ${ is malformed. Neither message echoes the command text.
func execArgv(content, auth, label string) ([]string, error) {
	if !strings.Contains(content, "${") {
		if argv := strings.Fields(content); len(argv) > 0 {
			return argv, nil
		}
	}
	return nil, malformedExecError(auth, label)
}

// checkExecOutput accepts one single-line value without control characters.
func checkExecOutput(value, auth, label string) error {
	switch {
	case value == "":
		return model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} produced an empty value", auth, label)
	case strings.ContainsFunc(value, unicode.IsControl):
		return model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} output contains control characters", auth, label)
	}
	return nil
}

// execFailureError formats a runner error without the command name or arguments.
func execFailureError(auth, label string, err error) error {
	var failure *model.ExecFailure
	if !errors.As(err, &failure) {
		return model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} failed: %s", auth, label, err.Error())
	}
	if failure.Code >= 0 {
		return model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} failed (exit %d): %s", auth, label, failure.Code, failure.Reason)
	}
	return model.Errorf(model.ExitLocal, "auth %q %s: ${exec:} failed: %s", auth, label, failure.Reason)
}

func malformedExecError(auth, label string) error {
	return model.Errorf(model.ExitLocal, "auth %q %s: malformed ${exec:} reference", auth, label)
}

// malformedExec reports a ${exec: opening with no closing brace: an unclosed
// reference is left over by the scan.
func malformedExec(s string) bool {
	open := strings.Count(s, "${exec:")
	if open == 0 {
		return false
	}
	closed := 0
	for _, ref := range valueRef.FindAllString(s, -1) {
		if strings.HasPrefix(ref, "${exec:") {
			closed++
		}
	}
	return open > closed
}

// ResolveAuth expands just one identity, without modifying the site's source.
func ResolveAuth(site *model.Site, name string, env *Env, sec *model.Secrets) (model.AuthSpec, error) {
	auth, _, err := resolveAuth(site, name, env, sec, false)
	return auth, err
}

// ResolveAuthDryRun is ResolveAuth, except that unset variables in credential
// fields (Token, Password, Value, Exec.Command, Exec.Env, Login.Body) keep their
// ${env:NAME} text and are not registered as secrets; set variables in the same
// string expand and register as usual. It returns the deduplicated, sorted names
// of the unset variables (control characters removed). Unset variables in any
// other field still return an error.
func ResolveAuthDryRun(site *model.Site, name string, env *Env, sec *model.Secrets) (model.AuthSpec, []string, error) {
	return resolveAuth(site, name, env, sec, true)
}

func resolveAuth(site *model.Site, name string, env *Env, sec *model.Secrets, tolerant bool) (model.AuthSpec, []string, error) {
	var missing map[string]struct{}
	if tolerant {
		missing = map[string]struct{}{}
	}
	auth, err := resolveAuthSpec(site, name, env, sec, missing)
	if err != nil {
		return model.AuthSpec{}, nil, err
	}
	return auth, missingNames(missing), nil
}

// missingNames returns the sorted names with control characters removed.
func missingNames(missing map[string]struct{}) []string {
	if len(missing) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for name := range missing {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, name)
		seen[clean] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveAuthSpec does the work; missing is nil in strict mode.
func resolveAuthSpec(site *model.Site, name string, env *Env, sec *model.Secrets, missing map[string]struct{}) (model.AuthSpec, error) {
	if site == nil {
		return model.AuthSpec{}, model.Errorf(model.ExitLocal, "auth %q: site is nil", name)
	}
	auth, ok := site.Auths[name]
	if !ok {
		return model.AuthSpec{}, model.Errorf(model.ExitLocal, "unknown auth %q", name)
	}
	// Only credential fields register their expanded values as secrets. Names,
	// methods and paths are not secret; registering them would mask unrelated
	// text such as a URL path that happens to contain the username.
	secretFields := []labeledField{{"token", &auth.Token}, {"password", &auth.Password}, {"value", &auth.Value}}
	plainFields := []*string{&auth.Type, &auth.Username, &auth.In, &auth.Name,
		&auth.Login.Method, &auth.Login.Path, &auth.Login.Token.From, &auth.Login.Token.Path,
		&auth.Login.Expires.From, &auth.Login.Expires.Path, &auth.Login.Apply.Header,
		&auth.Login.Apply.Query, &auth.Login.Apply.Value, &auth.Apply.Header,
		&auth.Apply.Query, &auth.Apply.Value, &auth.Exec.Timeout, &auth.Exec.Format,
		&auth.Exec.TTL, &auth.Exec.Token.Path, &auth.Exec.Expires.Path}
	// ${exec:} is allowed in credential values only, so reject every other
	// position before any expansion: a misplaced reference never runs (dry-run too).
	if misplacedExec(plainFields, auth.Exec.Command, auth.AllowMethods) {
		return model.AuthSpec{}, model.Errorf(model.ExitLocal, "auth %q: ${exec:} is only allowed in credential values", name)
	}
	for _, field := range secretFields {
		v, err := expandCredential(*field.value, env, sec, missing, name, field.label)
		if err != nil {
			return model.AuthSpec{}, err
		}
		*field.value = v
	}
	for _, field := range plainFields {
		v, err := expand(*field, env, nil, nil)
		if err != nil {
			return model.AuthSpec{}, err
		}
		*field = v
	}
	if auth.AllowMethods != nil {
		auth.AllowMethods = append([]string(nil), auth.AllowMethods...)
		for i, method := range auth.AllowMethods {
			v, err := expand(method, env, nil, nil)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.AllowMethods[i] = v
		}
	}
	if auth.Exec.Command != nil {
		auth.Exec.Command = append([]string(nil), auth.Exec.Command...)
		for i, arg := range auth.Exec.Command {
			v, err := expand(arg, env, sec, missing)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.Exec.Command[i] = v
		}
	}
	if auth.Exec.Env != nil {
		auth.Exec.Env = make(map[string]string, len(auth.Exec.Env))
		for key, value := range site.Auths[name].Exec.Env {
			v, err := expandCredential(value, env, sec, missing, name, "exec.env."+key)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.Exec.Env[key] = v
		}
	}
	if auth.Login.Body != nil {
		body, err := expandBody(auth.Login.Body, env, sec, missing, name, "login.body")
		if err != nil {
			return model.AuthSpec{}, err
		}
		auth.Login.Body = body.(map[string]any)
	}
	return auth, nil
}

// labeledField is a credential value and the label its ${exec:} errors use.
type labeledField struct {
	label string
	value *string
}

// misplacedExec reports a ${exec:} outside the credential values: the plain
// fields, the exec command and the allowed methods.
func misplacedExec(plain []*string, command, methods []string) bool {
	values := make([]string, 0, len(plain)+len(command)+len(methods))
	for _, field := range plain {
		values = append(values, *field)
	}
	values = append(values, command...)
	values = append(values, methods...)
	for _, value := range values {
		if strings.Contains(value, "${exec:") {
			return true
		}
	}
	return false
}

// expandBody resolves the strings of a login body; every string is a credential
// value, so ${exec:} is allowed there.
func expandBody(value any, env *Env, sec *model.Secrets, missing map[string]struct{}, auth, path string) (any, error) {
	switch v := value.(type) {
	case string:
		return expandCredential(v, env, sec, missing, auth, path)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			resolved, err := expandBody(child, env, sec, missing, auth, path+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			resolved, err := expandBody(child, env, sec, missing, auth, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

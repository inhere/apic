package config

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

var envRef = regexp.MustCompile(`\$\{env:([^}]+)\}`)

// Expand replaces environment references and registers each resolved secret.
func Expand(s string, env *Env, sec *model.Secrets) (string, error) {
	return expand(s, env, sec, nil)
}

// expand is Expand with an optional tolerant mode: when missing is non-nil, an
// unset variable keeps its ${env:NAME} text, is not registered as a secret and
// is recorded in missing instead of failing. Each string is replaced once.
func expand(s string, env *Env, sec *model.Secrets, missing map[string]struct{}) (string, error) {
	var expandErr error
	result := envRef.ReplaceAllStringFunc(s, func(ref string) string {
		name := ref[len("${env:") : len(ref)-1]
		value, ok := env.Lookup(name)
		if !ok {
			if missing != nil {
				missing[name] = struct{}{}
				return ref
			}
			expandErr = model.Errorf(model.ExitLocal, "missing environment variable %q", name)
			return ref
		}
		sec.Add(value)
		return value
	})
	if expandErr != nil {
		return "", expandErr
	}
	return result, nil
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
	secretFields := []*string{&auth.Token, &auth.Password, &auth.Value}
	plainFields := []*string{&auth.Type, &auth.Username, &auth.In, &auth.Name,
		&auth.Login.Method, &auth.Login.Path, &auth.Login.Token.From, &auth.Login.Token.Path,
		&auth.Login.Expires.From, &auth.Login.Expires.Path, &auth.Login.Apply.Header,
		&auth.Login.Apply.Query, &auth.Login.Apply.Value, &auth.Apply.Header,
		&auth.Apply.Query, &auth.Apply.Value, &auth.Exec.Timeout, &auth.Exec.Format,
		&auth.Exec.TTL, &auth.Exec.Token.Path, &auth.Exec.Expires.Path}
	for _, group := range []struct {
		fields  []*string
		sec     *model.Secrets
		missing map[string]struct{}
	}{{secretFields, sec, missing}, {plainFields, nil, nil}} {
		for _, field := range group.fields {
			v, err := expand(*field, env, group.sec, group.missing)
			if err != nil {
				return model.AuthSpec{}, err
			}
			*field = v
		}
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
			v, err := expand(value, env, sec, missing)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.Exec.Env[key] = v
		}
	}
	if auth.Login.Body != nil {
		body, err := expandBody(auth.Login.Body, env, sec, missing)
		if err != nil {
			return model.AuthSpec{}, err
		}
		auth.Login.Body = body.(map[string]any)
	}
	return auth, nil
}

func expandBody(value any, env *Env, sec *model.Secrets, missing map[string]struct{}) (any, error) {
	switch v := value.(type) {
	case string:
		return expand(v, env, sec, missing)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			resolved, err := expandBody(child, env, sec, missing)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			resolved, err := expandBody(child, env, sec, missing)
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

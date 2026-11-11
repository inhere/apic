package config

import (
	"regexp"

	"github.com/inhere/apic/internal/model"
)

var envRef = regexp.MustCompile(`\$\{env:([^}]+)\}`)

// Expand replaces environment references and registers each resolved secret.
func Expand(s string, env *Env, sec *model.Secrets) (string, error) {
	var expandErr error
	result := envRef.ReplaceAllStringFunc(s, func(ref string) string {
		name := ref[len("${env:") : len(ref)-1]
		value, ok := env.Lookup(name)
		if !ok {
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
		fields []*string
		sec    *model.Secrets
	}{{secretFields, sec}, {plainFields, nil}} {
		for _, field := range group.fields {
			v, err := Expand(*field, env, group.sec)
			if err != nil {
				return model.AuthSpec{}, err
			}
			*field = v
		}
	}
	if auth.AllowMethods != nil {
		auth.AllowMethods = append([]string(nil), auth.AllowMethods...)
		for i, method := range auth.AllowMethods {
			v, err := Expand(method, env, nil)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.AllowMethods[i] = v
		}
	}
	if auth.Exec.Command != nil {
		auth.Exec.Command = append([]string(nil), auth.Exec.Command...)
		for i, arg := range auth.Exec.Command {
			v, err := Expand(arg, env, sec)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.Exec.Command[i] = v
		}
	}
	if auth.Exec.Env != nil {
		auth.Exec.Env = make(map[string]string, len(auth.Exec.Env))
		for key, value := range site.Auths[name].Exec.Env {
			v, err := Expand(value, env, sec)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.Exec.Env[key] = v
		}
	}
	if auth.Login.Body != nil {
		body, err := expandBody(auth.Login.Body, env, sec)
		if err != nil {
			return model.AuthSpec{}, err
		}
		auth.Login.Body = body.(map[string]any)
	}
	return auth, nil
}

func expandBody(value any, env *Env, sec *model.Secrets) (any, error) {
	switch v := value.(type) {
	case string:
		return Expand(v, env, sec)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			resolved, err := expandBody(child, env, sec)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			resolved, err := expandBody(child, env, sec)
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

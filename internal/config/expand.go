package config

import (
	"regexp"

	"github.com/inhere/apix/internal/model"
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
	fields := []*string{&auth.Type, &auth.Token, &auth.Username, &auth.Password, &auth.In, &auth.Name, &auth.Value,
		&auth.Login.Method, &auth.Login.Path, &auth.Login.Token.From, &auth.Login.Token.Path,
		&auth.Login.Expires.From, &auth.Login.Expires.Path, &auth.Login.Apply.Header,
		&auth.Login.Apply.Query, &auth.Login.Apply.Value}
	for _, field := range fields {
		v, err := Expand(*field, env, sec)
		if err != nil {
			return model.AuthSpec{}, err
		}
		*field = v
	}
	if auth.AllowMethods != nil {
		auth.AllowMethods = append([]string(nil), auth.AllowMethods...)
		for i, method := range auth.AllowMethods {
			v, err := Expand(method, env, sec)
			if err != nil {
				return model.AuthSpec{}, err
			}
			auth.AllowMethods[i] = v
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

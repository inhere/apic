// Package auth applies authentication to outgoing requests: static credentials
// (none, bearer, basic, apikey) and login flows with a persistent token cache.
// It depends only on internal/model; *Provider satisfies request.Authorizer
// structurally.
package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/inhere/apic/internal/model"
)

// Deps are the collaborators injected by the wiring layer.
type Deps struct {
	Client    *http.Client // login requests; must not follow redirects
	Secrets   *model.Secrets
	Redactor  *model.Redactor
	CacheDir  string
	ConfigDir string    // working directory of exec commands; base for relative command paths
	Verbose   io.Writer // nil disables
	Now       func() time.Time
}

// Provider applies one auth identity to requests.
type Provider struct {
	site, envName, authName string
	spec                    model.AuthSpec
	env                     model.Env
	deps                    Deps

	mu      sync.Mutex
	token   string
	expires time.Time // zero: never expires
}

// New builds a Provider. spec must already be variable-expanded.
func New(site, envName, authName string, spec model.AuthSpec, env model.Env, deps Deps) (*Provider, error) {
	switch spec.Type {
	case "none", "bearer", "basic", "apikey", "login", "exec":
	default:
		return nil, model.Errorf(model.ExitLocal, "auth %q: unknown type %q", authName, spec.Type)
	}
	if spec.Type == "apikey" && spec.In != "header" && spec.In != "query" {
		return nil, model.Errorf(model.ExitLocal, "auth %q: apikey requires in: header or query", authName)
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Client == nil {
		deps.Client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Provider{site: site, envName: envName, authName: authName, spec: spec, env: env, deps: deps}, nil
}

// Apply injects credentials into req. With dry set, no login request is sent.
func (p *Provider) Apply(ctx context.Context, req *http.Request, dry bool) error {
	switch p.spec.Type {
	case "login":
		return p.applyLogin(ctx, req, dry)
	case "exec":
		return p.applyExec(ctx, req, dry)
	default:
		return p.applyStatic(req)
	}
}

// Refresh discards credentials and obtains new ones where possible.
func (p *Provider) Refresh(ctx context.Context) error {
	if p.spec.Type != "login" && p.spec.Type != "exec" {
		return model.ErrNoRefresh
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token, p.expires = "", time.Time{}
	p.removeCache()
	if p.spec.Type == "exec" {
		return p.doExec(ctx)
	}
	return p.doLogin(ctx)
}

func (p *Provider) addSecret(v string) { p.deps.Secrets.Add(v) }

func (p *Provider) redact(s string) string { return p.deps.Redactor.Redact(s) }

// injectToken sets the token on req per ap (login: spec.Login.Apply, exec: spec.Apply).
func (p *Provider) injectToken(req *http.Request, token string, ap model.ApplySpec) {
	value := ap.Value
	if value == "" {
		value = "Bearer {token}"
	}
	value = strings.ReplaceAll(value, "{token}", token)
	p.addSecret(value)
	switch {
	case ap.Query != "":
		q := req.URL.Query()
		q.Set(ap.Query, value)
		req.URL.RawQuery = q.Encode()
	default:
		name := ap.Header
		if name == "" {
			name = "Authorization"
		}
		req.Header.Set(name, value)
	}
}

// Package wiring constructs the dependencies used by commands.
package wiring

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/inhere/apic/internal/auth"
	"github.com/inhere/apic/internal/config"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/request"
	"github.com/inhere/apic/internal/spec"
)

// Deps contains the configuration and shared secret registry for one command.
type Deps struct {
	Dir      string
	Cfg      *model.Config
	Env      *config.Env
	State    *config.State
	Secrets  *model.Secrets
	Redactor *model.Redactor
}

// Load constructs the configuration dependencies.
func Load() (*Deps, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "config directory: %v", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	env, err := config.LoadEnv(dir)
	if err != nil {
		return nil, err
	}
	state, err := config.LoadState(dir)
	if err != nil {
		return nil, err
	}
	secrets := &model.Secrets{}
	return &Deps{Dir: dir, Cfg: cfg, Env: env, State: state, Secrets: secrets, Redactor: &model.Redactor{Secrets: secrets}}, nil
}

// FetchOptions describes the transport and optional static spec authentication.
// Both are constructed lazily, on the first request, so reading a valid cached
// index never depends on TLS files or authentication variables.
func FetchOptions(d *Deps, name string, site *model.Site) (spec.FetchOptions, error) {
	envName := site.DefaultEnv
	if selection := d.State.Selections[name]; selection.Env != "" {
		envName = selection.Env
	}
	env := site.Envs[envName]
	var (
		once   sync.Once
		client *http.Client
		cerr   error
	)
	getClient := func() (*http.Client, error) {
		once.Do(func() { client, cerr = request.NewClient(env) })
		return client, cerr
	}
	opt := spec.FetchOptions{Client: &http.Client{
		Transport:     lazyTransport(func() (http.RoundTripper, error) { c, err := getClient(); return transportOf(c), err }),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	if site.Source.Auth == "" {
		return opt, nil
	}
	var (
		authOnce sync.Once
		provider *auth.Provider
		aerr     error
	)
	opt.Apply = func(req *http.Request) error {
		authOnce.Do(func() {
			var c *http.Client
			if c, aerr = getClient(); aerr != nil {
				return
			}
			identity, err := config.ResolveAuth(site, site.Source.Auth, d.Env, d.Secrets)
			if err != nil {
				aerr = err
				return
			}
			switch identity.Type {
			case "bearer", "basic", "apikey":
			default:
				aerr = model.Errorf(model.ExitLocal, "spec auth %q must be bearer, basic, or apikey", site.Source.Auth)
				return
			}
			provider, aerr = auth.New(name, envName, site.Source.Auth, identity, env, auth.Deps{Client: c, Secrets: d.Secrets, Redactor: d.Redactor, CacheDir: filepath.Join(d.Dir, "cache")})
		})
		if aerr != nil {
			return aerr
		}
		return provider.Apply(req.Context(), req, false)
	}
	return opt, nil
}

// lazyTransport builds the real round tripper on first use.
type lazyTransport func() (http.RoundTripper, error)

func (f lazyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt, err := f()
	if err != nil {
		return nil, err
	}
	return rt.RoundTrip(req)
}

func transportOf(c *http.Client) http.RoundTripper {
	if c == nil {
		return nil
	}
	return c.Transport
}

// Selection holds the resolved environment and authentication names. Auth is
// empty when the site has no authentication selected (treated as none).
type Selection struct{ Env, Auth string }

// Resolve picks the environment and authentication for a site. Precedence is
// the per-call value, then state.yaml, then the site's default_env/default_auth.
func Resolve(d *Deps, name string, site *model.Site, envFlag, authFlag string) (Selection, error) {
	saved := d.State.Selections[name]
	sel := Selection{Env: firstNonEmpty(envFlag, saved.Env, site.DefaultEnv), Auth: firstNonEmpty(authFlag, saved.Auth, site.DefaultAuth)}
	if sel.Env == "" {
		return Selection{}, model.Errorf(model.ExitLocal, "site %q has no environment selected; use --env or set default_env", name)
	}
	if _, ok := site.Envs[sel.Env]; !ok {
		return Selection{}, model.Errorf(model.ExitLocal, "site %q has no environment %q", name, sel.Env)
	}
	if sel.Auth != "" {
		if _, ok := site.Auths[sel.Auth]; !ok {
			return Selection{}, model.Errorf(model.ExitLocal, "site %q has no auth %q", name, sel.Auth)
		}
	}
	return sel, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// CallFlags are the per-invocation settings that are not part of the request itself.
type CallFlags struct {
	DryRun, Yes, TTY bool
	Timeout          time.Duration
	Verbose, Stderr  io.Writer
	Prompt           func(msg string) bool
}

// NewCall builds the request and assembles the request.Call for one operation
// (op is nil for a raw path call).
func NewCall(d *Deps, name string, site *model.Site, idx *model.Index, op *model.Operation, in request.Input, sel Selection, fl CallFlags) (*request.Call, error) {
	env := site.Envs[sel.Env]
	client, err := request.NewClient(env)
	if err != nil {
		return nil, err
	}
	call := &request.Call{
		Env: env, Redactor: d.Redactor, Client: client,
		DryRun: fl.DryRun, Yes: fl.Yes, TTY: fl.TTY, Timeout: fl.Timeout,
		Verbose: fl.Verbose, Stderr: fl.Stderr, Prompt: fl.Prompt,
	}
	// The identity is resolved before the request is built so that every secret
	// it expands is registered before any error message can be printed.
	if sel.Auth != "" {
		provider, spec, err := newProvider(d, name, site, sel, client, fl.Verbose)
		if err != nil {
			return nil, err
		}
		call.Auth, call.Authorizer = spec, provider
	}
	if call.Prepared, err = request.Build(op, in, env, basePath(env, idx)); err != nil {
		return nil, err
	}
	return call, nil
}

// NewAuth builds the provider for the selected authentication, for the auth commands.
func NewAuth(d *Deps, name string, site *model.Site, sel Selection, verbose io.Writer) (*auth.Provider, model.AuthSpec, error) {
	client, err := request.NewClient(site.Envs[sel.Env])
	if err != nil {
		return nil, model.AuthSpec{}, err
	}
	return newProvider(d, name, site, sel, client, verbose)
}

func newProvider(d *Deps, name string, site *model.Site, sel Selection, client *http.Client, verbose io.Writer) (*auth.Provider, model.AuthSpec, error) {
	spec, err := config.ResolveAuth(site, sel.Auth, d.Env, d.Secrets)
	if err != nil {
		return nil, model.AuthSpec{}, err
	}
	provider, err := auth.New(name, sel.Env, sel.Auth, spec, site.Envs[sel.Env], auth.Deps{
		Client: client, Secrets: d.Secrets, Redactor: d.Redactor, CacheDir: filepath.Join(d.Dir, "cache"), ConfigDir: d.Dir, Verbose: verbose,
	})
	if err != nil {
		return nil, model.AuthSpec{}, err
	}
	return provider, spec, nil
}

// basePath prefers the environment override, then the definition's default,
// normalized to no trailing slash so "/" does not double up with operation paths.
func basePath(env model.Env, idx *model.Index) string {
	p := env.BasePath
	if p == "" && idx != nil {
		p = idx.DefaultBasePath
	}
	p = strings.TrimRight(p, "/")
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

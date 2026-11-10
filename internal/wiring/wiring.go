// Package wiring constructs the dependencies used by commands.
package wiring

import (
	"net/http"
	"path/filepath"

	"github.com/inhere/apix/internal/auth"
	"github.com/inhere/apix/internal/config"
	"github.com/inhere/apix/internal/model"
	"github.com/inhere/apix/internal/request"
	"github.com/inhere/apix/internal/spec"
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

// FetchOptions constructs the transport and optional static spec authentication.
func FetchOptions(d *Deps, name string, site *model.Site) (spec.FetchOptions, error) {
	envName := site.DefaultEnv
	if selection := d.State.Selections[name]; selection.Env != "" {
		envName = selection.Env
	}
	env := site.Envs[envName]
	client, err := request.NewClient(env)
	if err != nil {
		return spec.FetchOptions{}, err
	}
	opt := spec.FetchOptions{Client: client}
	if site.Source.Auth == "" {
		return opt, nil
	}
	identity, err := config.ResolveAuth(site, site.Source.Auth, d.Env, d.Secrets)
	if err != nil {
		return spec.FetchOptions{}, err
	}
	switch identity.Type {
	case "bearer", "basic", "apikey":
	default:
		return spec.FetchOptions{}, model.Errorf(model.ExitLocal, "spec auth %q must be bearer, basic, or apikey", site.Source.Auth)
	}
	provider, err := auth.New(name, envName, site.Source.Auth, identity, env, auth.Deps{Client: client, Secrets: d.Secrets, Redactor: d.Redactor, CacheDir: filepath.Join(d.Dir, "cache")})
	if err != nil {
		return spec.FetchOptions{}, err
	}
	opt.Apply = func(req *http.Request) error { return provider.Apply(req.Context(), req, false) }
	return opt, nil
}

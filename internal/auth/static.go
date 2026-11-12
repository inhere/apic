package auth

import (
	"net/http"
	"strings"

	"github.com/inhere/apic/internal/model"
)

// applyStatic sets a static credential. In a dry run a value holding the
// ${exec:} placeholder is not registered: its ${env:} parts are already
// registered by the dry-run resolution, and the placeholder must stay visible
// in the printed curl. Every other value (cached tokens included) still is.
func (p *Provider) applyStatic(req *http.Request, dry bool) error {
	s := p.spec
	addSecret := func(v string) {
		if dry && strings.Contains(v, model.ExecPlaceholder) {
			return
		}
		p.addSecret(v)
	}
	switch s.Type {
	case "none":
	case "bearer":
		addSecret(s.Token)
		req.Header.Set("Authorization", "Bearer "+s.Token)
	case "basic":
		addSecret(s.Password)
		req.SetBasicAuth(s.Username, s.Password)
	case "apikey":
		value := s.Value
		if value == "" {
			value = s.Token
		}
		addSecret(value)
		if s.Name == "" {
			return model.Errorf(model.ExitLocal, "auth %q: apikey requires name", p.authName)
		}
		if s.In == "query" {
			q := req.URL.Query()
			q.Set(s.Name, value)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set(s.Name, value)
		}
	}
	return nil
}

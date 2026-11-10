package auth

import (
	"net/http"

	"github.com/inhere/apic/internal/model"
)

func (p *Provider) applyStatic(req *http.Request) error {
	s := p.spec
	switch s.Type {
	case "none":
	case "bearer":
		p.addSecret(s.Token)
		req.Header.Set("Authorization", "Bearer "+s.Token)
	case "basic":
		p.addSecret(s.Password)
		req.SetBasicAuth(s.Username, s.Password)
	case "apikey":
		value := s.Value
		if value == "" {
			value = s.Token
		}
		p.addSecret(value)
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

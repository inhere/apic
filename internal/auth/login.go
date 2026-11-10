package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gookit/goutil/maputil"
	"github.com/inhere/apix/internal/model"
)

const dryToken = "<login-token>"

func (p *Provider) applyLogin(ctx context.Context, req *http.Request, dry bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && (p.expires.IsZero() || p.deps.Now().Add(expirySkew).Before(p.expires)) {
		p.addSecret(p.token)
		p.injectToken(req, p.token)
		return nil
	}
	if e, ok := p.readCache(); ok {
		p.token, p.expires = e.Token, e.ExpiresAt
		p.addSecret(p.token)
		p.injectToken(req, p.token)
		return nil
	}
	if dry {
		p.injectToken(req, dryToken)
		return nil
	}
	if err := p.doLogin(ctx); err != nil {
		return err
	}
	p.injectToken(req, p.token)
	return nil
}

func (p *Provider) registerBodySecrets(v any) {
	switch t := v.(type) {
	case string:
		p.addSecret(t)
	case map[string]any:
		for _, x := range t {
			p.registerBodySecrets(x)
		}
	case []any:
		for _, x := range t {
			p.registerBodySecrets(x)
		}
	}
}

// doLogin performs the login request and stores the token. Caller holds p.mu.
func (p *Provider) doLogin(ctx context.Context) error {
	lg := p.spec.Login
	method := strings.ToUpper(lg.Method)
	if method == "" {
		method = http.MethodPost
	}
	url := p.env.BaseURL + lg.Path
	p.registerBodySecrets(lg.Body)

	var body io.Reader
	if lg.Body != nil {
		raw, err := json.Marshal(lg.Body)
		if err != nil {
			return model.Errorf(model.ExitLocal, "login: encode body: %s", p.redact(err.Error()))
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return model.Errorf(model.ExitLocal, "login: %s", p.redact(err.Error()))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if w := p.deps.Verbose; w != nil {
		fmt.Fprintln(w, p.redact("> login "+method+" "+url))
	}
	resp, err := p.deps.Client.Do(req)
	if err != nil {
		return model.Errorf(model.ExitNetwork, "login: %s", p.redact(err.Error()))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return model.Errorf(model.ExitNetwork, "login: read response: %s", p.redact(err.Error()))
	}
	switch {
	case resp.StatusCode >= 500:
		return model.Errorf(model.ExitHTTP5xx, "login failed: HTTP %d", resp.StatusCode)
	case resp.StatusCode >= 400:
		return model.Errorf(model.ExitHTTP4xx, "login failed: HTTP %d", resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return model.Errorf(model.ExitLocal, "login failed: unexpected HTTP %d", resp.StatusCode)
	}

	var decoded any
	if lg.Token.From != "header" || lg.Expires.From != "header" {
		_ = json.Unmarshal(data, &decoded) // only needed for body extraction
	}
	tokenVal, ok := extract(lg.Token, resp.Header, decoded)
	token := fmt.Sprint(tokenVal)
	if !ok || token == "" || token == "<nil>" {
		return model.Errorf(model.ExitLocal, "login: token not found in response (from %s, path %q)", orBody(lg.Token.From), lg.Token.Path)
	}
	p.addSecret(token)

	var expires time.Time
	if lg.Expires.Path != "" {
		if v, ok := extract(lg.Expires, resp.Header, decoded); ok {
			if secs, ok := toSeconds(v); ok && secs > 0 {
				expires = p.deps.Now().Add(time.Duration(secs * float64(time.Second)))
			}
		}
	}
	p.token, p.expires = token, expires
	if err := p.writeCache(cacheEntry{Token: token, ExpiresAt: expires, Fingerprint: p.fingerprint()}); err != nil {
		return model.Errorf(model.ExitLocal, "login: write token cache: %s", p.redact(err.Error()))
	}
	return nil
}

func orBody(from string) string {
	if from == "" {
		return "body"
	}
	return from
}

func extract(vf model.ValueFrom, h http.Header, decoded any) (any, bool) {
	if vf.Path == "" {
		return nil, false
	}
	if vf.From == "header" {
		v := h.Get(vf.Path)
		return v, v != ""
	}
	return maputil.GetFromAny(vf.Path, decoded)
}

func toSeconds(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

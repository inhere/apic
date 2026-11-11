package request

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/inhere/apic/internal/model"
)

// Authorizer is implemented by the auth module; declared here so request does not import it.
type Authorizer interface {
	Apply(ctx context.Context, req *http.Request, dry bool) error
	Refresh(ctx context.Context) error
	// Invalidate drops cached credentials after a retried request is still rejected.
	Invalidate() error
}

// Call describes one request execution.
type Call struct {
	Prepared   *Prepared
	Env        model.Env
	Auth       model.AuthSpec
	Authorizer Authorizer // may be nil (no authentication)
	Redactor   *model.Redactor
	Client     *http.Client // nil: built from Env
	DryRun     bool
	Yes        bool
	TTY        bool
	Timeout    time.Duration
	Verbose    io.Writer // request line and redacted headers; nil disables
	Stderr     io.Writer // insecure warning; nil disables
	Prompt     func(msg string) bool
}

// Response is the result of Execute. In dry-run mode Curl is set and Status is 0.
type Response struct {
	Status  int
	Header  http.Header
	Body    []byte
	Request *http.Request // print only after redaction
	Curl    string
}

func (c *Call) newRequest(ctx context.Context) (*http.Request, error) {
	p := c.Prepared
	var body io.Reader
	if len(p.Body) > 0 {
		body = bytes.NewReader(p.Body)
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, body)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "build request: %s", c.Redactor.Redact(err.Error()))
	}
	req.Header = p.Header.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	return req, nil
}

// Execute runs protect check, authentication, sending and the single 401 retry.
func Execute(ctx context.Context, c Call) (*Response, error) {
	if !c.DryRun {
		if err := CheckProtect(c.Prepared.Method, c.Env, c.Auth, c.Yes, c.TTY, c.Prompt); err != nil {
			return nil, err
		}
	}
	if p := c.Prepared; len(p.Parts) > 0 {
		if c.DryRun {
			if err := CheckFiles(p.Parts); err != nil {
				return nil, c.wrapErr(err)
			}
		} else if p.Body == nil {
			body, ctype, err := Assemble(p.Parts)
			if err != nil {
				return nil, c.wrapErr(err)
			}
			p.Body = body
			if p.Header == nil {
				p.Header = http.Header{}
			}
			p.Header.Set("Content-Type", ctype)
		}
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	req, err := c.newRequest(ctx)
	if err != nil {
		return nil, err
	}
	if c.Authorizer != nil {
		if err := c.Authorizer.Apply(ctx, req, c.DryRun); err != nil {
			return nil, c.wrapErr(err)
		}
	}
	if c.DryRun {
		return &Response{Request: req, Curl: Curl(req, c.Prepared.Body, c.Prepared.Parts, c.Env, c.Redactor)}, nil
	}
	client := c.Client
	if client == nil {
		if client, err = NewClient(c.Env); err != nil {
			return nil, err
		}
	}
	resp, err := c.send(client, req)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusUnauthorized || c.Authorizer == nil {
		return resp, nil
	}
	if err := c.Authorizer.Refresh(ctx); err != nil {
		if errors.Is(err, model.ErrNoRefresh) {
			return resp, nil
		}
		return nil, c.wrapErr(err)
	}
	req, err = c.newRequest(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.Authorizer.Apply(ctx, req, false); err != nil {
		return nil, c.wrapErr(err)
	}
	resp, err = c.send(client, req)
	if err != nil {
		return nil, err
	}
	// Still 401 after a fresh credential: do not keep the useless token cached.
	if resp.Status == http.StatusUnauthorized {
		if err := c.Authorizer.Invalidate(); err != nil && c.Stderr != nil {
			fmt.Fprintln(c.Stderr, "warning: "+c.Redactor.Redact("drop cached token: "+err.Error()))
		}
	}
	return resp, nil
}

func (c *Call) send(client *http.Client, req *http.Request) (*Response, error) {
	if c.Env.Insecure && c.Stderr != nil {
		fmt.Fprintln(c.Stderr, "warning: TLS certificate verification is disabled (insecure: true)")
	}
	if c.Verbose != nil {
		fmt.Fprintf(c.Verbose, "> %s %s\n", req.Method, c.Redactor.Redact(req.URL.String()))
		hdr := c.Redactor.RedactHeaders(req.Header)
		names := make([]string, 0, len(hdr))
		for k := range hdr {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			for _, v := range hdr[k] {
				fmt.Fprintf(c.Verbose, "> %s: %s\n", k, v)
			}
		}
	}
	if c.Verbose != nil {
		for _, p := range c.Prepared.Parts {
			if p.File != "" {
				fmt.Fprintf(c.Verbose, "> part %s: file=%s type=%s size=%d\n", c.Redactor.Redact(p.Name), c.Redactor.Redact(p.Filename), p.Type, p.Size)
			} else {
				fmt.Fprintf(c.Verbose, "> part %s: %s\n", c.Redactor.Redact(p.Name), c.Redactor.Redact(p.Value))
			}
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, model.Errorf(model.ExitNetwork, "%s", c.Redactor.Redact(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model.Errorf(model.ExitNetwork, "read response: %s", c.Redactor.Redact(err.Error()))
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: body, Request: req}, nil
}

// wrapErr keeps coded errors and redacts everything else.
func (c *Call) wrapErr(err error) error {
	var ce *model.CodedError
	if errors.As(err, &ce) {
		return model.Errorf(ce.C, "%s", c.Redactor.Redact(ce.Error()))
	}
	return model.Errorf(model.ExitLocal, "%s", c.Redactor.Redact(err.Error()))
}

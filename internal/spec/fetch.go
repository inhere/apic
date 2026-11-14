package spec

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/inhere/apic/internal/model"
)

// FetchOptions provides HTTP transport, conditional request and caller-owned authentication.
type FetchOptions struct {
	Client *http.Client
	ETag   string
	Apply  func(*http.Request) error
}

// Fetch reads a local definition or requests a remote one without leaking URL queries.
func Fetch(ctx context.Context, src model.Source, dir string, opt FetchOptions) (raw []byte, etag string, notModified bool, err error) {
	if src.URL == "" {
		if src.File == "" {
			return nil, "", false, model.Errorf(model.ExitLocal, "definition source has no file or URL")
		}
		file := src.File
		if !filepath.IsAbs(file) {
			file = filepath.Join(dir, file)
		}
		if err := ctx.Err(); err != nil {
			return nil, "", false, model.Errorf(model.ExitLocal, "read definition: %v", err)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, "", false, model.Errorf(model.ExitLocal, "read definition %s: %v", file, err)
		}
		return model.StripBOM(data), "", false, nil
	}
	u, err := url.Parse(src.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, "", false, model.Errorf(model.ExitLocal, "invalid definition URL")
	}
	location := u.Scheme + "://" + u.Host + u.EscapedPath()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, "", false, model.Errorf(model.ExitLocal, "invalid definition URL")
	}
	if opt.ETag != "" {
		req.Header.Set("If-None-Match", opt.ETag)
	}
	if opt.Apply != nil {
		if err := opt.Apply(req); err != nil {
			return nil, "", false, model.Errorf(model.ExitLocal, "apply definition credentials failed")
		}
	}
	client := opt.Client
	if client == nil {
		client = http.DefaultClient
	}
	// Do not forward caller-supplied credentials to redirect targets.
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := clientCopy.Do(req)
	if err != nil {
		return nil, "", false, model.Errorf(model.ExitNetwork, "fetch definition %s: network request failed", location)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, resp.Header.Get("ETag"), true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", false, model.Errorf(model.ExitLocal, "fetch definition %s: HTTP %d", location, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", false, model.Errorf(model.ExitNetwork, "read definition %s: network response failed", location)
	}
	return data, resp.Header.Get("ETag"), false, nil
}

// MaxDownload is the largest body FetchText accepts.
const MaxDownload = 1 << 20

// downloadTimeout bounds one FetchText request.
const downloadTimeout = 30 * time.Second

// FetchText downloads a small text document (a simple definition or a site
// bundle) with a plain GET: no credentials, 30s timeout, at most MaxDownload
// bytes. Failures are coded like a request: network 3, HTTP 4xx 4, 5xx 5, any
// other status 1. Messages carry the URL without its query.
func FetchText(ctx context.Context, rawURL string, client *http.Client) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, model.Errorf(model.ExitLocal, "invalid URL: must be http or https")
	}
	location := u.Scheme + "://" + u.Host + u.EscapedPath()
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "invalid URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, model.Errorf(model.ExitNetwork, "download %s: network request failed", location)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 500:
		return nil, model.Errorf(model.ExitHTTP5xx, "download %s: HTTP %d", location, resp.StatusCode)
	case resp.StatusCode >= 400:
		return nil, model.Errorf(model.ExitHTTP4xx, "download %s: HTTP %d", location, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, model.Errorf(model.ExitLocal, "download %s: HTTP %d", location, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownload+1))
	if err != nil {
		return nil, model.Errorf(model.ExitNetwork, "download %s: network response failed", location)
	}
	if len(data) > MaxDownload {
		return nil, model.Errorf(model.ExitLocal, "download %s: larger than %d bytes", location, MaxDownload)
	}
	return data, nil
}

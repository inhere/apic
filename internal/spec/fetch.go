package spec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/inhere/apix/internal/model"
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
		return data, "", false, nil
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
			return nil, "", false, model.Errorf(model.ExitLocal, "apply definition credentials: %v", err)
		}
	}
	client := opt.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", false, model.Errorf(model.ExitNetwork, "fetch definition %s: %v", location, redactFetchError(err))
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
		return nil, "", false, model.Errorf(model.ExitNetwork, "read definition %s: %v", location, redactFetchError(err))
	}
	return data, resp.Header.Get("ETag"), false, nil
}

func redactFetchError(err error) error {
	// net/http wraps transport failures in url.Error, whose Error() includes the original URL.
	if e, ok := err.(*url.Error); ok {
		return fmt.Errorf("%s: %v", e.Op, e.Err)
	}
	return err
}

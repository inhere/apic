package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/inhere/apic/internal/model"
)

// IndexOptions configures cache refresh and the caller-owned fetch transport.
type IndexOptions struct {
	Refresh   bool
	CacheOnly bool
	Client    *http.Client
	Apply     func(*http.Request) error
}

const indexVersion = 2

type cacheMeta struct {
	ETag         string    `json:"etag"`
	FetchedAt    time.Time `json:"fetched_at"`
	IndexVersion int       `json:"index_version,omitempty"`
}

// Index loads a simple source directly, or maintains an on-disk OpenAPI index.
func Index(ctx context.Context, site string, src model.Source, dir string, opt IndexOptions) (*model.Index, []string, error) {
	if src.Type == "simple" {
		index, err := (SimpleLoader{}).Load(ctx, src, dir)
		return index, nil, err
	}
	if !model.ValidSiteName(site) {
		return nil, nil, model.Errorf(model.ExitLocal, "invalid site name %q", site)
	}
	base := filepath.Join(dir, "cache", "specs", site)
	indexFile, metaFile, rawFile := base+".index.json", base+".meta.json", base+".raw"
	var old model.Index
	oldData, readErr := os.ReadFile(indexFile)
	haveOld := readErr == nil && json.Unmarshal(oldData, &old) == nil
	var meta cacheMeta
	if data, err := os.ReadFile(metaFile); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	var warnings []string
	if haveOld && meta.IndexVersion != indexVersion {
		// Rebuild old indexes locally so a format change does not require a fetch.
		raw, err := os.ReadFile(rawFile)
		if err == nil {
			rebuilt, _, rebuildErr := loadOpenAPI(ctx, src, raw)
			if rebuildErr == nil {
				old = *rebuilt
				meta.ETag = ""
				meta.IndexVersion = indexVersion
				indexData, marshalErr := json.Marshal(&old)
				if marshalErr != nil {
					warnings = append(warnings, fmt.Sprintf("rebuild %s index failed: %v", site, marshalErr))
				} else if writeErr := atomicCacheWrite(indexFile, indexData); writeErr != nil {
					warnings = append(warnings, fmt.Sprintf("rebuild %s index failed: %v", site, writeErr))
				} else if metaData, marshalErr := json.Marshal(meta); marshalErr != nil {
					warnings = append(warnings, fmt.Sprintf("rebuild %s metadata failed: %v", site, marshalErr))
				} else if writeErr := atomicCacheWrite(metaFile, metaData); writeErr != nil {
					warnings = append(warnings, fmt.Sprintf("rebuild %s metadata failed: %v", site, writeErr))
				}
			} else {
				haveOld = false
				meta.ETag = ""
				meta.FetchedAt = time.Time{}
			}
		} else {
			haveOld = false
			meta.ETag = ""
			meta.FetchedAt = time.Time{}
		}
	}
	if opt.CacheOnly {
		if haveOld {
			return &old, warnings, nil
		}
		return nil, warnings, model.Errorf(model.ExitLocal, "no cached index for %s", site)
	}
	ttl := src.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if haveOld && !opt.Refresh && !meta.FetchedAt.IsZero() && time.Since(meta.FetchedAt) < ttl {
		return &old, warnings, nil
	}
	stale := func(err error) (*model.Index, []string, error) {
		if haveOld {
			return &old, append(warnings, fmt.Sprintf("refresh %s failed: %v; using cached index", site, err)), nil
		}
		return nil, warnings, model.Errorf(model.ExitLocal, "no cached index for %s; run apic site refresh %s: %v", site, site, err)
	}
	raw, etag, notModified, err := Fetch(ctx, src, dir, FetchOptions{Client: opt.Client, ETag: meta.ETag, Apply: opt.Apply})
	if err != nil {
		return stale(err)
	}
	if notModified {
		if !haveOld {
			return stale(errors.New("server returned 304 without a cached index"))
		}
		meta.FetchedAt = time.Now()
		meta.IndexVersion = indexVersion
		data, err := json.Marshal(meta)
		if err != nil {
			return stale(err)
		}
		if err := atomicCacheWrite(metaFile, data); err != nil {
			return stale(err)
		}
		return &old, warnings, nil
	}
	index, loadWarnings, err := loadOpenAPI(ctx, src, raw)
	if err != nil {
		return stale(err)
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		return stale(err)
	}
	metaData, err := json.Marshal(cacheMeta{ETag: etag, FetchedAt: time.Now(), IndexVersion: indexVersion})
	if err != nil {
		return stale(err)
	}
	if err := os.MkdirAll(filepath.Dir(base), 0700); err != nil {
		return stale(err)
	}
	if err := atomicCacheWrite(rawFile, raw); err != nil {
		return stale(err)
	}
	if err := atomicCacheWrite(indexFile, indexData); err != nil {
		return stale(err)
	}
	if err := atomicCacheWrite(metaFile, metaData); err != nil {
		return stale(err)
	}
	return index, append(warnings, loadWarnings...), nil
}

func atomicCacheWrite(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".apic-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// Remove deletes the three files belonging to a site's cached definition.
func Remove(dir, site string) error {
	if !model.ValidSiteName(site) {
		return model.Errorf(model.ExitLocal, "invalid site name %q", site)
	}
	base := filepath.Join(dir, "cache", "specs", site)
	for _, suffix := range []string{".raw", ".index.json", ".meta.json"} {
		if err := os.Remove(base + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

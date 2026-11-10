// Package link manages generated shortcut scripts without depending on configuration storage.
package link

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/inhere/apix/internal/model"
)

// Record identifies one generated file, its contents, and its target site.
type Record struct {
	Path string
	Hash string
	Site string
}

// Records maps the generated file name (including .cmd on Windows) to its record.
type Records map[string]Record

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Names validates every value that can be embedded in a shortcut or used as a filename.
func Names(site, name, env, auth string) error {
	for _, item := range []struct{ label, value string }{
		{"site", site}, {"name", name}, {"env", env}, {"auth", auth},
	} {
		if (item.label == "env" || item.label == "auth") && item.value == "" {
			continue
		}
		if !namePattern.MatchString(item.value) {
			return model.Errorf(model.ExitUsage, "invalid %s %q", item.label, item.value)
		}
	}
	return nil
}

func hash(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// Create generates scripts only when the destination is free or still matches
// the recorded path and hash. All destinations are checked before any write.
func Create(dir, name, site, env, auth string, st *Records) ([]string, error) {
	if err := Names(site, name, env, auth); err != nil {
		return nil, err
	}
	if st == nil {
		return nil, model.Errorf(model.ExitLocal, "missing link records")
	}
	paths := make([]string, 0, 2)
	scripts := Scripts(runtime.GOOS, site, env, auth)
	for _, suffix := range []string{"", ".cmd"} {
		_, ok := scripts[suffix]
		if !ok {
			continue
		}
		key := name + suffix
		path, err := filepath.Abs(filepath.Join(dir, key))
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "link path: %v", err)
		}
		if rec, ok := (*st)[key]; ok && (rec.Path != path || rec.Site != site) {
			return nil, model.Errorf(model.ExitLocal, "link %q is already recorded at %s", key, rec.Path)
		}
		data, err := os.ReadFile(path)
		if err == nil {
			rec, ok := (*st)[key]
			if !ok || rec.Path != path || rec.Hash != hash(data) {
				return nil, model.Errorf(model.ExitLocal, "refusing to overwrite unowned or changed file %s", path)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, model.Errorf(model.ExitLocal, "%s: %v", path, err)
		}
		// Do not follow a recorded symlink when replacing its target.
		if fi, err := os.Lstat(path); err == nil && !fi.Mode().IsRegular() {
			return nil, model.Errorf(model.ExitLocal, "refusing to overwrite non-regular file %s", path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, model.Errorf(model.ExitLocal, "%s: %v", path, err)
		}
		paths = append(paths, path)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", dir, err)
	}
	for _, path := range paths {
		suffix := ""
		if filepath.Ext(path) == ".cmd" {
			suffix = ".cmd"
		}
		data := []byte(scripts[suffix])
		mode := os.FileMode(0755)
		if suffix == ".cmd" {
			mode = 0644
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return nil, model.Errorf(model.ExitLocal, "%s: %v", path, err)
		}
		if suffix == "" {
			if err := os.Chmod(path, 0755); err != nil {
				return nil, model.Errorf(model.ExitLocal, "%s: %v", path, err)
			}
		}
		(*st)[name+suffix] = Record{Path: path, Hash: hash(data), Site: site}
	}
	return paths, nil
}

// Remove deletes only unchanged generated files. Missing files lose their records.
// Check every existing file before deleting any, so a changed .cmd protects its sh sibling.
func Remove(name string, st *Records) error {
	if !namePattern.MatchString(name) {
		return model.Errorf(model.ExitUsage, "invalid name %q", name)
	}
	if st == nil {
		return model.Errorf(model.ExitLocal, "missing link records")
	}
	keys := []string{name, name + ".cmd"}
	if _, ok := (*st)[name]; !ok {
		return model.Errorf(model.ExitLocal, "link %q does not exist", name)
	}
	for _, key := range keys {
		rec, ok := (*st)[key]
		if !ok {
			continue
		}
		fi, err := os.Lstat(rec.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return model.Errorf(model.ExitLocal, "%s: %v", rec.Path, err)
		}
		if !fi.Mode().IsRegular() {
			return model.Errorf(model.ExitLocal, "refusing to remove changed file %s", rec.Path)
		}
		data, err := os.ReadFile(rec.Path)
		if err != nil {
			return model.Errorf(model.ExitLocal, "%s: %v", rec.Path, err)
		}
		if hash(data) != rec.Hash {
			return model.Errorf(model.ExitLocal, "refusing to remove changed file %s", rec.Path)
		}
	}
	for _, key := range keys {
		if rec, ok := (*st)[key]; ok {
			if err := os.Remove(rec.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return model.Errorf(model.ExitLocal, "%s: %v", rec.Path, err)
			}
			delete(*st, key)
		}
	}
	return nil
}

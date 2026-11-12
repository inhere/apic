package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
)

// State holds current selections and locally managed resources.
type State struct {
	Selections   map[string]Selection `yaml:"selections,omitempty"`
	Links        map[string]LinkRec   `yaml:"links,omitempty"`
	ManagedSites []string             `yaml:"managed_sites,omitempty"`
}

// Selection records an environment and identity for a site.
type Selection struct {
	Env  string `yaml:"env,omitempty"`
	Auth string `yaml:"auth,omitempty"`
}

// LinkRec records a generated shortcut's path, content hash and site.
type LinkRec struct {
	Path string `yaml:"path"`
	Hash string `yaml:"hash"`
	Site string `yaml:"site"`
}

// LoadState reads state.yaml or returns an initialized empty state.
func LoadState(dir string) (*State, error) {
	st := &State{Selections: make(map[string]Selection), Links: make(map[string]LinkRec)}
	file := filepath.Join(dir, "state.yaml")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if err := yaml.Unmarshal(model.StripBOM(data), st); err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if st.Selections == nil {
		st.Selections = make(map[string]Selection)
	}
	if st.Links == nil {
		st.Links = make(map[string]LinkRec)
	}
	return st, nil
}

// SaveState atomically replaces state.yaml using a mode-0600 temporary file.
func SaveState(dir string, st *State) error {
	file := filepath.Join(dir, "state.yaml")
	data, err := yaml.Marshal(st)
	if err != nil {
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*.yaml")
	if err != nil {
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if err := tmp.Close(); err != nil {
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	return nil
}

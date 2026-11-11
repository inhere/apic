package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
)

var reservedNames = map[string]bool{
	"site": true, "ops": true, "desc": true, "use": true, "auth": true,
	"link": true, "config": true, "help": true, "version": true, "skill": true,
	"overlay": true,
}

// Load merges the optional config.yaml and one-site-per-file sites/*.yaml.
func Load(dir string) (*model.Config, error) {
	cfg := &model.Config{Version: 1, Sites: make(map[string]*model.Site)}
	mainFile := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(mainFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, model.Errorf(model.ExitLocal, "%s site <config>: %v", mainFile, err)
	}
	if err == nil {
		if e := decodeConfig(data, cfg); e != nil {
			return nil, model.Errorf(model.ExitLocal, "%s site <config>: %v", mainFile, e)
		}
	}
	if cfg.Version != 1 {
		return nil, model.Errorf(model.ExitLocal, "%s site <config>: unsupported version %d", mainFile, cfg.Version)
	}
	for name, site := range cfg.Sites {
		if reservedNames[name] {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("site %q uses a reserved name and is skipped; rename it", name))
			delete(cfg.Sites, name)
			continue
		}
		if err := validateSite(dir, mainFile, name, site); err != nil {
			return nil, err
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "sites", "*.yaml"))
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s site <config>: %v", filepath.Join(dir, "sites"), err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		if reservedNames[name] {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("site %q uses a reserved name and is skipped; rename it", name))
			continue
		}
		if err := validateName(file, name); err != nil {
			return nil, err
		}
		if _, exists := cfg.Sites[name]; exists {
			return nil, model.Errorf(model.ExitLocal, "%s site %q: duplicate site (already in %s)", file, name, mainFile)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, model.Errorf(model.ExitLocal, "%s site %q: %v", file, name, err)
		}
		var site model.Site
		if err := decodeSite(data, &site); err != nil {
			return nil, model.Errorf(model.ExitLocal, "%s site %q: %v", file, name, err)
		}
		if err := validateSite(dir, file, name, &site); err != nil {
			return nil, err
		}
		cfg.Sites[name] = &site
	}
	return cfg, nil
}

// ValidateSiteName checks the shared syntax and reserved command names.
func ValidateSiteName(name string) error {
	return validateName("site", name)
}

func validateName(file, name string) error {
	if !model.ValidSiteName(name) || reservedNames[name] {
		return model.Errorf(model.ExitLocal, "%s site %q: invalid or reserved site name", file, name)
	}
	return nil
}

func validateSite(dir, file, name string, site *model.Site) error {
	if err := validateName(file, name); err != nil {
		return err
	}
	if site == nil {
		return model.Errorf(model.ExitLocal, "%s site %q: site is null", file, name)
	}
	if site.Source.File != "" && !filepath.IsAbs(site.Source.File) {
		site.Source.File = filepath.Join(dir, site.Source.File)
	}
	for authName, auth := range site.Auths {
		if auth.Type != "exec" {
			continue
		}
		if len(auth.Exec.Command) == 0 || auth.Exec.Command[0] == "" {
			return model.Errorf(model.ExitLocal, "%s site %q auth %q: exec command must not be empty", file, name, authName)
		}
		if auth.Exec.Format != "" && auth.Exec.Format != "text" && auth.Exec.Format != "json" {
			return model.Errorf(model.ExitLocal, "%s site %q auth %q: exec format must be text or json", file, name, authName)
		}
		if auth.Exec.Format == "json" && auth.Exec.Token.Path == "" {
			return model.Errorf(model.ExitLocal, "%s site %q auth %q: exec token.path is required for json format", file, name, authName)
		}
		for field, value := range map[string]string{
			"timeout": auth.Exec.Timeout,
			"ttl":     auth.Exec.TTL,
		} {
			if value == "" {
				continue
			}
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return model.Errorf(model.ExitLocal, "%s site %q auth %q: exec %s must be a positive duration", file, name, authName, field)
			}
		}
	}
	site.Origin = file
	return nil
}

func decodeConfig(data []byte, cfg *model.Config) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	if entries, exists := raw["sites"]; exists && entries != nil {
		sites, ok := entries.(map[string]any)
		if !ok {
			return fmt.Errorf("sites must be a mapping")
		}
		for name, entry := range sites {
			if entry == nil {
				cfg.Sites[name] = nil
				continue
			}
			siteRaw, ok := entry.(map[string]any)
			if !ok {
				return fmt.Errorf("site %q: must be a mapping", name)
			}
			// The design example puts token/expires/apply beside login, while the
			// typed model stores them inside LoginSpec. Accept both spellings.
			if err := validateExec(siteRaw); err != nil {
				return fmt.Errorf("site %q: %w", name, err)
			}
			normalizeLogin(siteRaw)
			var site model.Site
			if err := decodeRaw(siteRaw, &site); err != nil {
				return fmt.Errorf("site %q: %w", name, err)
			}
			cfg.Sites[name] = &site
		}
	}
	delete(raw, "sites")
	return decodeRaw(raw, cfg)
}

func decodeSite(data []byte, site *model.Site) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := validateExec(raw); err != nil {
		return err
	}
	normalizeLogin(raw)
	return decodeRaw(raw, site)
}

func validateExec(site map[string]any) error {
	auths, ok := site["auths"].(map[string]any)
	if !ok {
		return nil
	}
	allowed := map[string]bool{
		"command": true, "env": true, "timeout": true, "format": true,
		"token": true, "expires": true, "ttl": true,
	}
	for name, value := range auths {
		auth, ok := value.(map[string]any)
		if !ok || auth["type"] != "exec" {
			continue
		}
		if _, exists := auth["login"]; exists {
			return fmt.Errorf("auth %q: exec authentication cannot contain login", name)
		}
		if exec, exists := auth["exec"]; exists && exec != nil {
			execMap, ok := exec.(map[string]any)
			if !ok {
				continue
			}
			for key := range execMap {
				if !allowed[key] {
					return fmt.Errorf("auth %q: unknown exec key %q", name, key)
				}
			}
		}
	}
	return nil
}

func decodeRaw(raw map[string]any, into any) error {
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, into)
}

func normalizeLogin(site map[string]any) {
	auths, ok := site["auths"].(map[string]any)
	if !ok {
		return
	}
	for _, value := range auths {
		auth, ok := value.(map[string]any)
		if !ok || auth["type"] != "login" {
			continue
		}
		login, ok := auth["login"].(map[string]any)
		if !ok {
			login = make(map[string]any)
			auth["login"] = login
		}
		for _, field := range []string{"token", "expires", "apply"} {
			if sibling, exists := auth[field]; exists {
				if _, nested := login[field]; !nested {
					login[field] = sibling
				}
				delete(auth, field)
			}
		}
	}
}

package model

import "time"

// Source describes where a site's API definition comes from.
type Source struct {
	// Type is "openapi" (OpenAPI 3.x / Swagger 2.0) or "simple" (apic YAML definition).
	Type string `yaml:"type"`
	// URL of an OpenAPI document; empty when File is used.
	URL string `yaml:"url,omitempty"`
	// File is a local definition path; relative paths resolve against the config dir.
	File string `yaml:"file,omitempty"`
	// Auth names the site auth used to fetch the document (bearer/basic/apikey only).
	Auth string `yaml:"auth,omitempty"`
	// TTL of the cached operation index; zero means the default.
	TTL time.Duration `yaml:"ttl,omitempty"`
}

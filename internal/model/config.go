package model

// Config is the merged user configuration from config.yaml and sites/*.yaml.
type Config struct {
	Version  int              `yaml:"version,omitempty"`
	Defaults Defaults         `yaml:"defaults,omitempty"`
	Sites    map[string]*Site `yaml:"sites,omitempty"`
}

// Defaults contains settings shared by sites.
type Defaults struct {
	Output  string `yaml:"output,omitempty"`
	Timeout string `yaml:"timeout,omitempty"`
	LinkDir string `yaml:"link_dir,omitempty"`
}

// Site describes a definition source, its environments and identities.
type Site struct {
	Source      Source              `yaml:"source"`
	Envs        map[string]Env      `yaml:"envs,omitempty"`
	DefaultEnv  string              `yaml:"default_env,omitempty"`
	Auths       map[string]AuthSpec `yaml:"auths,omitempty"`
	DefaultAuth string              `yaml:"default_auth,omitempty"`
	Origin      string              `yaml:"-"` // config.yaml or sites/<name>.yaml
}

// Env describes a target API environment.
type Env struct {
	BaseURL  string `yaml:"base_url,omitempty"`
	BasePath string `yaml:"base_path,omitempty"`
	Protect  bool   `yaml:"protect,omitempty"`
	Insecure bool   `yaml:"insecure,omitempty"`
	CAFile   string `yaml:"ca_file,omitempty"`
}

// AuthSpec describes an authentication identity. Login extraction settings
// live in Login; the loader also accepts the design example's sibling form.
type AuthSpec struct {
	Type         string    `yaml:"type"`
	Token        string    `yaml:"token,omitempty"`
	Username     string    `yaml:"username,omitempty"`
	Password     string    `yaml:"password,omitempty"`
	In           string    `yaml:"in,omitempty"`
	Name         string    `yaml:"name,omitempty"`
	Value        string    `yaml:"value,omitempty"`
	AllowMethods []string  `yaml:"allow_methods,omitempty"`
	Login        LoginSpec `yaml:"login,omitempty"`
}

// LoginSpec describes a login request and how its response supplies credentials.
type LoginSpec struct {
	Method  string         `yaml:"method,omitempty"`
	Path    string         `yaml:"path,omitempty"`
	Body    map[string]any `yaml:"body,omitempty"`
	Token   ValueFrom      `yaml:"token,omitempty"`
	Expires ValueFrom      `yaml:"expires,omitempty"`
	Apply   ApplySpec      `yaml:"apply,omitempty"`
}

// ValueFrom selects a value from a login response.
type ValueFrom struct {
	From string `yaml:"from,omitempty"`
	Path string `yaml:"path,omitempty"`
}

// ApplySpec describes where to attach a login token.
type ApplySpec struct {
	Header string `yaml:"header,omitempty"`
	Query  string `yaml:"query,omitempty"`
	Value  string `yaml:"value,omitempty"`
}

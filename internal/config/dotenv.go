package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/inhere/apic/internal/model"
)

// ParseDotenv reads simple KEY=VALUE lines without modifying the OS environment.
func ParseDotenv(text string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvKey(key) {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			quote := value[0]
			var b strings.Builder
			for i := 1; i < len(value); i++ {
				if value[i] == quote {
					break
				}
				if quote == '"' && value[i] == '\\' && i+1 < len(value) {
					i++
					switch value[i] {
					case 'n':
						b.WriteByte('\n')
					default:
						b.WriteByte(value[i])
					}
				} else {
					b.WriteByte(value[i])
				}
			}
			value = b.String()
		} else {
			// A whitespace-delimited # starts a comment, not part of a value.
			if strings.HasPrefix(value, "#") {
				value = ""
			}
			for i := 1; i < len(value); i++ {
				if value[i] == '#' && (value[i-1] == ' ' || value[i-1] == '\t') {
					value = strings.TrimSpace(value[:i])
					break
				}
			}
		}
		values[key] = value
	}
	return values
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// Env holds variables read from the config directory's .env only.
type Env struct {
	Values map[string]string
	// Exec runs one ${exec:...} reference; nil reports it as not available.
	Exec func(argv []string) (string, error)

	// execMu guards execCache, which deduplicates ${exec:} references within one
	// process. Both are filled on first use so a zero Env works too.
	execMu    sync.Mutex
	execCache map[string]string
}

// cachedExec returns the output already produced for the reference text ref.
func (e *Env) cachedExec(ref string) (string, bool) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	value, ok := e.execCache[ref]
	return value, ok
}

// rememberExec stores the output of ref. A failure is never stored.
func (e *Env) rememberExec(ref, value string) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	if e.execCache == nil {
		e.execCache = make(map[string]string)
	}
	e.execCache[ref] = value
}

// LoadEnv never consults the working directory's .env file.
func LoadEnv(dir string) (*Env, error) {
	if dir == "" {
		return nil, model.Errorf(model.ExitLocal, "empty config directory: refusing to read working-directory .env")
	}
	file := filepath.Join(dir, ".env")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return &Env{Values: make(map[string]string)}, nil
	}
	if err != nil {
		return nil, model.Errorf(model.ExitLocal, "%s: %v", file, err)
	}
	return &Env{Values: ParseDotenv(string(model.StripBOM(data)))}, nil
}

// Lookup prefers even an empty value explicitly present in the OS environment.
func (e *Env) Lookup(name string) (string, bool) {
	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}
	if e == nil {
		return "", false
	}
	v, ok := e.Values[name]
	return v, ok
}

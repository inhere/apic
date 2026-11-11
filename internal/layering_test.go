// Package internal_test guards the module's production dependency layering.
package internal_test

import (
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const apicModule = "github.com/inhere/apic/"

// layeringTable follows the approved phase-one plan's layering table. Entries
// for future packages are intentional; only packages that exist are checked.
var layeringTable = map[string][]string{
	"cmd/apic":          {"internal/commands"},
	"internal/model":    {},
	"internal/config":   {"internal/model"},
	"internal/spec":     {"internal/model"},
	"internal/auth":     {"internal/model"},
	"internal/request":  {"internal/model"},
	"internal/output":   {"internal/model"},
	"internal/link":     {"internal/model"},
	"internal/skillgen": {"internal/model"},
	"internal/wiring":   {"internal/model", "internal/config", "internal/spec", "internal/auth", "internal/request", "internal/output", "internal/link", "internal/skillgen"},
	"internal/commands": {"internal/model", "internal/config", "internal/spec", "internal/auth", "internal/request", "internal/output", "internal/link", "internal/skillgen", "internal/wiring"},
}

type pkgEdges struct {
	name string
	deps []string
}

// TestLayering rejects unregistered production packages and dependency edges.
func TestLayering(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	pkgs := listInternalEdges(t)
	var bad []string
	for _, p := range pkgs {
		allowed, ok := layeringTable[p.name]
		if !ok {
			bad = append(bad, p.name+": package not in layering table")
			continue
		}
		for _, dep := range p.deps {
			if !slices.Contains(allowed, dep) {
				bad = append(bad, p.name+" -> "+dep+": edge not in layering table")
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("layering violations:\n  %s", strings.Join(bad, "\n  "))
	}
}

// listInternalEdges uses production imports only, as in xhen's layering guard.
func listInternalEdges(t *testing.T) []pkgEdges {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./cmd/...", "./internal/...")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("go list failed: %v\n%s", err, stderr)
	}

	var pkgs []pkgEdges
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		p := pkgEdges{name: strings.TrimPrefix(fields[0], apicModule)}
		if p.name == "internal" {
			continue // this test-only package
		}
		for _, imp := range fields[1:] {
			if strings.HasPrefix(imp, apicModule) {
				p.deps = append(p.deps, strings.TrimPrefix(imp, apicModule))
			} else if p.name == "internal/model" && strings.Contains(strings.SplitN(imp, "/", 2)[0], ".") && imp != "github.com/gookit/goutil/errorx" {
				p.deps = append(p.deps, imp)
			}
		}
		pkgs = append(pkgs, p)
	}
	assert.Require(t, assert.NotEmpty(t, pkgs), "go list returned no packages")
	return pkgs
}

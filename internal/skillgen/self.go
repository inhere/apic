package skillgen

import (
	_ "embed"
	"strings"
)

// selfTemplate is the static body of apic's own skill. It is shipped in the
// binary and only the version placeholder is filled in at render time.
//
//go:embed self.md
var selfTemplate string

// selfVersion is the placeholder replaced by the apic version in selfTemplate.
const selfVersion = "{{version}}"

// selfDescription is the frontmatter description of apic's own skill.
const selfDescription = "Use the apic CLI to call HTTP APIs: configure sites, write simple definitions, list operations, pick response values and generate per-site skills."

// RenderSelf returns the complete SKILL.md that teaches an agent to use apic
// itself. It reads no configuration, so it works with a missing or broken
// config directory; version empty becomes "dev".
func RenderSelf(name, version string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if version == "" {
		version = "dev"
	}
	var b strings.Builder
	if err := writeHeader(&b, name, selfDescription); err != nil {
		return nil, err
	}
	b.WriteString(strings.ReplaceAll(selfTemplate, selfVersion, version))
	return []byte(b.String()), nil
}

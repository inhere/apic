package model

// Overlay describes per-site operation overrides loaded from an overlay file.
type Overlay struct {
	File string                `yaml:"-"`
	Ops  map[string]*OpOverlay `yaml:"operations"`
}

// OpOverlay describes changes to one indexed operation.
type OpOverlay struct {
	Name    *string                  `yaml:"name"`
	Summary *string                  `yaml:"summary"`
	Body    *string                  `yaml:"body"`
	Output  *string                  `yaml:"output"`
	Hide    bool                     `yaml:"hide"`
	Params  map[string]*ParamOverlay `yaml:"params"`
}

// ParamOverlay describes changes to an existing or newly added parameter.
type ParamOverlay struct {
	In       string   `yaml:"in"`
	Required *bool    `yaml:"required"`
	Type     *string  `yaml:"type"`
	Desc     *string  `yaml:"desc"`
	Enum     []string `yaml:"enum"`
	Hide     bool     `yaml:"hide"`
}

// OverlayChange records a field changed while applying an overlay.
type OverlayChange struct{ Op, Field string }

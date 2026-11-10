package model

// Operation describes one callable API operation.
type Operation struct {
	Name      string
	Method    string
	Path      string
	Summary   string
	Tags      []string
	Params    []Param
	Body      *BodySpec
	Output    string
	Responses map[string]SchemaSummary
}

// Param describes a path, query or header parameter.
type Param struct {
	Name     string
	In       string
	Required bool
	Type     string
	Enum     []string
	Desc     string
}

// BodySpec describes an operation's request body.
type BodySpec struct {
	Type     string
	Required bool
	Schema   SchemaSummary
}

// SchemaSummary describes the top-level fields of a schema.
type SchemaSummary struct {
	Type   string
	Fields []Field
}

// Field describes a top-level schema field.
type Field struct {
	Name     string
	Type     string
	Required bool
}

// Index contains operations and defaults from an API definition.
type Index struct {
	Ops             []Operation
	DefaultBasePath string
	DefaultBaseURL  string
}

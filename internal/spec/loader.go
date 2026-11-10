// Package spec loads API definitions into the operation model.
package spec

import (
	"context"

	"github.com/inhere/apic/internal/model"
)

// Loader loads a definition; dir resolves relative source file paths.
type Loader interface {
	Load(ctx context.Context, src model.Source, dir string) (*model.Index, error)
}

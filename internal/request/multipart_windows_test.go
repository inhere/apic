//go:build windows

package request

import (
	"context"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func TestMultipartNULWindows(t *testing.T) {
	srv, got := mpServer(t, func(int) int { return 200 })
	pr, err := mpBuild(t, srv.URL, Input{Forms: []string{"f=@NUL;filename=x"}})
	assert.NoErr(t, err)
	_, err = Execute(context.Background(), call(t, srv, pr, nil, nil))
	assert.Eq(t, model.ExitLocal, code(err))
	c := call(t, srv, pr, nil, nil)
	c.DryRun = true
	_, err = Execute(context.Background(), c)
	assert.Eq(t, model.ExitLocal, code(err))
	assert.Eq(t, 0, len(got()))
}

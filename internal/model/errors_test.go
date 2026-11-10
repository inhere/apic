package model_test

import (
	"errors"
	"testing"

	"github.com/gookit/goutil/errorx"
	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func TestErrorf(t *testing.T) {
	t.Setenv("APIC_CONFIG_DIR", t.TempDir())
	err := model.Errorf(model.ExitUsage, "invalid %s", "demo")
	var coded errorx.ErrorCoder
	assert.Require(t, assert.True(t, errors.As(err, &coded)))
	assert.Eq(t, model.ExitUsage, coded.Code())
	assert.Eq(t, "invalid demo", err.Error())
	assert.Eq(t, "invalid demo", errors.Unwrap(err).Error())
}

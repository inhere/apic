package commands

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func TestParamOptionsHidden(t *testing.T) {
	op := &model.Operation{Params: []model.Param{
		{Name: "debug", In: "query", Hidden: true},
		{Name: "page", In: "query"},
	}}
	var names []string
	for _, p := range paramOptions(op) {
		names = append(names, p.Name)
	}
	assert.Eq(t, []string{"page"}, names)
	assert.True(t, reservedOptNames["F"])
}

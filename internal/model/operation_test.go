package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
)

func TestParamJSONHiddenOmitempty(t *testing.T) {
	for _, tc := range []struct {
		name    string
		param   model.Param
		absent  string
		present string
	}{
		{name: "visible", param: model.Param{Name: "debug"}, absent: "Hidden", present: ""},
		{name: "hidden", param: model.Param{Name: "debug", Hidden: true}, absent: "", present: "\"Hidden\":true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.param)
			assert.NoErr(t, err)
			got := string(data)
			if tc.absent != "" {
				assert.False(t, strings.Contains(got, tc.absent))
			}
			if tc.present != "" {
				assert.True(t, strings.Contains(got, tc.present))
			}
		})
	}
}

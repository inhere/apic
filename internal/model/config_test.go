package model

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestEffectiveAuth(t *testing.T) {
	site := &Site{
		Envs: map[string]Env{
			"factory": {Auth: "fa"},
			"build":   {},
		},
		DefaultAuth: "def",
	}

	tests := []struct {
		name              string
		env, saved, flag  string
		wantName, wantSrc string
	}{
		{"flag wins", "factory", "sv", "fl", "fl", AuthFromFlag},
		{"env beats saved", "factory", "sv", "", "fa", AuthFromEnv},
		{"saved beats default", "build", "sv", "", "sv", AuthFromSaved},
		{"default", "build", "", "", "def", AuthFromDefault},
		{"empty env decl falls to saved", "build", "sv", "", "sv", AuthFromSaved},
		{"unknown env skips env level", "nope", "sv", "", "sv", AuthFromSaved},
		{"unknown env, default", "", "", "", "def", AuthFromDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, src := site.EffectiveAuth(tt.env, tt.saved, tt.flag)
			assert.Eq(t, tt.wantName, n)
			assert.Eq(t, tt.wantSrc, src)
		})
	}

	t.Run("all empty", func(t *testing.T) {
		n, src := (&Site{}).EffectiveAuth("x", "", "")
		assert.Eq(t, "", n)
		assert.Eq(t, "", src)
	})
}

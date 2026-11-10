package model

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestValidSiteName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{{"a-", false}, {"a--b", false}, {"a-b", true}, {"1a", false}, {"a", true}, {"", false}} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Eq(t, tc.want, ValidSiteName(tc.name))
		})
	}
}

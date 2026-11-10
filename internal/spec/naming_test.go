package spec_test

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
)

func TestNaming(t *testing.T) {
	cases := []struct {
		name     string
		ops      []model.Operation
		ids      []string
		want     []string
		warnings int
	}{
		{"operation-id", []model.Operation{{}}, []string{"listUsers"}, []string{"list-users"}, 0},
		{"acronym", []model.Operation{{}}, []string{"getHTTPInfo"}, []string{"get-http-info"}, 0},
		{"underscore", []model.Operation{{}}, []string{"user_list"}, []string{"user-list"}, 0},
		{"post-path", []model.Operation{{Method: "POST", Path: "/back/cfg/get"}}, []string{""}, []string{"back-cfg-get"}, 0},
		{"common-prefix", []model.Operation{{Path: "/api/v1/a/x"}, {Path: "/api/v1/b/y"}}, []string{"", ""}, []string{"a-x", "b-y"}, 0},
		{"single", []model.Operation{{Path: "/api/v1/x"}}, []string{""}, []string{"api-v1-x"}, 0},
		{"root", []model.Operation{{Path: "/"}}, []string{""}, []string{"root"}, 0},
		{"methods", []model.Operation{{Method: "GET", Path: "/users"}, {Method: "DELETE", Path: "/users"}}, []string{"", ""}, []string{"get-users", "delete-users"}, 2},
		{"parameter", []model.Operation{{Method: "GET", Path: "/users"}, {Method: "GET", Path: "/users/{id}"}}, []string{"", ""}, []string{"users", "users-by-id"}, 1},
		{"multiple-parameters", []model.Operation{{Method: "GET", Path: "/users/{id}/{zone}"}, {Method: "GET", Path: "/users/{name}"}}, []string{"", ""}, []string{"users-by-id-and-zone", "users-by-name"}, 2},
		{"numbers", []model.Operation{{Method: "GET", Path: "/users"}, {Method: "GET", Path: "/users"}, {Method: "GET", Path: "/users"}}, []string{"", "", ""}, []string{"users", "users-2", "users-3"}, 2},
		{"reserved-number", []model.Operation{{Method: "GET", Path: "/users"}, {Method: "GET", Path: "/users"}, {Method: "GET"}}, []string{"", "", "users-2"}, []string{"users", "users-3", "users-2"}, 1},
		{"mixed-ids", []model.Operation{{Method: "GET", Path: "/other"}, {Method: "DELETE", Path: "/users"}}, []string{"users", ""}, []string{"get-users", "delete-users"}, 2},
		{"prefix-preserves-last", []model.Operation{{Path: "/api/v1"}, {Path: "/api/v1/x"}}, []string{"", ""}, []string{"v1", "v1-x"}, 0},
		{"root-and-path", []model.Operation{{Path: "/"}, {Path: "/api/v1"}}, []string{"", ""}, []string{"root", "api-v1"}, 0},
		{"punctuation", []model.Operation{{Path: "/a.b/c_d"}}, []string{""}, []string{"a-b-c-d"}, 0},
		{"path-camel", []model.Operation{{Path: "/userProfile"}}, []string{""}, []string{"userprofile"}, 0},
		{"parameter-camel", []model.Operation{{Method: "GET", Path: "/users"}, {Method: "GET", Path: "/users/{userId}"}}, []string{"", ""}, []string{"users", "users-by-userid"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APIC_CONFIG_DIR", t.TempDir())
			warnings := spec.AssignNames(tc.ops, tc.ids)
			names := make([]string, len(tc.ops))
			for i := range tc.ops {
				names[i] = tc.ops[i].Name
			}
			assert.Eq(t, tc.want, names)
			assert.Eq(t, tc.warnings, len(warnings))
		})
	}
}

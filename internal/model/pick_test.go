package model

import (
	"bytes"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func pickJSON(t *testing.T, body, path string) (string, error) {
	t.Helper()
	v, err := Decode([]byte(body))
	assert.NoError(t, err)
	p, err := ParsePick(path)
	assert.NoError(t, err)
	got, err := p.Apply(v)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	assert.NoError(t, EncodeJSON(&out, got, false))
	return out.String(), nil
}

func TestPick(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		for _, p := range []string{"", ".", "a..b", ".a", "a."} {
			_, err := ParsePick(p)
			assert.Err(t, err, p)
			assert.Eq(t, ExitUsage, err.(*CodedError).Code())
		}
		p, err := ParsePick("a.0.*")
		assert.NoError(t, err)
		assert.Eq(t, "a.0.*", p.String())
	})
	t.Run("plain path and index", func(t *testing.T) {
		got, err := pickJSON(t, `{"a":{"b":[10,{"c":"x"}]}}`, "a.b.1.c")
		assert.NoError(t, err)
		assert.Eq(t, `"x"`, got)
		got, err = pickJSON(t, `[{"n":1},{"n":2}]`, "1.n")
		assert.NoError(t, err)
		assert.Eq(t, `2`, got)
	})
	t.Run("star over array", func(t *testing.T) {
		got, err := pickJSON(t, `{"d":[{"id":1},{"id":2}]}`, "d.*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2]`, got)
	})
	t.Run("star over empty array", func(t *testing.T) {
		got, err := pickJSON(t, `{"d":[]}`, "d.*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[]`, got)
	})
	t.Run("star missing field fills null", func(t *testing.T) {
		got, err := pickJSON(t, `[{"id":1},{"x":2},{"id":3}]`, "*.id")
		assert.NoError(t, err)
		assert.Eq(t, `[1,null,3]`, got)
		got, err = pickJSON(t, `[{"a":{"b":1}},{"a":5},{"a":{"c":1}}]`, "*.a.b")
		assert.NoError(t, err)
		assert.Eq(t, `[1,null,null]`, got)
	})
	t.Run("star over object follows key order", func(t *testing.T) {
		got, err := pickJSON(t, `{"z":1,"a":2,"m":3}`, "*")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2,3]`, got)
		got, err = pickJSON(t, `{"z":{"k":1},"a":{"k":2}}`, "*.k")
		assert.NoError(t, err)
		assert.Eq(t, `[1,2]`, got)
	})
	t.Run("errors carry exit 7 and segment", func(t *testing.T) {
		for _, tc := range []struct{ body, path, seg string }{
			{`{"a":1}`, "b", `"b"`},
			{`{"a":[1,2]}`, "a.2", `"2"`},
			{`{"a":[1,2]}`, "a.x", `"x"`},
			{`{"a":1}`, "a.b", `"b"`},
			{`{"a":{"b":1}}`, "a.c", `"c"`},
		} {
			_, err := pickJSON(t, tc.body, tc.path)
			assert.Err(t, err, tc.path)
			assert.Eq(t, ExitPostProcess, err.(*CodedError).Code())
			assert.Contains(t, err.Error(), tc.seg)
		}
		_, err := pickJSON(t, `{"a":{"b":1}}`, "a.c")
		assert.Contains(t, err.Error(), "a")
	})
	t.Run("exit code value", func(t *testing.T) {
		assert.Eq(t, 7, ExitPostProcess)
	})
}

func TestPickItems(t *testing.T) {
	body := `{"code":0,"data":{"username":"u","id":7}}`
	for _, tc := range []struct{ name, path, want string }{
		{"one path keeps the plain value", "data.username", `"u"`},
		{"two items from the root", "code,data.username", `{"code":0,"username":"u"}`},
		{"expandable group is flat", "data.{username,id},code", `{"username":"u","id":7,"code":0}`},
		{"alias and path", "uid=data.id,code", `{"uid":7,"code":0}`},
		{"single aliased item is an object", "uid=data.id", `{"uid":7}`}, // R1
		{"missing item is null", "code,data.missing", `{"code":0,"missing":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickJSON(t, body, tc.path)
			assert.NoError(t, err)
			assert.Eq(t, tc.want, got)
		})
	}
	t.Run("index path takes the key name", func(t *testing.T) {
		got, err := pickJSON(t, `[{"name":"x","c":1}]`, "0.name,0.c")
		assert.NoError(t, err)
		assert.Eq(t, `{"name":"x","c":1}`, got)
	})
	t.Run("key order follows the written order", func(t *testing.T) {
		got, err := pickJSON(t, `{"a":1,"b":2,"c":3}`, "c,a,b")
		assert.NoError(t, err)
		assert.Eq(t, `{"c":3,"a":1,"b":2}`, got)
	})
	t.Run("index or star alone needs an alias", func(t *testing.T) {
		body := `{"*":1,"0":2,"a":3}`
		for _, path := range []string{"*,a", "0,a"} {
			_, err := ParsePick(path)
			assert.Err(t, err, path)
			assert.Eq(t, ExitUsage, err.(*CodedError).Code(), path)
		}
		got, err := pickJSON(t, body, "star=*,zero=0,a")
		assert.NoError(t, err)
		assert.Eq(t, `{"star":[1,2,3],"zero":2,"a":3}`, got)
	})
	t.Run("single aliased star item", func(t *testing.T) {
		got, err := pickJSON(t, `{"d":[{"id":1},{"id":2}]}`, "ids=d.*.id")
		assert.NoError(t, err)
		assert.Eq(t, `{"ids":[1,2]}`, got)
	})
}

func TestPickGroups(t *testing.T) {
	list := `{"data":{"list":[{"id":1,"name":"a","z":9},{"id":2,"name":"b"}],"total":42}}`

	t.Run("group below star gives object array", func(t *testing.T) {
		got, err := pickJSON(t, list, "data.list.*.{id,name}")
		assert.NoError(t, err)
		assert.Eq(t, `[{"id":1,"name":"a"},{"id":2,"name":"b"}]`, got)
	})
	t.Run("list and total", func(t *testing.T) {
		got, err := pickJSON(t, list, "data.list.*.{id,name},data.total")
		assert.NoError(t, err)
		assert.Eq(t, `{"list":[{"id":1,"name":"a"},{"id":2,"name":"b"}],"total":42}`, got)
	})
	t.Run("nested group is flat", func(t *testing.T) {
		got, err := pickJSON(t, `{"data":{"a":1,"sub":{"x":2,"y":3}}}`, "data.{a,sub.{x,y}}")
		assert.NoError(t, err)
		assert.Eq(t, `{"a":1,"x":2,"y":3}`, got)
	})
	t.Run("aliased group is nested", func(t *testing.T) { // R2
		got, err := pickJSON(t, `{"data":{"a":1,"b":2}}`, "u=data.{a,b}")
		assert.NoError(t, err)
		assert.Eq(t, `{"u":{"a":1,"b":2}}`, got)
	})
	t.Run("aliased group inside a group", func(t *testing.T) {
		got, err := pickJSON(t, `{"data":{"a":1,"sub":{"x":2,"y":3}}}`, "data.{a,u=sub.{x,y}}")
		assert.NoError(t, err)
		assert.Eq(t, `{"a":1,"u":{"x":2,"y":3}}`, got)
	})
	t.Run("missing group prefix of a single item exits 7", func(t *testing.T) { // R3
		_, err := pickJSON(t, `{"x":1}`, "data.{a,b}")
		assert.Err(t, err)
		assert.Eq(t, ExitPostProcess, err.(*CodedError).Code())
		assert.Contains(t, err.Error(), `"data"`)
	})
	t.Run("missing group prefix among items is null", func(t *testing.T) { // R3
		got, err := pickJSON(t, `{"x":1}`, "data.{a,b},x")
		assert.NoError(t, err)
		assert.Eq(t, `{"a":null,"b":null,"x":1}`, got)
	})
	t.Run("group prefix that is not an object", func(t *testing.T) { // R3
		_, err := pickJSON(t, `{"data":5}`, "data.{a,b}")
		assert.Err(t, err)
		assert.Eq(t, ExitPostProcess, err.(*CodedError).Code())
		assert.Contains(t, err.Error(), "needs an object")
		got, err := pickJSON(t, `{"data":5,"x":1}`, "data.{a,b},x")
		assert.NoError(t, err)
		assert.Eq(t, `{"a":null,"b":null,"x":1}`, got)
	})
	t.Run("non-object element below star is null", func(t *testing.T) { // R4
		got, err := pickJSON(t, `[{"id":1,"name":"a"},5,"s"]`, "*.{id,name}")
		assert.NoError(t, err)
		assert.Eq(t, `[{"id":1,"name":"a"},null,null]`, got)
	})
	t.Run("missing key of an element below star is null", func(t *testing.T) { // R4
		got, err := pickJSON(t, `[{"id":1},{"name":"b"}]`, "*.{id,name}")
		assert.NoError(t, err)
		assert.Eq(t, `[{"id":1,"name":null},{"id":null,"name":"b"}]`, got)
	})
	t.Run("group below star inside an item", func(t *testing.T) {
		got, err := pickJSON(t, list, "data.list.*.{id},data.total")
		assert.NoError(t, err)
		assert.Eq(t, `{"list":[{"id":1},{"id":2}],"total":42}`, got)
	})
	t.Run("missing plain item among items is null", func(t *testing.T) {
		got, err := pickJSON(t, `{"a":1}`, "a,missing.deeper")
		assert.NoError(t, err)
		assert.Eq(t, `{"a":1,"deeper":null}`, got)
	})
}

func TestPickSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{".", "empty segment"},
		{"a..b", "empty segment"},
		{".a", "empty segment"},
		{"a.", "empty segment"},
		{"a,,b", "empty item"},
		{"a,", "empty item"},
		{",a", "empty item"},
		{"{}", "empty item"},
		{"a.{}", "empty item"},
		{"=a", "empty alias"},
		{"a b=id", "invalid alias"},
		{"a.{b", "unbalanced braces"},
		{"a.b}", "unbalanced braces"},
		{"a{b}", "malformed group"},
		{"a.{b}{c}", "malformed group"},
		{"data.{a,b}.x", "a group must be the last segment"},
		{"a,a", `duplicate key "a"`},
		{"uid=data.id,uid=code", `duplicate key "uid"`},
		{"data.{a,a}", `duplicate key "a"`},
		{"data.list.*.{id,id}", `duplicate key "id"`},
		{"a.{b,c.{b}}", `duplicate key "b"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			_, err := ParsePick(tc.path)
			assert.Require(t, assert.Error(t, err))
			assert.Eq(t, ExitUsage, err.(*CodedError).Code())
			assert.Contains(t, err.Error(), `--pick "`+tc.path+`": `)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestPickLiteralKeys(t *testing.T) {
	// A comma always splits items: no literal key fallback anymore.
	got, err := pickJSON(t, `{"a,b":"lit","a":1,"b":2}`, "a,b")
	assert.NoError(t, err)
	assert.Eq(t, `{"a":1,"b":2}`, got)
	got, err = pickJSON(t, `{"a,b":{"c":1},"a":1}`, "a,b.c")
	assert.NoError(t, err)
	assert.Eq(t, `{"a":1,"c":null}`, got)
	// A dot inside a single segment is unreachable, so "a.b" is two levels.
	_, err = pickJSON(t, `{"a.b":1}`, "a.b")
	assert.Err(t, err)
	assert.Eq(t, ExitPostProcess, err.(*CodedError).Code())
}

func TestPickIsPlainPath(t *testing.T) {
	for _, tc := range []struct {
		path  string
		plain bool
	}{
		{"a.b", true},
		{"a.0.b", true},
		{"a", true},
		{"a.*", false},
		{"a,b", false},
		{"uid=a", false},
		{"a.{b}", false},
		{"*.{a}", false},
	} {
		p, err := ParsePick(tc.path)
		assert.Require(t, assert.NoError(t, err))
		assert.Eq(t, tc.plain, p.IsPlainPath(), tc.path)
	}
}

func TestPickStringKeepsInput(t *testing.T) {
	for _, path := range []string{"a.b,c", "data.{a,b},code", "uid=data.id", "a.0.*"} {
		p, err := ParsePick(path)
		assert.Require(t, assert.NoError(t, err))
		assert.Eq(t, path, p.String())
	}
}

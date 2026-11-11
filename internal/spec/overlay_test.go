package spec_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/apic/internal/model"
	"github.com/inhere/apic/internal/spec"
)

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

func oc(op, field string) model.OverlayChange { return model.OverlayChange{Op: op, Field: field} }

func ovIndex() *model.Index {
	return &model.Index{
		DefaultBasePath: "/v1",
		Ops: []model.Operation{
			{Name: "list-users", Method: "GET", Path: "/users", Summary: "List", Tags: []string{"u"},
				Params: []model.Param{
					{Name: "limit", In: "query", Type: "integer", Enum: []string{"1", "2"}},
					{Name: "mode", In: "query", Required: true, Type: "string"},
					{Name: "Authorization", In: "header", Type: "string"},
				},
				Output: "json", Responses: map[string]model.SchemaSummary{"200": {Fields: []model.Field{{Name: "a"}}}}},
			{Name: "get-user", Method: "GET", Path: "/users/{id}",
				Params: []model.Param{{Name: "id", In: "path", Required: true, Type: "string"}}},
			{Name: "create-user", Method: "POST", Path: "/users", Body: &model.BodySpec{Type: "json", Schema: model.SchemaSummary{Fields: []model.Field{{Name: "n"}}}}},
			{Name: "ping", Method: "GET", Path: "/ping"},
		},
	}
}

func opNames(idx *model.Index) []string {
	var names []string
	for _, op := range idx.Ops {
		names = append(names, op.Name)
	}
	return names
}

func findOp(idx *model.Index, name string) *model.Operation {
	for i := range idx.Ops {
		if idx.Ops[i].Name == name {
			return &idx.Ops[i]
		}
	}
	return nil
}

func ovOf(ops map[string]*model.OpOverlay) *model.Overlay {
	return &model.Overlay{File: "o.yaml", Ops: ops}
}

func TestApplyOverlay(t *testing.T) {
	t.Run("nil overlay returns equal copy", func(t *testing.T) {
		idx := ovIndex()
		got, ch, w, err := spec.ApplyOverlay(idx, nil)
		assert.NoErr(t, err)
		assert.Empty(t, ch)
		assert.Empty(t, w)
		assert.True(t, reflect.DeepEqual(idx, got))
		got.Ops[0].Params[0].Enum[0] = "x"
		got.Ops[0].Tags[0] = "x"
		assert.Eq(t, "1", idx.Ops[0].Params[0].Enum[0])
	})

	t.Run("original index unchanged", func(t *testing.T) {
		idx := ovIndex()
		_, _, _, err := spec.ApplyOverlay(idx, ovOf(map[string]*model.OpOverlay{
			"list-users": {Summary: sp("S"), Output: sp("raw"), Params: map[string]*model.ParamOverlay{
				"limit": {Enum: []string{"9"}, Type: sp("string")}, "mode": {Required: bp(false), Hide: true}, "extra": {In: "query"}}},
			"create-user": {Body: sp("form"), Name: sp("mk")},
			"ping":        {Hide: true},
		}))
		assert.NoErr(t, err)
		assert.True(t, reflect.DeepEqual(ovIndex(), idx))
	})

	t.Run("key resolution", func(t *testing.T) {
		cases := []struct {
			name, key, wantErr, wantWarn string
			want                         string
		}{
			{name: "by name", key: "ping", want: "p"},
			{name: "by route", key: "GET /ping", want: "p"},
			{name: "method case-insensitive", key: "get /ping", want: "p"},
			{name: "path exact", key: "GET /ping/", wantWarn: "overlay o.yaml: GET /ping/ matches no operation"},
			{name: "unknown name", key: "nope", wantWarn: "overlay o.yaml: nope matches no operation"},
			{name: "route with other method", key: "POST /users", want: "p"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				_, ch, w, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{c.key: {Name: sp("p")}}))
				assert.NoErr(t, err)
				if c.wantWarn != "" {
					assert.Eq(t, []string{c.wantWarn}, w)
					assert.Empty(t, ch)
				} else {
					assert.Eq(t, []model.OverlayChange{{Op: "p", Field: "name"}}, ch)
				}
			})
		}
		t.Run("two keys same op", func(t *testing.T) {
			_, _, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"ping": {}, "GET /ping": {}}))
			assert.Err(t, err)
			assert.StrContains(t, err.Error(), "overlay o.yaml: ping: targets the same operation")
		})
		t.Run("key matches several ops", func(t *testing.T) {
			idx := ovIndex()
			idx.Ops = append(idx.Ops, idx.Ops[3])
			_, _, _, err := spec.ApplyOverlay(idx, ovOf(map[string]*model.OpOverlay{"GET /ping": {}}))
			assert.Err(t, err)
			assert.StrContains(t, err.Error(), "GET /ping: matches 2 operations")
		})
		t.Run("nil entry is empty", func(t *testing.T) {
			got, ch, w, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"ping": nil}))
			assert.NoErr(t, err)
			assert.Empty(t, ch)
			assert.Empty(t, w)
			assert.Eq(t, 4, len(got.Ops))
		})
	})

	t.Run("hide operation", func(t *testing.T) {
		got, ch, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"ping": {Hide: true}}))
		assert.NoErr(t, err)
		assert.Eq(t, []string{"list-users", "get-user", "create-user"}, opNames(got))
		assert.Empty(t, ch)
	})

	t.Run("simple fields and records", func(t *testing.T) {
		got, ch, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{
			"list-users":  {Summary: sp("New"), Output: sp("raw"), Name: sp("users")},
			"create-user": {Body: sp("form")},
			"ping":        {Body: sp("xml")},
		}))
		assert.NoErr(t, err)
		u := findOp(got, "users")
		assert.Eq(t, "New", u.Summary)
		assert.Eq(t, "raw", u.Output)
		assert.Eq(t, "form", findOp(got, "create-user").Body.Type)
		assert.Eq(t, "n", findOp(got, "create-user").Body.Schema.Fields[0].Name)
		assert.Eq(t, &model.BodySpec{Type: "xml"}, findOp(got, "ping").Body)
		assert.Eq(t, []model.OverlayChange{
			oc("create-user", "body"), oc("users", "name"), oc("users", "summary"), oc("users", "output"), oc("ping", "body"),
		}, ch)
	})

	t.Run("existing params", func(t *testing.T) {
		got, ch, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"list-users": {Params: map[string]*model.ParamOverlay{
			"limit": {Type: sp("string"), Enum: []string{"a"}, Desc: sp("d"), Required: bp(true)},
			"mode":  {Required: bp(false), Hide: true},
		}}}))
		assert.NoErr(t, err)
		ps := findOp(got, "list-users").Params
		assert.Eq(t, model.Param{Name: "limit", In: "query", Type: "string", Enum: []string{"a"}, Desc: "d", Required: true}, ps[0])
		assert.Eq(t, model.Param{Name: "mode", In: "query", Type: "string", Hidden: true}, ps[1])
		assert.Eq(t, []model.OverlayChange{
			oc("list-users", "params.query.limit.required"), oc("list-users", "params.query.limit.type"),
			oc("list-users", "params.query.limit.enum"), oc("list-users", "params.query.limit.desc"),
			oc("list-users", "params.query.mode.required"), oc("list-users", "params.query.mode.hide"),
		}, ch)
	})

	t.Run("param matching", func(t *testing.T) {
		two := func() *model.Index {
			idx := ovIndex()
			idx.Ops[3].Params = []model.Param{{Name: "id", In: "query", Type: "string"}, {Name: "id", In: "header", Type: "string"}}
			return idx
		}
		cases := []struct {
			name    string
			idx     *model.Index
			key     string
			po      model.ParamOverlay
			wantErr string
			check   func(*testing.T, *model.Operation)
		}{
			{name: "in.name form", idx: two(), key: "header.id", po: model.ParamOverlay{Desc: sp("h")},
				check: func(t *testing.T, op *model.Operation) {
					assert.Eq(t, "h", op.Params[1].Desc)
					assert.Eq(t, "", op.Params[0].Desc)
				}},
			{name: "in.name with matching in", idx: two(), key: "query.id", po: model.ParamOverlay{In: "query", Desc: sp("q")},
				check: func(t *testing.T, op *model.Operation) { assert.Eq(t, "q", op.Params[0].Desc) }},
			{name: "full name with in disambiguates", idx: two(), key: "id", po: model.ParamOverlay{In: "header", Desc: sp("h")},
				check: func(t *testing.T, op *model.Operation) { assert.Eq(t, "h", op.Params[1].Desc) }},
			{name: "ambiguous without in", idx: two(), key: "id", po: model.ParamOverlay{Desc: sp("x")}, wantErr: "ping.id: matches parameters in several locations, set in"},
			{name: "in mismatch", idx: ovIndex(), key: "limit", po: model.ParamOverlay{In: "header", Desc: sp("x")}, wantErr: "list-users.limit: in"},
			{name: "in mismatch dotted", idx: ovIndex(), key: "query.limit", po: model.ParamOverlay{In: "header"}, wantErr: "list-users.query.limit: in"},
			{name: "Authorization disambiguated", idx: ovIndex(), key: "Authorization", po: model.ParamOverlay{In: "header", Desc: sp("tok")},
				check: func(t *testing.T, op *model.Operation) { assert.Eq(t, "tok", op.Params[2].Desc) }},
			{name: "hide path param", idx: ovIndex(), key: "id", po: model.ParamOverlay{Hide: true}, wantErr: "get-user.id: path parameter cannot be hidden"},
			{name: "hide path param dotted", idx: ovIndex(), key: "path.id", po: model.ParamOverlay{Hide: true}, wantErr: "get-user.path.id: path parameter cannot be hidden"},
			{name: "hide required", idx: ovIndex(), key: "mode", po: model.ParamOverlay{Hide: true}, wantErr: "list-users.mode: hidden parameter is still required"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				opKey := "list-users"
				if strings.Contains(c.name, "path") {
					opKey = "get-user"
				}
				if c.idx.Ops[3].Params != nil { // the two-location fixture lives on ping
					opKey = "ping"
				}
				po := c.po
				got, _, _, err := spec.ApplyOverlay(c.idx, ovOf(map[string]*model.OpOverlay{opKey: {Params: map[string]*model.ParamOverlay{c.key: &po}}}))
				if c.wantErr != "" {
					assert.Err(t, err)
					assert.StrContains(t, err.Error(), "overlay o.yaml: "+c.wantErr)
					assert.Eq(t, model.ExitLocal, err.(*model.CodedError).C)
					return
				}
				assert.NoErr(t, err)
				c.check(t, findOp(got, opKey))
			})
		}
	})

	t.Run("new params", func(t *testing.T) {
		cases := []struct {
			name    string
			key     string
			po      model.ParamOverlay
			want    *model.Param
			wantErr string
		}{
			{name: "query default type", key: "debug", po: model.ParamOverlay{In: "query"}, want: &model.Param{Name: "debug", In: "query", Type: "string"}},
			{name: "query typed", key: "n", po: model.ParamOverlay{In: "query", Type: sp("integer"), Required: bp(true), Desc: sp("d"), Enum: []string{"1"}},
				want: &model.Param{Name: "n", In: "query", Type: "integer", Required: true, Desc: "d", Enum: []string{"1"}}},
			{name: "header", key: "X-Trace", po: model.ParamOverlay{In: "header"}, want: &model.Param{Name: "X-Trace", In: "header", Type: "string"}},
			{name: "in.name form", key: "header.X-Trace", po: model.ParamOverlay{}, want: &model.Param{Name: "X-Trace", In: "header", Type: "string"}},
			{name: "hidden optional", key: "q", po: model.ParamOverlay{In: "query", Hide: true}, want: &model.Param{Name: "q", In: "query", Type: "string", Hidden: true}},
			{name: "hidden required", key: "q", po: model.ParamOverlay{In: "query", Hide: true, Required: bp(true)}, wantErr: "ping.q: hidden parameter is still required"},
			{name: "path", key: "p", po: model.ParamOverlay{In: "path"}, wantErr: "ping.p: new parameter needs in: query or header"},
			{name: "no in", key: "p", po: model.ParamOverlay{}, wantErr: "ping.p: new parameter needs in"},
			{name: "bad token", key: "X Trace", po: model.ParamOverlay{In: "header"}, wantErr: "is not a valid token"},
			{name: "empty-ish token", key: "a/b", po: model.ParamOverlay{In: "header"}, wantErr: "is not a valid token"},
			{name: "dotted mismatch", key: "query.p", po: model.ParamOverlay{In: "header"}, wantErr: "ping.query.p: in"},
		}
		for _, h := range []string{"Host", "Content-Type", "Content-Length", "Transfer-Encoding", "Authorization", "Cookie", "Proxy-Authorization", "host", "COOKIE"} {
			cases = append(cases, struct {
				name    string
				key     string
				po      model.ParamOverlay
				want    *model.Param
				wantErr string
			}{name: "forbidden " + h, key: h, po: model.ParamOverlay{In: "header"}, wantErr: "cannot be added"})
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				po := c.po
				got, ch, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"ping": {Params: map[string]*model.ParamOverlay{c.key: &po}}}))
				if c.wantErr != "" {
					assert.Err(t, err)
					assert.StrContains(t, err.Error(), c.wantErr)
					return
				}
				assert.NoErr(t, err)
				assert.Eq(t, []model.Param{*c.want}, findOp(got, "ping").Params)
				assert.Eq(t, []model.OverlayChange{oc("ping", "params."+c.want.In+"."+c.want.Name+".added")}, ch)
			})
		}
		t.Run("Authorization query param is allowed", func(t *testing.T) {
			_, _, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{"ping": {Params: map[string]*model.ParamOverlay{"Authorization": {In: "query"}}}}))
			assert.NoErr(t, err)
		})
	})

	t.Run("rename", func(t *testing.T) {
		cases := []struct {
			name     string
			idx      *model.Index
			ov       map[string]*model.OpOverlay
			want     []string
			warnings []string
			wantErr  string
		}{
			{name: "simple", ov: map[string]*model.OpOverlay{"ping": {Name: sp("health")}},
				want: []string{"list-users", "get-user", "create-user", "health"}},
			{name: "swap", ov: map[string]*model.OpOverlay{"get-user": {Name: sp("create-user")}, "create-user": {Name: sp("get-user")}},
				want: []string{"list-users", "create-user", "get-user", "ping"}},
			{name: "collides with other op suffixes it", ov: map[string]*model.OpOverlay{"ping": {Name: sp("get-user")}},
				want:     []string{"list-users", "get-user-2", "create-user", "get-user"},
				warnings: []string{`operation GET /users/{id} renamed "get-user" to "get-user-2"`}},
			{name: "suffix skips reserved names", ov: map[string]*model.OpOverlay{"ping": {Name: sp("get-user")}},
				idx: func() *model.Index {
					i := ovIndex()
					i.Ops[2].Name = "get-user-2"
					return i
				}(),
				want:     []string{"list-users", "get-user-3", "get-user-2", "get-user"},
				warnings: []string{`operation GET /users/{id} renamed "get-user" to "get-user-3"`}},
			{name: "suffix skips another explicit name", ov: map[string]*model.OpOverlay{"ping": {Name: sp("get-user")}, "list-users": {Name: sp("get-user-2")}},
				want:     []string{"get-user-2", "get-user-3", "create-user", "get-user"},
				warnings: []string{`operation GET /users/{id} renamed "get-user" to "get-user-3"`}},
			{name: "duplicate explicit", ov: map[string]*model.OpOverlay{"ping": {Name: sp("x")}, "get-user": {Name: sp("x")}},
				wantErr: "overlay o.yaml: ping: name \"x\" is also set by \"get-user\""},
			{name: "empty name", ov: map[string]*model.OpOverlay{"ping": {Name: sp(" ")}}, wantErr: "ping: name must not be empty"},
			{name: "hidden op frees name", ov: map[string]*model.OpOverlay{"ping": {Name: sp("get-user")}, "get-user": {Hide: true}},
				want: []string{"list-users", "create-user", "get-user"}},
			{name: "rename to own name", ov: map[string]*model.OpOverlay{"ping": {Name: sp("ping")}},
				want: []string{"list-users", "get-user", "create-user", "ping"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				for range 20 { // map iteration order must not matter
					idx := c.idx
					if idx == nil {
						idx = ovIndex()
					}
					got, _, w, err := spec.ApplyOverlay(idx, ovOf(c.ov))
					if c.wantErr != "" {
						assert.Err(t, err)
						assert.StrContains(t, err.Error(), c.wantErr)
						continue
					}
					assert.NoErr(t, err)
					assert.Eq(t, c.want, opNames(got))
					assert.Eq(t, c.warnings, w)
				}
			})
		}
	})

	t.Run("records use final names", func(t *testing.T) {
		_, ch, _, err := spec.ApplyOverlay(ovIndex(), ovOf(map[string]*model.OpOverlay{
			"ping": {Name: sp("health"), Summary: sp("s")},
		}))
		assert.NoErr(t, err)
		assert.Eq(t, []model.OverlayChange{oc("health", "name"), oc("health", "summary")}, ch)
	})
}

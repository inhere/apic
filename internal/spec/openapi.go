package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/goccy/go-yaml"
	"github.com/inhere/apic/internal/model"
)

// OpenAPILoader loads Swagger 2.0 and OpenAPI 3.x definitions.
type OpenAPILoader struct{}

var _ Loader = OpenAPILoader{}

func (OpenAPILoader) Load(ctx context.Context, src model.Source, dir string) (*model.Index, error) {
	raw, _, _, err := Fetch(ctx, src, dir, FetchOptions{})
	if err != nil {
		return nil, err
	}
	index, _, err := loadOpenAPI(ctx, src, raw)
	return index, err
}

// loadOpenAPI keeps naming warnings available to the cache while preserving Loader's interface.
func loadOpenAPI(ctx context.Context, src model.Source, raw []byte) (*model.Index, []string, error) {
	data, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, nil, model.Errorf(model.ExitLocal, "invalid OpenAPI document: %v", err)
	}
	var version struct {
		Swagger string `json:"swagger"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, nil, model.Errorf(model.ExitLocal, "invalid OpenAPI document: %v", err)
	}
	var doc *openapi3.T
	index := &model.Index{}
	if version.Swagger == "2.0" {
		var v2 openapi2.T
		if err = json.Unmarshal(data, &v2); err == nil {
			doc, err = openapi2conv.ToV3(&v2)
		}
		if err != nil {
			return nil, nil, model.Errorf(model.ExitLocal, "invalid Swagger 2.0 document: %v", err)
		}
		if v2.BasePath != "/" {
			index.DefaultBasePath = v2.BasePath
		}
		if v2.Host != "" {
			scheme := "http"
			if len(v2.Schemes) > 0 {
				scheme = v2.Schemes[0]
			} else if src.URL != "" {
				if u, parseErr := url.Parse(src.URL); parseErr == nil && u.Scheme != "" {
					scheme = u.Scheme
				}
			}
			index.DefaultBaseURL = scheme + "://" + v2.Host
		}
	} else {
		loader := openapi3.NewLoader()
		loader.Context = ctx
		doc, err = loader.LoadFromData(raw)
		if err != nil {
			return nil, nil, model.Errorf(model.ExitLocal, "invalid OpenAPI document: %v", err)
		}
		if len(doc.Servers) > 0 && doc.Servers[0] != nil {
			if u, parseErr := url.Parse(doc.Servers[0].URL); parseErr == nil {
				index.DefaultBasePath = u.Path
				if u.Scheme != "" && u.Host != "" {
					index.DefaultBaseURL = u.Scheme + "://" + u.Host
				}
			}
		}
	}
	var ids []string
	if doc.Paths != nil {
		for _, path := range doc.Paths.Keys() {
			item := doc.Paths.Value(path)
			if item == nil {
				continue
			}
			methods := make([]string, 0, len(item.Operations()))
			for method := range item.Operations() {
				methods = append(methods, method)
			}
			sort.Strings(methods)
			for _, method := range methods {
				op := item.Operations()[method]
				if op == nil {
					continue
				}
				summary := op.Summary
				if summary == "" {
					summary = strings.SplitN(op.Description, "\n", 2)[0]
				}
				entry := model.Operation{Method: strings.ToUpper(method), Path: path, Summary: summary, Tags: op.Tags}
				params := append(openapi3.Parameters{}, item.Parameters...)
				params = append(params, op.Parameters...)
				positions := make(map[string]int)
				for _, ref := range params {
					if ref == nil || ref.Value == nil {
						continue
					}
					p := ref.Value
					if p.In != "path" && p.In != "query" && p.In != "header" {
						continue
					}
					field := model.Param{Name: p.Name, In: p.In, Required: p.Required, Desc: p.Description}
					if p.Schema != nil && p.Schema.Value != nil {
						field.Type = schemaType(p.Schema.Value)
						for _, v := range p.Schema.Value.Enum {
							field.Enum = append(field.Enum, fmt.Sprint(v))
						}
					}
					key := p.In + ":" + p.Name
					if at, ok := positions[key]; ok {
						entry.Params[at] = field
					} else {
						positions[key] = len(entry.Params)
						entry.Params = append(entry.Params, field)
					}
				}
				if op.RequestBody != nil && op.RequestBody.Value != nil {
					body := op.RequestBody.Value
					entry.Body = &model.BodySpec{Required: body.Required}
					for _, media := range sortedMedia(body.Content) {
						entry.Body.Type = bodyType(media)
						if content := body.Content[media]; content != nil {
							entry.Body.Schema = schemaSummary(content.Schema)
						}
						break
					}
				}
				if op.Responses != nil {
					for _, code := range op.Responses.Keys() {
						if !strings.HasPrefix(code, "2") {
							continue
						}
						ref := op.Responses.Value(code)
						if ref == nil || ref.Value == nil {
							continue
						}
						var summary model.SchemaSummary
						for _, media := range sortedMedia(ref.Value.Content) {
							if content := ref.Value.Content[media]; content != nil {
								summary = schemaSummary(content.Schema)
							}
							break
						}
						if entry.Responses == nil {
							entry.Responses = make(map[string]model.SchemaSummary)
						}
						entry.Responses[code] = summary
					}
				}
				index.Ops = append(index.Ops, entry)
				ids = append(ids, op.OperationID)
			}
		}
	}
	return index, AssignNames(index.Ops, ids), nil
}

func schemaType(s *openapi3.Schema) string {
	if s == nil || s.Type == nil || len(*s.Type) == 0 {
		return ""
	}
	return (*s.Type)[0]
}

func schemaSummary(ref *openapi3.SchemaRef) model.SchemaSummary {
	if ref == nil || ref.Value == nil {
		return model.SchemaSummary{}
	}
	s := ref.Value
	result := model.SchemaSummary{Type: schemaType(s)}
	required := make(map[string]bool, len(s.Required))
	for _, name := range s.Required {
		required[name] = true
	}
	keys := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		field := model.Field{Name: name, Required: required[name]}
		if prop := s.Properties[name]; prop != nil {
			field.Type = schemaType(prop.Value)
		}
		result.Fields = append(result.Fields, field)
	}
	return result
}

func bodyType(media string) string {
	switch {
	case media == "application/json", strings.HasPrefix(media, "application/json;"):
		return "json"
	case media == "application/x-www-form-urlencoded", strings.HasPrefix(media, "application/x-www-form-urlencoded;"):
		return "form"
	case media == "multipart/form-data", strings.HasPrefix(media, "multipart/form-data;"):
		return "multipart"
	case strings.Contains(media, "xml"):
		return "xml"
	default:
		return "raw"
	}
}

func sortedMedia(content openapi3.Content) []string {
	keys := make([]string, 0, len(content))
	for key := range content {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		rank := func(k string) int {
			switch bodyType(k) {
			case "json":
				return 0
			case "form":
				return 1
			case "multipart":
				return 2
			case "xml":
				return 3
			default:
				return 4
			}
		}
		if rank(keys[i]) != rank(keys[j]) {
			return rank(keys[i]) < rank(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}

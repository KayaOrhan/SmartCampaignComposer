package httpapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/docs"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
)

func TestOpenAPIResponseFields(t *testing.T) {
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(docs.OpenAPI), &spec); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"Campaign": campaign.Campaign{}, "CampaignPage": campaign.Page{}, "Audience": campaign.Preview{},
		"ApprovalPreview": campaign.ApprovalPreview{}, "Bounds": campaign.Bounds{}, "SMS": campaign.SMS{},
		"Email": campaign.Email{}, "Messages": campaign.Messages{}, "SavedMessages": campaign.SavedMessages{},
		"MessageValidation": campaign.MessageValidation{}, "DeliveryStatus": campaign.DeliveryStatus{},
		"DeliveryCounts": campaign.DeliveryCounts{}, "ErrorDetail": llm.Error{},
	} {
		fields := map[string]bool{}
		var collect func(reflect.Type)
		collect = func(typ reflect.Type) {
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				if field.Anonymous {
					collect(field.Type)
					continue
				}
				tag := strings.Split(field.Tag.Get("json"), ",")[0]
				if tag != "" && tag != "-" {
					fields[tag] = true
				}
			}
		}
		collect(reflect.TypeOf(value))
		properties := spec.Components.Schemas[name].Properties
		if len(properties) != len(fields) {
			t.Errorf("%s response field count differs from schema", name)
		}
		for field := range fields {
			if properties[field] == nil {
				t.Errorf("%s.%s missing from schema", name, field)
			}
		}
	}
}

func TestOpenAPIRoutesAndReferences(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(docs.OpenAPI), &spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.2.1" {
		t.Fatal("unexpected OpenAPI version")
	}
	paths := spec["paths"].(map[string]any)
	registered := map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(source, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" || len(call.Args) < 1 {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatal("route must be statically inspectable")
			}
			route, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.SplitN(route, " ", 2)
			if len(parts) != 2 {
				t.Fatal("unrecognized route", route)
			}
			registered[route] = true
			entry, ok := paths[parts[1]].(map[string]any)
			if !ok || entry[strings.ToLower(parts[0])] == nil {
				t.Error("undocumented route", route)
			}
			return true
		})
	}
	for path, item := range paths {
		for method := range item.(map[string]any) {
			if !registered[strings.ToUpper(method)+" "+path] {
				t.Error("documented nonexistent route", method, path)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatal("external runtime reference", ref)
				}
				var target any = spec
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
					object, ok := target.(map[string]any)
					if !ok || object[part] == nil {
						t.Fatal("unresolved reference", ref)
					}
					target = object[part]
				}
			}
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(spec)
	// Verify mutation preconditions and preview-only generation stay documented.
	for _, path := range []string{"/campaigns/{id}", "/campaigns/{id}/segment", "/campaigns/{id}/generate-segment", "/campaigns/{id}/messages", "/campaigns/{id}/approve"} {
		for method, raw := range paths[path].(map[string]any) {
			if method == "get" {
				continue
			}
			operation := raw.(map[string]any)
			found := false
			for _, p := range operation["parameters"].([]any) {
				if p.(map[string]any)["$ref"] == "#/components/parameters/IfMatch" {
					found = true
				}
			}
			if !found || operation["responses"].(map[string]any)["428"] == nil || operation["responses"].(map[string]any)["412"] == nil {
				t.Error("missing revision contract", method, path)
			}
		}
	}
	stream := paths["/campaigns/{id}/generate-messages"].(map[string]any)["post"].(map[string]any)
	if len(stream["parameters"].([]any)) != 1 || stream["requestBody"] != nil {
		t.Fatal("preview-only SSE must be bodyless without If-Match")
	}
}

func TestOpenAPIEndpoint(t *testing.T) {
	handler := Handler(func(context.Context) error { return nil }, time.Second, nil, nil, nil, time.Second)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != 200 || w.Body.String() != docs.OpenAPI+"\n" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("spec endpoint differs from canonical artifact")
	}
}

func TestOpenAPIPostmanCoverage(t *testing.T) {
	raw, err := os.ReadFile("../../postman/SmartCampaignComposer.postman_collection.json")
	if err != nil {
		t.Fatal(err)
	}
	var collection map[string]any
	if err := json.Unmarshal(raw, &collection); err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal([]byte(docs.OpenAPI), &spec); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func([]any)
	walk = func(items []any) {
		for _, raw := range items {
			item := raw.(map[string]any)
			if children, ok := item["item"].([]any); ok {
				walk(children)
			}
			request, ok := item["request"].(map[string]any)
			if !ok {
				continue
			}
			url, ok := request["url"].(string)
			if !ok || !strings.HasPrefix(url, "{{baseUrl}}") {
				continue
			}
			path := strings.SplitN(strings.TrimPrefix(url, "{{baseUrl}}"), "?", 2)[0]
			for _, key := range []string{"campaignId", "deletionId"} {
				path = strings.ReplaceAll(path, "{{"+key+"}}", "{id}")
			}
			seen[strings.ToLower(request["method"].(string))+" "+path] = true
		}
	}
	walk(collection["item"].([]any))
	for path, operations := range spec.Paths {
		for method := range operations {
			if !seen[method+" "+path] {
				t.Error("documented route lacks maintained Postman request", method, path)
			}
		}
	}
}

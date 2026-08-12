package openapi

import (
	"fmt"
	"reflect"
	"strings"
)

type RouteInfo struct {
	Method        string
	Path          string
	Summary       string
	Tags          []string
	ResponseModel any
	RequestModel  any
	NoAuth        bool
	HideDoc       bool
	ContentType   string
}

type SecurityScheme string

const (
	SecurityNone   SecurityScheme = ""
	SecurityBearer SecurityScheme = "bearer" // Authorization: Bearer <token>
	SecurityAPIKey SecurityScheme = "apikey" // X-API-Key: <key>
	SecurityBasic  SecurityScheme = "basic"  // Basic Auth (username + password)
)

func GenerateSchema(title string, description string, routes []*RouteInfo, security SecurityScheme) string {
	order := []string{}
	grouped := map[string][]string{}

	globalSecurityReq := ""
	if security != SecurityNone {
		globalSecurityReq = `,"security":[{"` + string(security) + `":[]}]`
	}

	for _, r := range routes {
		openapiPath := toOpenAPIPath(r.Path)

		var responsesBlock string
		if r.ResponseModel != nil {
			schema, example := reflectModel(r.ResponseModel)
			responsesBlock = fmt.Sprintf(
				`"responses":{"200":{"description":"OK","content":{"application/json":{"schema":%s,"example":%s}}}}`,
				schema, example)
		} else {
			responsesBlock = `"responses":{"200":{"description":"OK"}}`
		}

		securityReq := globalSecurityReq
		if r.NoAuth {
			securityReq = `,"security":[]`
		}

		var requestBlock string
		var parametersBlock string
		if r.RequestModel != nil {
			schema, example := reflectModel(r.RequestModel)

			contentType := r.ContentType
			if contentType == "" {
				contentType = "application/json"
			}

			requestBlock = fmt.Sprintf(
				`,"requestBody":{"required":true,"content":{%q:{"schema":%s,"example":%s}}}`,
				contentType, schema, example)

			skipHeaders := map[string]bool{}
			if security == SecurityBearer || security == SecurityBasic {
				skipHeaders["Authorization"] = true
			}
			if security == SecurityAPIKey {
				skipHeaders["X-API-Key"] = true
			}
			parametersBlock = reflectHeaderParams(r.RequestModel, skipHeaders)
		}

		// Extract and add path parameters
		pathParams := extractPathParams(r.Path)
		for _, param := range pathParams {
			pathParam := fmt.Sprintf(
				`{"name":%q,"in":"path","required":true,"description":%q,"schema":{"type":"string"}}`,
				param, param+" path parameter",
			)
			if parametersBlock == "" {
				parametersBlock = pathParam
			} else {
				parametersBlock = parametersBlock + "," + pathParam
			}
		}

		if parametersBlock != "" {
			parametersBlock = `,"parameters":[` + parametersBlock + `]`
		}

		summary := r.Summary
		if summary == "" {
			summary = strings.ToUpper(r.Method) + " " + r.Path
		}

		tags := `["default"]`
		if len(r.Tags) > 0 {
			quoted := make([]string, len(r.Tags))
			for i, t := range r.Tags {
				quoted[i] = `"` + t + `"`
			}
			tags = "[" + strings.Join(quoted, ",") + "]"
		}

		op := fmt.Sprintf(`%q:{"summary":%q,"tags":%s%s,%s%s%s}`,
			strings.ToLower(r.Method), summary, tags, parametersBlock, responsesBlock, requestBlock, securityReq)

		if _, exists := grouped[openapiPath]; !exists {
			order = append(order, openapiPath)
		}
		grouped[openapiPath] = append(grouped[openapiPath], op)
	}

	pathItems := make([]string, 0, len(order))
	for _, path := range order {
		ops := grouped[path]
		pathItems = append(pathItems,
			fmt.Sprintf(`%q:{%s}`, path, strings.Join(ops, ",")))
	}

	securitySchemes := buildSecuritySchemes(security)

	components := ""
	if securitySchemes != "" {
		components = `,"components":{"securitySchemes":{` + securitySchemes + `}}`
	}

	desc := ""
	if description != "" {
		desc = fmt.Sprintf(`,"description":%q`, description)
	}

	return fmt.Sprintf(`{
  "openapi":"3.0.0",
  "info":{"title":%q%s,"version":"1.0.0"},
  "paths":{%s}%s
}`, title, desc, strings.Join(pathItems, ","), components)
}

func buildSecuritySchemes(s SecurityScheme) string {
	switch s {
	case SecurityBearer:
		return `"bearer":{"type":"http","scheme":"bearer","bearerFormat":"JWT"}`
	case SecurityAPIKey:
		return `"apikey":{"type":"apiKey","in":"header","name":"X-API-Key"}`
	case SecurityBasic:
		return `"basic":{"type":"http","scheme":"basic"}`
	default:
		return ""
	}
}

func ScalarHTML(pageTitle, specURL string, darkMode bool, jsURL string) string {
	if jsURL == "" {
		jsURL = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@latest/dist/browser/standalone.js"
	}
	cssURL := strings.Replace(jsURL, ".js", ".css", 1)

	darkConfig := ""
	if darkMode {
		darkConfig = ` data-configuration='{"darkMode":true}'`
	}

	return fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <link rel="stylesheet" href="%s" />
  </head>
  <body>
    <script id="api-reference" data-url="%s"%s></script>
    <script src="%s"></script>
  </body>
</html>`, pageTitle, cssURL, specURL, darkConfig, jsURL)
}

func reflectHeaderParams(model any, skip map[string]bool) string {
	t := reflect.TypeOf(model)
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		t = t.Elem()
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	if t.Kind() != reflect.Struct {
		return ""
	}

	var params []string
	reflectHeaderParamsRecursive(t, skip, &params)
	return strings.Join(params, ",")
}

func reflectHeaderParamsRecursive(t reflect.Type, skip map[string]bool, params *[]string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			reflectHeaderParamsRecursive(field.Type, skip, params)
			continue
		}

		headerName := field.Tag.Get("header")
		queryName := field.Tag.Get("query")

		if headerName != "" {
			if skip[headerName] {
				continue
			}

			required := strings.Contains(field.Tag.Get("validate"), "required")
			desc := field.Tag.Get("desc")
			if desc == "" {
				desc = headerName + " header"
			}

			*params = append(*params, fmt.Sprintf(
				`{"name":%q,"in":"header","required":%v,"description":%q,"schema":{"type":"string"}}`,
				headerName, required, desc,
			))
		}

		if queryName != "" {
			required := strings.Contains(field.Tag.Get("validate"), "required")
			desc := field.Tag.Get("desc")
			if desc == "" {
				desc = queryName + " parameter"
			}

			*params = append(*params, fmt.Sprintf(
				`{"name":%q,"in":"query","required":%v,"description":%q,"schema":{"type":"string"}}`,
				queryName, required, desc,
			))
		}
	}
}

func extractPathParams(path string) []string {
	var params []string
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			params = append(params, p[1:])
		} else if strings.HasPrefix(p, "*") {
			params = append(params, p[1:])
		}
	}
	return params
}

func toOpenAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		} else if strings.HasPrefix(p, "*") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func reflectModel(model any) (schema string, example string) {
	if model == nil {
		return `{"type":"object"}`, `{}`
	}
	t := reflect.TypeOf(model)
	v := reflect.ValueOf(model)
	return reflectField(t, v, "")
}

func reflectStruct(t reflect.Type, v reflect.Value) (schema string, example string) {
	var schemaProps []string
	var exampleProps []string
	var required []string

	reflectStructRecursive(t, v, &schemaProps, &exampleProps, &required)

	req := ""
	if len(required) > 0 {
		req = fmt.Sprintf(`,"required":[%s]`, strings.Join(required, ","))
	}

	schema = fmt.Sprintf(`{"type":"object","properties":{%s}%s}`,
		strings.Join(schemaProps, ","), req)
	example = fmt.Sprintf(`{%s}`, strings.Join(exampleProps, ","))
	return
}

func reflectStructRecursive(t reflect.Type, v reflect.Value, schemaProps, exampleProps, required *[]string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			reflectStructRecursive(field.Type, fv, schemaProps, exampleProps, required)
			continue
		}

		if field.Tag.Get("header") != "" || field.Tag.Get("query") != "" || field.Tag.Get("param") != "" {
			continue
		}

		jsonName := field.Tag.Get("json")
		if jsonName == "" {
			jsonName = field.Name
		} else {
			jsonName = strings.Split(jsonName, ",")[0]
		}
		if jsonName == "-" {
			continue
		}

		if strings.Contains(field.Tag.Get("validate"), "required") {
			*required = append(*required, fmt.Sprintf("%q", jsonName))
		}

		desc := field.Tag.Get("desc")
		fieldSchema, fieldExample := reflectField(field.Type, fv, desc)
		*schemaProps = append(*schemaProps, fmt.Sprintf("%q:%s", jsonName, fieldSchema))
		*exampleProps = append(*exampleProps, fmt.Sprintf("%q:%s", jsonName, fieldExample))
	}
}

func reflectField(t reflect.Type, v reflect.Value, desc string) (schema, example string) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if v.IsValid() && !v.IsNil() {
			v = v.Elem()
		} else {
			v = reflect.New(t).Elem()
		}
	}

	descPart := ""
	if desc != "" {
		descPart = fmt.Sprintf(`,"description":%q`, desc)
	}

	switch t.Kind() {
	case reflect.String:
		val := ""
		if v.IsValid() && v.Kind() == reflect.String && v.String() != "" {
			val = v.String()
		} else {
			val = exampleStringFor(strings.ToLower(t.Name()))
		}
		schema = fmt.Sprintf(`{"type":"string"%s}`, descPart)
		example = fmt.Sprintf("%q", val)

	case reflect.Bool:
		val := false
		if v.IsValid() && v.Kind() == reflect.Bool {
			val = v.Bool()
		}
		schema = fmt.Sprintf(`{"type":"boolean"%s}`, descPart)
		example = fmt.Sprintf("%v", val)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val := int64(0)
		if v.IsValid() {
			val = v.Int()
		}
		schema = fmt.Sprintf(`{"type":"integer"%s}`, descPart)
		example = fmt.Sprintf("%d", val)

	case reflect.Float32, reflect.Float64:
		val := float64(0)
		if v.IsValid() {
			val = v.Float()
		}
		schema = fmt.Sprintf(`{"type":"number"%s}`, descPart)
		example = fmt.Sprintf("%g", val)

	case reflect.Slice:
		itemT := t.Elem()
		itemSchema, itemExample := reflectField(itemT, reflect.New(itemT).Elem(), "")
		schema = fmt.Sprintf(`{"type":"array","items":%s%s}`, itemSchema, descPart)
		example = fmt.Sprintf(`[%s]`, itemExample)

	case reflect.Struct:
		s, e := reflectStruct(t, v)
		schema = s
		example = e

	case reflect.Map:
		itemT := t.Elem()
		itemSchema, itemExample := reflectField(itemT, reflect.New(itemT).Elem(), "")
		schema = fmt.Sprintf(`{"type":"object","additionalProperties":%s%s}`, itemSchema, descPart)
		if v.IsValid() && v.Kind() == reflect.Map && v.Len() > 0 {
			var parts []string
			iter := v.MapRange()
			for iter.Next() {
				k := iter.Key()
				mv := iter.Value()
				if k.Kind() == reflect.String {
					_, valExample := reflectField(mv.Type(), mv, "")
					parts = append(parts, fmt.Sprintf("%q:%s", k.String(), valExample))
				}
			}
			example = fmt.Sprintf("{%s}", strings.Join(parts, ","))
		} else {
			example = fmt.Sprintf(`{"key":%s}`, itemExample)
		}

	default:
		schema = fmt.Sprintf(`{"type":"string"%s}`, descPart)
		example = `""`
	}

	return
}

func exampleStringFor(name string) string {
	switch name {
	case "email":
		return "user@example.com"
	case "name":
		return "kurosaki ichigo"
	case "password":
		return "••••••••"
	case "phone":
		return "+66812345678"
	case "url", "image", "avatar":
		return "https://example.com/image.png"
	case "token":
		return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
	default:
		return "string"
	}
}

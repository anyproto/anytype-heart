package apiv2

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type responseContractDocument struct {
	Components struct {
		Responses map[string]responseContractResponse `json:"responses" yaml:"responses"`
		Schemas   map[string]struct {
			Required   []string       `json:"required" yaml:"required"`
			Properties map[string]any `json:"properties" yaml:"properties"`
		} `json:"schemas" yaml:"schemas"`
	} `json:"components" yaml:"components"`
	Paths map[string]map[string]responseContractOperation `json:"paths" yaml:"paths"`
}

type responseContractOperation struct {
	OperationId string                              `json:"operationId" yaml:"operationId"`
	Responses   map[string]responseContractResponse `json:"responses" yaml:"responses"`
	Security    []map[string][]string               `json:"security" yaml:"security"`
}

type responseContractResponse struct {
	Ref         string `json:"$ref" yaml:"$ref"`
	Description string `json:"description" yaml:"description"`
	Content     map[string]struct {
		Schema struct {
			Ref    string `json:"$ref" yaml:"$ref"`
			Type   string `json:"type" yaml:"type"`
			Format string `json:"format" yaml:"format"`
			AnyOf  []struct {
				Ref string `json:"$ref" yaml:"$ref"`
			} `json:"anyOf" yaml:"anyOf"`
		} `json:"schema" yaml:"schema"`
	} `json:"content" yaml:"content"`
}

func responseContractOperations(t *testing.T, doc responseContractDocument) map[string]responseContractOperation {
	t.Helper()
	operations := map[string]responseContractOperation{}
	for _, pathItem := range doc.Paths {
		for _, operation := range pathItem {
			if operation.OperationId == "" {
				continue
			}
			require.NotContains(t, operations, operation.OperationId)
			operations[operation.OperationId] = operation
		}
	}
	return operations
}

func responseStatusInventory(operations map[string]responseContractOperation) map[string][]string {
	inventory := make(map[string][]string, len(operations))
	for operationId, operation := range operations {
		statuses := make([]string, 0, len(operation.Responses))
		for status := range operation.Responses {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		inventory[operationId] = statuses
	}
	return inventory
}

func stringSet(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func TestOpenAPIGrantDiscoveryResponses(t *testing.T) {
	for _, name := range []string{"../docs/v2/openapi.json", "../docs/v2/openapi.yaml"} {
		body, err := os.ReadFile(name)
		require.NoError(t, err)
		var doc responseContractDocument
		if strings.HasSuffix(name, ".json") {
			require.NoError(t, json.Unmarshal(body, &doc))
		} else {
			require.NoError(t, yaml.Unmarshal(body, &doc))
		}
		spaces := doc.Components.Schemas["ListSpacesResponse"]
		assert.Contains(t, spaces.Properties, "has_not_granted_spaces", name)
		assert.Contains(t, spaces.Required, "has_not_granted_spaces", name)
		assert.Contains(t, doc.Components.Schemas["CreateApiKeyResponse"].Required, "grant", name)
		assert.ElementsMatch(t, []string{"all_spaces", "space_ids", "permission"}, doc.Components.Schemas["ApiKeyGrant"].Required, name)
		response := doc.Paths["/v2/spaces"]["get"].Responses["200"]
		assert.Equal(t, "#/components/schemas/ListSpacesResponse", response.Content["application/json"].Schema.Ref, name)
	}
}

func TestV2OpenAPIResponsePolicies(t *testing.T) {
	jsonBody, err := os.ReadFile("../docs/v2/openapi.json")
	require.NoError(t, err)
	var jsonDoc responseContractDocument
	require.NoError(t, json.Unmarshal(jsonBody, &jsonDoc))
	jsonOperations := responseContractOperations(t, jsonDoc)
	require.Len(t, jsonOperations, 56)

	yamlBody, err := os.ReadFile("../docs/v2/openapi.yaml")
	require.NoError(t, err)
	var yamlDoc responseContractDocument
	require.NoError(t, yaml.Unmarshal(yamlBody, &yamlDoc))
	yamlOperations := responseContractOperations(t, yamlDoc)
	assert.Equal(t, responseStatusInventory(jsonOperations), responseStatusInventory(yamlOperations),
		"the two checked-in OpenAPI forms must declare the same operation/status pairs")
	for form, doc := range map[string]responseContractDocument{"json": jsonDoc, "yaml": yamlDoc} {
		for path, operationId := range map[string]string{
			"/v2/auth/challenges": "create_auth_challenge",
			"/v2/auth/api_keys":   "create_api_key",
		} {
			operation := doc.Paths[path]["post"]
			require.Equal(t, operationId, operation.OperationId, "%s must document %s", form, path)
			require.NotNil(t, operation.Security)
			require.Empty(t, operation.Security, "%s needs no existing key", path)
		}
		file := doc.Paths["/v2/spaces/{space_id}/files/{file_id}/content"]
		require.Equal(t, "download_file", file["get"].OperationId, form)
		require.Equal(t, "head_file", file["head"].OperationId, form)
		for _, status := range []string{"200", "206"} {
			content := file["get"].Responses[status].Content
			require.Len(t, content, 1, "%s file %s must describe bytes only", form, status)
			assert.Equal(t, "string", content["application/octet-stream"].Schema.Type)
			assert.Equal(t, "binary", content["application/octet-stream"].Schema.Format)
		}
		assert.Empty(t, file["get"].Responses["304"].Content)
		assert.Empty(t, file["head"].Responses["200"].Content)
		assert.Empty(t, file["head"].Responses["304"].Content)
		for _, schema := range []string{"Space", "SpaceRow", "MemberRow"} {
			assert.Contains(t, doc.Components.Schemas[schema].Properties, "icon_image", "%s %s", form, schema)
		}
	}

	// Every space-scoped operation resolves the space FIRST, so a well-shaped
	// id for a space that does not exist is a 404 on all of them. Asserting
	// the RULE rather than a list is what keeps a route added later honest:
	// the earlier sweep declared the router-level policies and left 404 to
	// per-handler annotations, so it reached only 28 of 37 and nobody noticed
	// that create_object could answer an undeclared status.
	for path, pathItem := range jsonDoc.Paths {
		if !strings.Contains(path, "{space_id}") {
			continue
		}
		for method, operation := range pathItem {
			if operation.OperationId == "" {
				continue
			}
			assert.Contains(t, operation.Responses, "404",
				"%s %s resolves a space, so it can answer 404", strings.ToUpper(method), path)
		}
	}

	for _, component := range []string{"BadRequest", "Unauthorized", "Forbidden", "Conflict", "NotFound", "RequestTooLarge", "RateLimited"} {
		assert.Contains(t, jsonDoc.Components.Responses, component)
		assert.Contains(t, yamlDoc.Components.Responses, component)
	}
	assert.Equal(t, "#/components/schemas/Error",
		jsonDoc.Components.Responses["NotFound"].Content["application/json"].Schema.Ref,
		"the derived 404 answers in the C6 envelope")

	// swag emits no schema-level `required`, so the whole envelope read as
	// optional — including the members that are always on the wire. A
	// generated client then types them optional and a consumer branches on a
	// field that cannot be absent (§8.53). These are injected by
	// scripts/fix_openapi_v2.py and nothing else would notice if they stopped.
	for name, want := range map[string][]string{
		"Error":             {"status", "code", "message", "issues"},
		"Issue":             {"message"},
		"UnauthorizedError": {"object", "status", "code", "message"},
		"ForbiddenError":    {"object", "status", "code", "message"},
	} {
		for form, doc := range map[string]responseContractDocument{"json": jsonDoc, "yaml": yamlDoc} {
			schema, ok := doc.Components.Schemas[name]
			require.True(t, ok, "%s: %s schema is missing", form, name)
			assert.ElementsMatch(t, want, schema.Required, "%s: %s.required", form, name)
			for _, field := range schema.Required {
				assert.Contains(t, schema.Properties, field,
					"%s: %s.required names a property the schema does not have", form, name)
			}
		}
	}

	forbiddenAlternatives := jsonDoc.Components.Responses["Forbidden"].Content["application/json"].Schema.AnyOf
	require.Len(t, forbiddenAlternatives, 2, "403 accepts either real envelope; oneOf is invalid because the schemas overlap")
	assert.Equal(t, "#/components/schemas/ForbiddenError", forbiddenAlternatives[0].Ref)
	assert.Equal(t, "#/components/schemas/Error", forbiddenAlternatives[1].Ref)

	pairCount := 0
	for _, operation := range jsonOperations {
		pairCount += len(operation.Responses)
	}
	assert.Equal(t, 364, pairCount, "the checked-in response inventory changes only deliberately")

	dryRunCreates := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_object", "create_property",
		"create_query", "create_space", "create_template", "create_type", "create_widget", "upload_file",
	)
	idempotent := stringSet(
		"validate", "create_space", "update_space", "create_object", "create_template", "create_type",
		"update_type", "delete_type", "create_property", "update_property", "delete_property", "create_query",
		"create_collection", "upload_file", "patch_object", "delete_object", "create_chat", "add_chat_message",
		"edit_chat_message", "delete_chat_message", "toggle_chat_reaction", "read_chat",
		"publish_chat_status",
		"create_widget", "update_widget", "delete_widget",
		// create_discussion is a create with a dry run, but not in
		// dryRunCreates: its 200 is ALSO the answer for an object that already
		// has a discussion, and that description says so rather than
		// claiming the 200 is a dry run only
		"create_discussion",
	)
	requestBodyLimited := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_property", "create_query", "create_space",
		"edit_chat_message", "read_chat", "toggle_chat_reaction", "update_property", "update_space", "update_type", "upload_file",
		"publish_chat_status",
		"create_widget", "update_widget",
	)

	// A concurrency cap is not a rate limit: the chat stream refuses when too
	// many are held AT ONCE, in v2's own envelope, so it declares its own 429
	// rather than the shared limiter's legacy one.
	resourceLimited := stringSet("stream_chat_messages")

	for operationId, operation := range jsonOperations {
		if operationId == "create_auth_challenge" || operationId == "create_api_key" {
			// Shared pairing handlers run outside all authenticated v2 gates.
			assert.Len(t, operation.Responses, 4)
			assert.Contains(t, operation.Responses, "201")
			assert.Equal(t, "#/components/schemas/ValidationError", operation.Responses["400"].Content["application/json"].Schema.Ref)
			assert.Equal(t, "#/components/schemas/ForbiddenError", operation.Responses["403"].Content["application/json"].Schema.Ref)
			assert.Equal(t, "#/components/schemas/ServerError", operation.Responses["500"].Content["application/json"].Schema.Ref)
			continue
		}
		for status, component := range map[string]string{
			"401": "Unauthorized",
			"403": "Forbidden",
		} {
			response, ok := operation.Responses[status]
			require.True(t, ok, "%s must declare shared %s", operationId, status)
			assert.Equal(t, "#/components/responses/"+component, response.Ref, "%s %s envelope", operationId, status)
		}
		assert.Contains(t, operation.Responses, "400", "%s is reached by shared query validation", operationId)

		if dryRunCreates[operationId] {
			assert.Contains(t, operation.Responses, "201", "%s real create", operationId)
			response, ok := operation.Responses["200"]
			require.True(t, ok, "%s dry run", operationId)
			assert.Equal(t, "Dry run; validation result without committing", response.Description)
		}
		if idempotent[operationId] {
			assert.Equal(t, "#/components/responses/Conflict", operation.Responses["409"].Ref, "%s idempotency conflict", operationId)
		} else {
			assert.NotContains(t, operation.Responses, "409", "%s is not idempotency-guarded", operationId)
		}
		if idempotent[operationId] && operationId != "validate" {
			assert.Equal(t, "#/components/responses/RateLimited", operation.Responses["429"].Ref, "%s write limiter", operationId)
		} else if resourceLimited[operationId] {
			assert.Contains(t, operation.Responses, "429",
				"%s caps a resource, so it declares its own 429", operationId)
		} else {
			assert.NotContains(t, operation.Responses, "429",
				"%s neither write-limits nor caps a resource", operationId)
		}
		if requestBodyLimited[operationId] {
			assert.Equal(t, "#/components/responses/RequestTooLarge", operation.Responses["413"].Ref, "%s body cap", operationId)
		} else {
			assert.NotContains(t, operation.Responses, "413", "%s has no assertion-linked body cap", operationId)
		}
	}
}

// TestV2OpenAPIQueryPaths pins the Query resource's PATHS and operation ids
// in the generated document. The object's product name is Query while its
// internal uniqueKey is still "set", and that split is what makes a rename
// easy to half-apply: the annotations could say query while the checked-in
// document — the artifact consumers actually read — still says sets, simply
// because `make openapi` was not re-run. Pinning both the wanted paths and
// the absence of the old spelling makes that a red test, not a stale doc.
func TestV2OpenAPIQueryPaths(t *testing.T) {
	for _, name := range []string{"../docs/v2/openapi.json", "../docs/v2/openapi.yaml"} {
		body, err := os.ReadFile(name)
		require.NoError(t, err)
		var doc responseContractDocument
		if strings.HasSuffix(name, ".json") {
			require.NoError(t, json.Unmarshal(body, &doc))
		} else {
			require.NoError(t, yaml.Unmarshal(body, &doc))
		}

		for path, wantOperations := range map[string]map[string]string{
			"/v2/spaces/{space_id}/queries":                             {"post": "create_query"},
			"/v2/spaces/{space_id}/queries/{query_id}/objects":          {"get": "get_query_objects"},
			"/v2/spaces/{space_id}/queries/{query_id}/views":            {"get": "get_query_views"},
			"/v2/spaces/{space_id}/collections":                         {"post": "create_collection"},
			"/v2/spaces/{space_id}/collections/{collection_id}/objects": {"get": "get_collection_objects"},
			"/v2/spaces/{space_id}/collections/{collection_id}/views":   {"get": "get_collection_views"},
		} {
			pathItem, ok := doc.Paths[path]
			require.True(t, ok, "%s must document %s", name, path)
			for method, operationId := range wantOperations {
				require.Contains(t, pathItem, method, "%s %s %s", name, method, path)
				assert.Equal(t, operationId, pathItem[method].OperationId, "%s %s %s", name, method, path)
			}
		}

		for path := range doc.Paths {
			assert.NotContains(t, path, "/sets",
				"%s still documents the pre-rename noun in %s — the REST resource is queries (the type key stays \"set\")", name, path)
			assert.NotContains(t, path, "{set_id}",
				"%s still documents the pre-rename path param in %s", name, path)
		}
	}
}

// TestV2WidgetSchemasArePinned keeps the served widget shapes honest: the
// result flattens its row (no nested `widget` member), a link row omits its
// limit, and the three mutations advertise the replay header.
func TestV2WidgetSchemasArePinned(t *testing.T) {
	body, err := os.ReadFile("../docs/v2/openapi.json")
	require.NoError(t, err)
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))

	yamlBody, err := os.ReadFile("../docs/v2/openapi.yaml")
	require.NoError(t, err)
	var yamlDoc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Type string `yaml:"type"`
					Ref  string `yaml:"$ref"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(yamlBody, &yamlDoc))

	// the members, their types, and the same shape in both forms
	rowTypes := map[string]string{"id": "string", "scope": "string", "target": "string", "layout": "string", "limit": "integer", "view_id": "string"}
	// placed is a reference to its own component, so its type sits there
	resultTypes := map[string]string{"dry_run": "boolean", "removed": "boolean", "warnings": "array", "placed": ""}
	for member, typ := range rowTypes {
		resultTypes[member] = typ
	}
	for schema, want := range map[string]map[string]string{"WidgetRow": rowTypes, "WidgetResult": resultTypes} {
		jsonProps := doc.Components.Schemas[schema].Properties
		yamlProps := yamlDoc.Components.Schemas[schema].Properties
		require.Len(t, jsonProps, len(want), schema)
		require.Len(t, yamlProps, len(want), schema)
		for member, typ := range want {
			var prop struct {
				Type string `json:"type"`
				Ref  string `json:"$ref"`
			}
			require.Contains(t, jsonProps, member, "%s.%s (the row is flattened into the result)", schema, member)
			require.NoError(t, json.Unmarshal(jsonProps[member], &prop))
			if typ == "" {
				assert.Equal(t, "#/components/schemas/WidgetPlaced", prop.Ref, "%s.%s", schema, member)
				continue
			}
			assert.Equal(t, typ, prop.Type, "%s.%s", schema, member)
			assert.Equal(t, typ, yamlProps[member].Type, "%s.%s in yaml", schema, member)
		}
	}

	// the placement receipt's own members, in both forms
	placedTypes := map[string]string{"after": "string", "before": "string", "position": "string"}
	require.Len(t, doc.Components.Schemas["WidgetPlaced"].Properties, len(placedTypes))
	for member, typ := range placedTypes {
		var prop struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(doc.Components.Schemas["WidgetPlaced"].Properties[member], &prop))
		assert.Equal(t, typ, prop.Type, "WidgetPlaced.%s", member)
		assert.Equal(t, typ, yamlDoc.Components.Schemas["WidgetPlaced"].Properties[member].Type, "WidgetPlaced.%s in yaml", member)
	}
	assert.Equal(t, "#/components/schemas/WidgetPlaced", yamlDoc.Components.Schemas["WidgetResult"].Properties["placed"].Ref, "placed in yaml")

	// the replay header on the three mutations, and the success bindings
	var bindings struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(body, &bindings))
	// nested item references, both forms
	var nested struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Items struct {
						Ref string `json:"$ref" yaml:"$ref"`
					} `json:"items" yaml:"items"`
				} `json:"properties" yaml:"properties"`
				Required []string `json:"required" yaml:"required"`
			} `json:"schemas" yaml:"schemas"`
		} `json:"components" yaml:"components"`
	}
	// each form decoded into its own value: a yaml decode into a value the
	// json decode already filled would keep the json's components and let a
	// component missing from the yaml pass
	nestedYaml := nested
	require.NoError(t, json.Unmarshal(body, &nested))
	require.NoError(t, yaml.Unmarshal(yamlBody, &nestedYaml))
	for form, n := range map[string]*struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Items struct {
						Ref string `json:"$ref" yaml:"$ref"`
					} `json:"items" yaml:"items"`
				} `json:"properties" yaml:"properties"`
				Required []string `json:"required" yaml:"required"`
			} `json:"schemas" yaml:"schemas"`
		} `json:"components" yaml:"components"`
	}{"json": &nested, "yaml": &nestedYaml} {
		for _, component := range []string{"WidgetRow", "WidgetResult", "WidgetPlaced", "ListResponse-WidgetRow"} {
			require.Contains(t, n.Components.Schemas, component, "%s in %s", component, form)
		}
		assert.Equal(t, "#/components/schemas/Issue", n.Components.Schemas["WidgetResult"].Properties["warnings"].Items.Ref, form)
		assert.Equal(t, "#/components/schemas/WidgetRow", n.Components.Schemas["ListResponse-WidgetRow"].Properties["data"].Items.Ref, form)
		assert.NotContains(t, n.Components.Schemas["WidgetRow"].Required, "limit", "limit stays optional in %s: a link row omits it", form)
	}

	var yamlBindings struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `yaml:"name"`
				In   string `yaml:"in"`
			} `yaml:"parameters"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(yamlBody, &yamlBindings))

	for _, route := range []struct{ path, method, status, ref string }{
		{"/v2/spaces/{space_id}/widgets", "post", "201", "#/components/schemas/WidgetResult"},
		{"/v2/spaces/{space_id}/widgets", "post", "200", "#/components/schemas/WidgetResult"},
		{"/v2/spaces/{space_id}/widgets/{widget_id}", "patch", "200", "#/components/schemas/WidgetResult"},
		{"/v2/spaces/{space_id}/widgets/{widget_id}", "delete", "200", "#/components/schemas/WidgetResult"},
		{"/v2/spaces/{space_id}/widgets", "get", "200", "#/components/schemas/ListResponse-WidgetRow"},
	} {
		assert.Equal(t, route.ref, bindings.Paths[route.path][route.method].Responses[route.status].Content["application/json"].Schema.Ref,
			"%s %s %s", route.method, route.path, route.status)
		assert.Equal(t, route.ref, yamlBindings.Paths[route.path][route.method].Responses[route.status].Content["application/json"].Schema.Ref,
			"%s %s %s in yaml", route.method, route.path, route.status)
		if route.method == "get" {
			continue
		}
		var found, foundYaml bool
		for _, param := range doc.Paths[route.path][route.method].Parameters {
			if param.Name == "Idempotency-Key" && param.In == "header" {
				found = true
			}
		}
		for _, param := range yamlBindings.Paths[route.path][route.method].Parameters {
			if param.Name == "Idempotency-Key" && param.In == "header" {
				foundYaml = true
			}
		}
		assert.True(t, found, "%s %s advertises Idempotency-Key", route.method, route.path)
		assert.True(t, foundYaml, "%s %s advertises Idempotency-Key in yaml", route.method, route.path)
	}
}

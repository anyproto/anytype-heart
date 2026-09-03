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
}

type responseContractResponse struct {
	Ref         string `json:"$ref" yaml:"$ref"`
	Description string `json:"description" yaml:"description"`
	Content     map[string]struct {
		Schema struct {
			Ref   string `json:"$ref" yaml:"$ref"`
			AnyOf []struct {
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

func TestV2OpenAPIResponsePolicies(t *testing.T) {
	jsonBody, err := os.ReadFile("../docs/v2/openapi.json")
	require.NoError(t, err)
	var jsonDoc responseContractDocument
	require.NoError(t, json.Unmarshal(jsonBody, &jsonDoc))
	jsonOperations := responseContractOperations(t, jsonDoc)
	require.Len(t, jsonOperations, 46)

	yamlBody, err := os.ReadFile("../docs/v2/openapi.yaml")
	require.NoError(t, err)
	var yamlDoc responseContractDocument
	require.NoError(t, yaml.Unmarshal(yamlBody, &yamlDoc))
	yamlOperations := responseContractOperations(t, yamlDoc)
	assert.Equal(t, responseStatusInventory(jsonOperations), responseStatusInventory(yamlOperations),
		"the two checked-in OpenAPI forms must declare the same operation/status pairs")

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
	assert.Equal(t, 292, pairCount, "the checked-in response inventory changes only deliberately")

	dryRunCreates := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_object", "create_property",
		"create_query", "create_space", "create_template", "create_type", "upload_file",
	)
	idempotent := stringSet(
		"validate", "create_space", "update_space", "create_object", "create_template", "create_type",
		"update_type", "delete_type", "create_property", "update_property", "delete_property", "create_query",
		"create_collection", "upload_file", "patch_object", "delete_object", "create_chat", "add_chat_message",
		"edit_chat_message", "delete_chat_message", "toggle_chat_reaction", "read_chat",
	)
	requestBodyLimited := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_property", "create_query", "create_space",
		"edit_chat_message", "read_chat", "toggle_chat_reaction", "update_property", "update_space", "update_type", "upload_file",
	)

	// A concurrency cap is not a rate limit: the chat stream refuses when too
	// many are held AT ONCE, in v2's own envelope, so it declares its own 429
	// rather than the shared limiter's legacy one.
	resourceLimited := stringSet("stream_chat_messages")

	for operationId, operation := range jsonOperations {
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

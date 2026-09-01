package apiv2

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type responseContractDocument struct {
	Components struct {
		Responses map[string]responseContractResponse `json:"responses" yaml:"responses"`
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
	require.Len(t, jsonOperations, 45)

	yamlBody, err := os.ReadFile("../docs/v2/openapi.yaml")
	require.NoError(t, err)
	var yamlDoc responseContractDocument
	require.NoError(t, yaml.Unmarshal(yamlBody, &yamlDoc))
	yamlOperations := responseContractOperations(t, yamlDoc)
	assert.Equal(t, responseStatusInventory(jsonOperations), responseStatusInventory(yamlOperations),
		"the two checked-in OpenAPI forms must declare the same operation/status pairs")

	for _, component := range []string{"BadRequest", "Unauthorized", "Forbidden", "Conflict", "RequestTooLarge", "RateLimited"} {
		assert.Contains(t, jsonDoc.Components.Responses, component)
		assert.Contains(t, yamlDoc.Components.Responses, component)
	}
	forbiddenAlternatives := jsonDoc.Components.Responses["Forbidden"].Content["application/json"].Schema.AnyOf
	require.Len(t, forbiddenAlternatives, 2, "403 accepts either real envelope; oneOf is invalid because the schemas overlap")
	assert.Equal(t, "#/components/schemas/ForbiddenError", forbiddenAlternatives[0].Ref)
	assert.Equal(t, "#/components/schemas/Error", forbiddenAlternatives[1].Ref)

	pairCount := 0
	for _, operation := range jsonOperations {
		pairCount += len(operation.Responses)
	}
	assert.Equal(t, 277, pairCount, "the checked-in response inventory changes only deliberately")

	dryRunCreates := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_object", "create_property",
		"create_set", "create_space", "create_template", "create_type", "upload_file",
	)
	idempotent := stringSet(
		"validate", "create_space", "update_space", "create_object", "create_template", "create_type",
		"update_type", "delete_type", "create_property", "update_property", "delete_property", "create_set",
		"create_collection", "upload_file", "patch_object", "delete_object", "create_chat", "add_chat_message",
		"edit_chat_message", "delete_chat_message", "toggle_chat_reaction", "read_chat",
	)
	requestBodyLimited := stringSet(
		"add_chat_message", "create_chat", "create_collection", "create_property", "create_set", "create_space",
		"edit_chat_message", "read_chat", "toggle_chat_reaction", "update_property", "update_space", "update_type", "upload_file",
	)

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
		} else {
			assert.NotContains(t, operation.Responses, "429", "%s is not write-limited", operationId)
		}
		if requestBodyLimited[operationId] {
			assert.Equal(t, "#/components/responses/RequestTooLarge", operation.Responses["413"].Ref, "%s body cap", operationId)
		} else {
			assert.NotContains(t, operation.Responses, "413", "%s has no assertion-linked body cap", operationId)
		}
	}
}

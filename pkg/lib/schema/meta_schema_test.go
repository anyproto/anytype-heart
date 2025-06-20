package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestMetaSchemaValidation(t *testing.T) {
	// Load the meta-schema
	metaSchemaPath, err := filepath.Abs("anytype-schema.meta.json")
	require.NoError(t, err)
	metaSchemaLoader := gojsonschema.NewReferenceLoader("file://" + metaSchemaPath)

	// Test that our test schemas validate against the meta-schema
	testSchemas := []string{
		"testdata/task_schema.json",
		"testdata/project_schema.json",
		"testdata/system_object_schema.json",
	}

	for _, schemaPath := range testSchemas {
		t.Run(filepath.Base(schemaPath), func(t *testing.T) {
			absSchemaPath, err := filepath.Abs(schemaPath)
			require.NoError(t, err)
			documentLoader := gojsonschema.NewReferenceLoader("file://" + absSchemaPath)

			result, err := gojsonschema.Validate(metaSchemaLoader, documentLoader)
			require.NoError(t, err)

			if !result.Valid() {
				for _, desc := range result.Errors() {
					t.Errorf("- %s", desc)
				}
				t.Fail()
			}
		})
	}
}

func TestMetaSchemaItself(t *testing.T) {
	// The meta-schema should validate against JSON Schema Draft 7
	metaSchemaPath := filepath.Join("anytype-schema.meta.json")

	// Load the meta-schema
	metaSchemaData, err := os.ReadFile(metaSchemaPath)
	require.NoError(t, err)

	var metaSchema map[string]interface{}
	err = json.Unmarshal(metaSchemaData, &metaSchema)
	require.NoError(t, err)

	// Verify it has the required fields
	require.Contains(t, metaSchema, "$schema")
	require.Contains(t, metaSchema, "$id")
	require.Contains(t, metaSchema, "title")
	require.Contains(t, metaSchema, "type")
	require.Contains(t, metaSchema, "properties")
	require.Contains(t, metaSchema, "definitions")

	// Verify the $id follows our pattern
	schemaId := metaSchema["$id"].(string)
	require.Contains(t, schemaId, "https://schemas.anytype.io/meta/v1.0.0/schema.json")
}

func TestInvalidSchemaDetection(t *testing.T) {
	// Load the meta-schema
	metaSchemaPath, err := filepath.Abs("anytype-schema.meta.json")
	require.NoError(t, err)
	metaSchemaLoader := gojsonschema.NewReferenceLoader("file://" + metaSchemaPath)

	// Test invalid schemas
	invalidSchemas := []struct {
		name   string
		schema string
		errors []string
	}{
		{
			name: "missing required fields",
			schema: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object"
			}`,
			errors: []string{"$id", "title", "x-type-key", "properties"},
		},
		{
			name: "invalid $id format",
			schema: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "invalid-id",
				"type": "object",
				"title": "Test",
				"x-type-key": "test",
				"properties": {
					"id": {
						"type": "string",
						"description": "Unique identifier of the Anytype object",
						"readOnly": true,
						"x-order": 0,
						"x-key": "id",
						"x-hidden": true
					}
				}
			}`,
			errors: []string{"pattern"},
		},
		{
			name: "missing x-format in relation",
			schema: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "urn:anytype:schema:2025-01-01:test:type-test:gen-1.0",
				"type": "object",
				"title": "Test",
				"x-type-key": "test",
				"properties": {
					"id": {
						"type": "string",
						"description": "Unique identifier of the Anytype object",
						"readOnly": true,
						"x-order": 0,
						"x-key": "id",
						"x-hidden": true
					},
					"Name": {
						"type": "string",
						"x-key": "name"
					}
				}
			}`,
			errors: []string{"x-format"},
		},
	}

	for _, tc := range invalidSchemas {
		t.Run(tc.name, func(t *testing.T) {
			documentLoader := gojsonschema.NewStringLoader(tc.schema)

			result, err := gojsonschema.Validate(metaSchemaLoader, documentLoader)
			require.NoError(t, err)

			require.False(t, result.Valid(), "Schema should be invalid")

			// Check that expected errors are present
			errorMessages := []string{}
			for _, desc := range result.Errors() {
				errorMessages = append(errorMessages, desc.String())
			}

			for _, expectedError := range tc.errors {
				found := false
				for _, msg := range errorMessages {
					if contains(msg, expectedError) {
						found = true
						break
					}
				}
				require.True(t, found, "Expected error containing '%s' not found in: %v", expectedError, errorMessages)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || len(substr) > 0 && len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

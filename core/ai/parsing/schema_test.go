package parsing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertDefinitionToSchema(t *testing.T) {
	t.Run("primitive", func(t *testing.T) {
		result := convertDefinitionToSchema("string")
		expected := map[string]interface{}{"type": "string"}
		assert.Equal(t, expected, result)
	})

	t.Run("nested object", func(t *testing.T) {
		input := map[string]interface{}{
			"name": "string",
			"age":  "number",
		}
		result := convertDefinitionToSchema(input)
		expected := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
				"age":  map[string]interface{}{"type": "number"},
			},
			"required":             []string{"name", "age"},
			"additionalProperties": false,
		}
		// Use JSON marshalling to compare (avoids ordering issues)
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})

	t.Run("array non-empty", func(t *testing.T) {
		input := []interface{}{"string"}
		result := convertDefinitionToSchema(input)
		expected := map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})

	t.Run("array empty", func(t *testing.T) {
		input := []interface{}{}
		result := convertDefinitionToSchema(input)
		expected := map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})

	t.Run("unrecognized type", func(t *testing.T) {
		input := 123 // unsupported type should fall back to "string"
		result := convertDefinitionToSchema(input)
		expected := map[string]interface{}{"type": "string"}
		assert.Equal(t, expected, result)
	})

	t.Run("nested object with nested object", func(t *testing.T) {
		input := map[string]interface{}{
			"user": map[string]interface{}{
				"name": "string",
				"address": map[string]interface{}{
					"street": "string",
					"city":   "string",
				},
			},
		}
		result := convertDefinitionToSchema(input)
		expected := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
						"address": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"street": map[string]interface{}{"type": "string"},
								"city":   map[string]interface{}{"type": "string"},
							},
							"required":             []string{"street", "city"},
							"additionalProperties": false,
						},
					},
					"required":             []string{"name", "address"},
					"additionalProperties": false,
				},
			},
			"required":             []string{"user"},
			"additionalProperties": false,
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})
}

func TestBuildJSONSchema(t *testing.T) {
	t.Run("primitive definition", func(t *testing.T) {
		result := BuildJSONSchema("test", "string")
		expected := map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   "test_response",
				"strict": true,
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"test": map[string]interface{}{"type": "string"},
					},
					"required":             []string{"test"},
					"additionalProperties": false,
				},
			},
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})

	t.Run("nested object definition", func(t *testing.T) {
		def := map[string]interface{}{
			"name": "string",
			"date": "string",
		}
		result := BuildJSONSchema("relations", def)
		innerExpected := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
				"date": map[string]interface{}{"type": "string"},
			},
			"required":             []string{"name", "date"},
			"additionalProperties": false,
		}
		expected := map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   "relations_response",
				"strict": true,
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"relations": innerExpected,
					},
					"required":             []string{"relations"},
					"additionalProperties": false,
				},
			},
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})

	t.Run("array definition", func(t *testing.T) {
		def := []interface{}{"number"}
		result := BuildJSONSchema("numbers", def)
		innerExpected := map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "number",
			},
		}
		expected := map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   "numbers_response",
				"strict": true,
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"numbers": innerExpected,
					},
					"required":             []string{"numbers"},
					"additionalProperties": false,
				},
			},
		}
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})
}

func TestSingleStringSchema(t *testing.T) {
	t.Run("single string equals BuildJSONSchema with 'string'", func(t *testing.T) {
		result := SingleStringSchema("example")
		expected := BuildJSONSchema("example", "string")
		expectedJSON, _ := json.Marshal(expected)
		resultJSON, _ := json.Marshal(result)
		assert.JSONEq(t, string(expectedJSON), string(resultJSON))
	})
}

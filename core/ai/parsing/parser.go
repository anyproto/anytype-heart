package parsing

import "fmt"

// ResponseParser defines how to parse and extract content from the model's JSON response.
// It abstracts away the differences between different types of tasks (e.g., WritingTools vs. Autofill).
type ResponseParser interface {
	// NewResponseStruct returns a new instance of the response structure into which JSON can be unmarshalled.
	NewResponseStruct() interface{}

	// ModeToField maps modes to the name of the field in the response struct that should be returned.
	ModeToField() map[int]string

	// ModeToSchema maps modes to the structure of the response schema.
	ModeToSchema() map[int]func(key string) map[string]interface{}

	// ExtractContent uses the mode and the already-unmarshalled response struct to
	// return the final answer string. Returns an error if extraction fails.
	ExtractContent(mode int, response interface{}) (string, error)
}

// CheckEmpty checks if the content is empty and returns an error if it is.
func CheckEmpty(content string, mode int) error {
	if content == "" {
		return fmt.Errorf("content is empty for mode %d", mode)
	}
	return nil
}

// FieldDef defines the configuration for a field in the JSON schema.
type FieldDef struct {
	Type  string // e.g., "string", "number", etc.
	Array bool   // if true, the field is an array of the specified type
}

// FlexibleSchema dynamically creates a JSON schema.
// - key: a base key used to form the schema name (e.g., "myField" or "relations").
// - fields: a map of field names to their definitions.
// - required: a slice of required field names; if nil, all fields will be required.
func FlexibleSchema(key string, fields map[string]FieldDef, required []string) map[string]interface{} {
	// If no required slice is provided, default to all keys from the fields map.
	if required == nil {
		for field := range fields {
			required = append(required, field)
		}
	}

	// Build the properties map for the JSON schema.
	properties := make(map[string]interface{})
	for field, def := range fields {
		if def.Array {
			properties[field] = map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": def.Type,
				},
			}
		} else {
			properties[field] = map[string]interface{}{
				"type": def.Type,
			}
		}
	}

	// Build and return the final JSON schema.
	return map[string]interface{}{
		"type": "json_schema",
		"json_schema": map[string]interface{}{
			"name":   key + "_response",
			"strict": true,
			"schema": map[string]interface{}{
				"type":                 "object",
				"properties":           properties,
				"additionalProperties": false,
				"required":             required,
			},
		},
	}
}

func SingleStringSchema(key string) map[string]interface{} {
	fields := map[string]FieldDef{
		key: {Type: "string"},
	}
	return FlexibleSchema(key, fields, nil)
}

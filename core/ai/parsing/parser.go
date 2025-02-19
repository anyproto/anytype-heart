package parsing

import (
	"encoding/json"
	"fmt"
)

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
	ExtractContent(jsonData string, mode int) (ParsedResult, error)
}

// CheckEmpty checks if the content is empty and returns an error if it is.
func CheckEmpty(content string, mode int) error {
	if content == "" {
		return fmt.Errorf("content is empty for mode %d", mode)
	}
	return nil
}

// convertDefinitionToSchema recursively converts a user-defined schema definition
// (which can be a primitive type, a map for nested objects, or a slice for arrays)
// into the corresponding JSON Schema fragment.
func convertDefinitionToSchema(def interface{}) map[string]interface{} {
	switch t := def.(type) {
	case string:
		// If the definition is a string, assume it represents a primitive type.
		return map[string]interface{}{
			"type": t,
		}
	case map[string]interface{}:
		// If it's a map, treat it as an object with properties.
		properties := make(map[string]interface{})
		required := []string{}
		for key, subDef := range t {
			properties[key] = convertDefinitionToSchema(subDef)
			// Assume every key in the map is required.
			required = append(required, key)
		}
		return map[string]interface{}{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		}
	case []interface{}:
		// If it's a slice, assume it defines an array.
		// We use the first element of the slice to define the item type.
		if len(t) == 0 {
			// Default to an array of strings if the slice is empty.
			return map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			}
		}
		return map[string]interface{}{
			"type":  "array",
			"items": convertDefinitionToSchema(t[0]),
		}
	default:
		// Fallback to a string type if the definition is not recognized.
		return map[string]interface{}{
			"type": "string",
		}
	}
}

// BuildJSONSchema wraps the inner schema (produced by convertDefinitionToSchema)
// under a top-level property defined by 'key'. This is useful if you need a JSON
// schema that expects an object with a single property (e.g., { "relations": { ... } }).
func BuildJSONSchema(key string, def interface{}) map[string]interface{} {
	innerSchema := convertDefinitionToSchema(def)
	return map[string]interface{}{
		"type": "json_schema",
		"json_schema": map[string]interface{}{
			"name":   key + "_response",
			"strict": true,
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					key: innerSchema,
				},
				"required":             []string{key},
				"additionalProperties": false,
			},
		},
	}
}

func SingleStringSchema(key string) map[string]interface{} {
	return BuildJSONSchema(key, "string")
}

type ParsedResult struct {
	// Raw holds the underlying parsed value.
	Raw interface{}
}

// AsString returns the parsed result as a JSON-encoded string.
// If the underlying value is a map, it will be marshaled to JSON.
func (pr ParsedResult) AsString() (string, error) {
	switch v := pr.Raw.(type) {
	case string:
		return v, nil
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unexpected type %T", v)
	}
}

// AsMap returns the parsed result as a map[string]string.
// If the underlying value is a string, it will attempt to unmarshal it.
func (pr ParsedResult) AsMap() (map[string]string, error) {
	switch v := pr.Raw.(type) {
	case map[string]interface{}:
		result := make(map[string]string)
		for key, value := range v {
			strValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected value type %T for key %s", value, key)
			}
			result[key] = strValue
		}
		return result, nil
	case string:
		var m map[string]string
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", v)
	}
}

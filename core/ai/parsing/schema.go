package parsing

// SingleStringSchema returns a JSON Schema fragment that expects a single string value.
func SingleStringSchema(key string) map[string]interface{} {
	return BuildJSONSchema(key, "string")
}

// BuildJSONSchema wraps the inner schema under a top-level property defined by 'key'.
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

// convertDefinitionToSchema recursively converts a user-defined schema definition (which can be a primitive type,
// a map for nested objects, or a slice for arrays) into the corresponding JSON Schema fragment.
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

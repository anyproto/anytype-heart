package wrapper

// schema.go renders each tool's argument schema from the same Arg table the
// CLI flags and the GBNF come from. Every schema is C13
// strict-mode-compatible: flat, non-recursive, additionalProperties: false,
// bounded. The one non-scalar shape (ArgObject) is the setProperties value
// map — string keys to scalar-or-array values, the same shape the shipped
// setProperties op schema uses.

import (
	"encoding/json"
	"fmt"
)

// argObjectValues is the value schema of an ArgObject map: scalars, arrays
// or null — never nested objects (C13: no recursion).
var argObjectValues = map[string]any{
	"type": []string{"string", "number", "boolean", "array", "null"},
}

// toolSchema renders one tool's strict JSON argument schema.
func toolSchema(t Tool) (json.RawMessage, error) {
	properties := map[string]any{}
	required := []string{}
	for _, a := range t.Args {
		prop := map[string]any{}
		switch a.Type {
		case ArgString:
			prop["type"] = "string"
			if len(a.Enum) > 0 {
				prop["enum"] = a.Enum
			} else {
				if a.Required && !a.AllowEmpty {
					prop["minLength"] = 1
				}
				if a.MaxLen > 0 {
					prop["maxLength"] = a.MaxLen
				}
			}
		case ArgInteger:
			prop["type"] = "integer"
			if a.Min != 0 || a.Max != 0 {
				prop["minimum"] = a.Min
				prop["maximum"] = a.Max
			}
		case ArgBoolean:
			prop["type"] = "boolean"
		case ArgObject:
			prop["type"] = "object"
			prop["maxProperties"] = 64
			prop["additionalProperties"] = argObjectValues
		default:
			return nil, fmt.Errorf("tool %s arg %s: unknown arg type %q", t.Name, a.Name, a.Type)
		}
		if a.Description != "" {
			prop["description"] = a.Description
		}
		properties[a.Name] = prop
		if a.Required {
			required = append(required, a.Name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode tool schema: %w", err)
	}
	return data, nil
}

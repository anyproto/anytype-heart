package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// JSONSchemaParser parses JSON Schema format into Schema
type JSONSchemaParser struct{}

// NewJSONSchemaParser creates a new JSON Schema parser
func NewJSONSchemaParser() Parser {
	return &JSONSchemaParser{}
}

// Parse reads a JSON Schema and converts it to Schema
func (p *JSONSchemaParser) Parse(reader io.Reader) (*Schema, error) {
	var jsonSchema map[string]interface{}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&jsonSchema); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	schema := NewSchema()

	// Parse as a type schema
	if jsonSchema["type"] == "object" && jsonSchema["properties"] != nil {
		t, err := p.parseTypeFromSchema(jsonSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to parse type: %w", err)
		}

		// Parse relations from properties
		properties, ok := jsonSchema["properties"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid properties format")
		}

		for propName, propValue := range properties {
			prop, ok := propValue.(map[string]interface{})
			if !ok {
				continue
			}

			rel, err := p.parseRelationFromProperty(propName, prop)
			if err != nil {
				continue // Skip invalid relations
			}

			// Add relation to schema
			if err := schema.AddRelation(rel); err != nil {
				return nil, fmt.Errorf("failed to add relation %s: %w", rel.Key, err)
			}

			// Add relation to type
			isFeatured := getBoolField(prop, "x-featured")
			order := getIntField(prop, "x-order")

			// Use order to determine if it should be featured (first few relations)
			if isFeatured || order > 0 && order <= 3 {
				t.AddRelation(rel.Key, true)
			} else {
				t.AddRelation(rel.Key, false)
			}
		}

		// Set type for schema
		if err := schema.SetType(t); err != nil {
			return nil, fmt.Errorf("failed to set type: %w", err)
		}
	}

	return schema, nil
}

// parseTypeFromSchema parses a Type from JSON Schema
func (p *JSONSchemaParser) parseTypeFromSchema(jsonSchema map[string]interface{}) (*Type, error) {
	t := &Type{
		Extension: make(map[string]interface{}),
	}

	// Get type name from title
	if title, ok := jsonSchema["title"].(string); ok {
		t.Name = title
	} else {
		return nil, fmt.Errorf("type name (title) is required")
	}

	// Get type key from x-type-key
	if typeKey, ok := jsonSchema["x-type-key"].(string); ok {
		t.Key = typeKey
	} else {
		// Generate key from name
		t.Key = strings.ToLower(strings.ReplaceAll(t.Name, " ", "_"))
	}

	// Get description
	if desc, ok := jsonSchema["description"].(string); ok {
		t.Description = desc
	}

	// Get Anytype-specific extensions
	if plural, ok := jsonSchema["x-plural"].(string); ok {
		t.PluralName = plural
	}

	if emoji, ok := jsonSchema["x-icon-emoji"].(string); ok {
		t.IconEmoji = emoji
	}

	if iconName, ok := jsonSchema["x-icon-name"].(string); ok {
		t.IconImage = iconName
	}

	// Store other x-* fields in extension
	for key, value := range jsonSchema {
		if strings.HasPrefix(key, "x-") && !isKnownExtension(key) {
			t.Extension[key] = value
		}
	}

	return t, nil
}

// parseRelationFromProperty parses a Relation from a JSON Schema property
func (p *JSONSchemaParser) parseRelationFromProperty(name string, prop map[string]interface{}) (*Relation, error) {
	r := &Relation{
		Name:      name,
		Extension: make(map[string]interface{}),
	}

	// Get key from x-key or generate from name
	if key, ok := prop["x-key"].(string); ok {
		r.Key = key
	} else {
		r.Key = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	}

	// Get description
	if desc, ok := prop["description"].(string); ok {
		r.Description = desc
	}

	// Determine format from x-format first
	if xFormat, ok := prop["x-format"].(string); ok {
		r.Format = parseFormatString(xFormat)
	} else {
		// Infer format from schema structure
		r.Format = inferRelationFormat(prop)
	}

	// Get format-specific properties
	switch r.Format {
	case model.RelationFormat_status:
		if enum, ok := prop["enum"].([]interface{}); ok {
			for _, v := range enum {
				if s, ok := v.(string); ok {
					r.Options = append(r.Options, s)
				}
			}
		}

	case model.RelationFormat_tag:
		if examples, ok := prop["examples"].([]interface{}); ok {
			for _, v := range examples {
				if s, ok := v.(string); ok {
					r.Examples = append(r.Examples, s)
				}
			}
		}

	case model.RelationFormat_date:
		if format, ok := prop["format"].(string); ok {
			r.IncludeTime = (format == "date-time")
		}

	case model.RelationFormat_object:
		// Parse object types from items schema if available
		if items, ok := prop["items"].(map[string]interface{}); ok {
			if props, ok := items["properties"].(map[string]interface{}); ok {
				if objTypeProp, ok := props["Object type"].(map[string]interface{}); ok {
					if enum, ok := objTypeProp["enum"].([]interface{}); ok {
						for _, v := range enum {
							if s, ok := v.(string); ok {
								r.ObjectTypes = append(r.ObjectTypes, s)
							}
						}
					}
				}
			}
		}
	}

	// Check if read-only
	if readOnly, ok := prop["readOnly"].(bool); ok {
		r.IsReadOnly = readOnly
	}

	// Check if multi-value (array type)
	if propType, ok := prop["type"].(string); ok && propType == "array" {
		r.IsMulti = true
	}

	// Store other fields in extension
	for key, value := range prop {
		if strings.HasPrefix(key, "x-") && key != "x-key" && key != "x-format" && key != "x-featured" && key != "x-order" {
			r.Extension[key] = value
		}
	}

	return r, nil
}

// inferRelationFormat infers the relation format from JSON Schema structure
func inferRelationFormat(prop map[string]interface{}) model.RelationFormat {
	propType := getStringField(prop, "type")
	format := getStringField(prop, "format")

	// Check for specific formats first
	switch format {
	case "email":
		return model.RelationFormat_email
	case "uri":
		return model.RelationFormat_url
	case "date":
		return model.RelationFormat_date
	case "date-time":
		return model.RelationFormat_date
	}

	// Check for enum (status)
	if _, hasEnum := prop["enum"]; hasEnum {
		return model.RelationFormat_status
	}

	// Check for boolean
	if propType == "boolean" {
		return model.RelationFormat_checkbox
	}

	// Check for number
	if propType == "number" || propType == "integer" {
		return model.RelationFormat_number
	}

	// Check for array
	if propType == "array" {
		if items, ok := prop["items"].(map[string]interface{}); ok {
			itemType := getStringField(items, "type")
			if itemType == "object" {
				return model.RelationFormat_object
			}
			// Array of strings with examples = tag
			if _, hasExamples := prop["examples"]; hasExamples {
				return model.RelationFormat_tag
			}
		}
		// Default array to tag
		return model.RelationFormat_tag
	}

	// Check for object type
	if propType == "object" {
		return model.RelationFormat_object
	}

	// Default to short text
	return model.RelationFormat_shorttext
}

// parseFormatString parses x-format string to RelationFormat
func parseFormatString(format string) model.RelationFormat {
	switch format {
	case "shorttext":
		return model.RelationFormat_shorttext
	case "longtext":
		return model.RelationFormat_longtext
	case "number":
		return model.RelationFormat_number
	case "checkbox":
		return model.RelationFormat_checkbox
	case "date":
		return model.RelationFormat_date
	case "tag":
		return model.RelationFormat_tag
	case "status":
		return model.RelationFormat_status
	case "email":
		return model.RelationFormat_email
	case "url":
		return model.RelationFormat_url
	case "phone":
		return model.RelationFormat_phone
	case "file":
		return model.RelationFormat_file
	case "object":
		return model.RelationFormat_object
	default:
		return model.RelationFormat_shorttext
	}
}

// Helper functions

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBoolField(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getIntField(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func isKnownExtension(key string) bool {
	knownExtensions := []string{
		"x-type-key", "x-plural", "x-icon-emoji", "x-icon-name",
		"x-key", "x-format", "x-featured", "x-order",
	}
	for _, known := range knownExtensions {
		if key == known {
			return true
		}
	}
	return false
}

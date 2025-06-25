package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/globalsign/mgo/bson"

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

	// Check schema version if present
	if versionStr, ok := jsonSchema[anytypeFieldSchemaVersion].(string); ok {
		if err := p.checkSchemaVersion(versionStr); err != nil {
			return nil, err
		}
	}

	// Parse as a type schema
	if jsonSchema[jsonSchemaFieldType] == jsonSchemaTypeObject && jsonSchema[jsonSchemaFieldProperties] != nil {
		t, err := p.parseTypeFromSchema(jsonSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to parse type: %w", err)
		}

		// Parse relations from properties
		properties, ok := jsonSchema[jsonSchemaFieldProperties].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid properties format")
		}

		// First, parse all relations and add them to schema
		relationsByKey := make(map[string]*Relation)
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

			relationsByKey[rel.Key] = rel
		}

		// Collect relations with their order to sort them properly
		type relationInfo struct {
			key      string
			order    int
			featured bool
			hidden   bool
		}

		var relationsWithOrder []relationInfo

		// Add relations to type based on x-featured and x-hidden flags on properties
		for _, propValue := range properties {
			prop, ok := propValue.(map[string]interface{})
			if !ok {
				continue
			}

			// Get the relation key
			relKey := getStringField(prop, anytypeFieldKey)
			if relKey == "" {
				relKey = bson.NewObjectId().Hex() // Generate a new key if not specified
			}

			// Skip only the "id" property, but include "type" if it has x-featured
			if relKey == propertyNameID {
				continue
			}

			// For "type" relation, only include if explicitly featured
			isFeatured := getBoolField(prop, anytypeFieldFeatured)
			isHidden := getBoolField(prop, anytypeFieldHidden)
			// Get the order
			order := 999 // Default high value for unordered items
			if orderVal, ok := prop[anytypeFieldOrder]; ok {
				switch v := orderVal.(type) {
				case float64:
					order = int(v)
				case int:
					order = v
				}
			}

			relationsWithOrder = append(relationsWithOrder, relationInfo{
				key:      relKey,
				order:    order,
				featured: isFeatured,
				hidden:   isHidden,
			})
		}

		// Sort relations by order
		sort.Slice(relationsWithOrder, func(i, j int) bool {
			return relationsWithOrder[i].order < relationsWithOrder[j].order
		})

		// Add relations in sorted order
		for _, rel := range relationsWithOrder {
			t.AddRelation(rel.key, rel.featured, rel.hidden)
		}

		// Add system properties to hidden relations if not already present
		p.addSystemPropertiesToType(t)

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
	if title, ok := jsonSchema[jsonSchemaFieldTitle].(string); ok {
		t.Name = title
	} else {
		return nil, fmt.Errorf("type name (title) is required")
	}

	// Get type key from x-type-key
	if typeKey, ok := jsonSchema[anytypeFieldTypeKey].(string); ok {
		t.Key = typeKey
	} else {
		// Generate a new key if not specified
		t.Key = bson.NewObjectId().Hex()
	}

	// Get description
	if desc, ok := jsonSchema[jsonSchemaFieldDescription].(string); ok {
		t.Description = desc
	}

	// Get Anytype-specific extensions
	if plural, ok := jsonSchema[anytypeFieldPlural].(string); ok {
		t.PluralName = plural
	}

	if emoji, ok := jsonSchema[anytypeFieldIconEmoji].(string); ok {
		t.IconEmoji = emoji
	}

	if iconName, ok := jsonSchema[anytypeFieldIconName].(string); ok {
		t.IconName = iconName
	}

	// Store other x-* fields in extension
	for key, value := range jsonSchema {
		if strings.HasPrefix(key, extensionPrefix) && !isKnownExtension(key) {
			t.Extension[key] = value
		}
	}

	// Check if this is a collection type by looking for Collection property
	if props, ok := jsonSchema[jsonSchemaFieldProperties].(map[string]interface{}); ok {
		if collProp, hasCollection := props[propertyNameCollection]; hasCollection {
			// Verify it's an array property
			if collMap, ok := collProp.(map[string]interface{}); ok {
				if propType, ok := collMap[jsonSchemaFieldType].(string); ok && propType == jsonSchemaTypeArray {
					t.Layout = model.ObjectType_collection
				}
			}
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
	if key, ok := prop[anytypeFieldKey].(string); ok {
		r.Key = key
	} else {
		r.Key = bson.NewObjectId().Hex() // Generate a new key if not specified
	}

	// Get description
	if desc, ok := prop[jsonSchemaFieldDescription].(string); ok {
		r.Description = desc
	}

	// Determine format from x-format first
	if xFormat, ok := prop[anytypeFieldFormat].(string); ok {
		r.Format = parseFormatString(xFormat)
	} else {
		// Infer format from schema structure
		r.Format = inferRelationFormat(prop)
	}

	// Get format-specific properties
	switch r.Format {
	case model.RelationFormat_status:
		if enum, ok := prop[jsonSchemaFieldEnum].([]interface{}); ok {
			for _, v := range enum {
				if s, ok := v.(string); ok {
					r.Options = append(r.Options, s)
				}
			}
		}

	case model.RelationFormat_tag:
		if examples, ok := prop[jsonSchemaFieldExamples].([]interface{}); ok {
			for _, v := range examples {
				if s, ok := v.(string); ok {
					r.Examples = append(r.Examples, s)
				}
			}
		}

	case model.RelationFormat_date:
		if format, ok := prop[jsonSchemaFieldFormat].(string); ok {
			r.IncludeTime = (format == jsonSchemaFormatDateTime)
		}

	case model.RelationFormat_object:
		// Parse object types from x-object-types
		if objTypes, ok := prop[anytypeFieldObjectTypes].([]interface{}); ok {
			for _, v := range objTypes {
				if s, ok := v.(string); ok {
					r.ObjectTypes = append(r.ObjectTypes, s)
				}
			}
		}
	}

	// Check if read-only
	if readOnly, ok := prop[jsonSchemaFieldReadOnly].(bool); ok {
		r.IsReadOnly = readOnly
	}

	// Check if multi-value (array type)
	if propType, ok := prop[jsonSchemaFieldType].(string); ok && propType == jsonSchemaTypeArray {
		r.IsMulti = true
	}

	// Store other fields in extension
	for key, value := range prop {
		if strings.HasPrefix(key, extensionPrefix) && key != anytypeFieldKey && key != anytypeFieldFormat && key != anytypeFieldFeatured && key != anytypeFieldOrder {
			r.Extension[key] = value
		}
	}

	return r, nil
}

// inferRelationFormat infers the relation format from JSON Schema structure
func inferRelationFormat(prop map[string]interface{}) model.RelationFormat {
	propType := getStringField(prop, jsonSchemaFieldType)
	format := getStringField(prop, jsonSchemaFieldFormat)

	// Check for specific formats first
	switch format {
	case jsonSchemaFormatEmail:
		return model.RelationFormat_email
	case jsonSchemaFormatURI:
		return model.RelationFormat_url
	case jsonSchemaFormatDate:
		return model.RelationFormat_date
	case jsonSchemaFormatDateTime:
		return model.RelationFormat_date
	}

	// Check for enum (status)
	if _, hasEnum := prop[jsonSchemaFieldEnum]; hasEnum {
		return model.RelationFormat_status
	}

	// Check for boolean
	if propType == jsonSchemaTypeBoolean {
		return model.RelationFormat_checkbox
	}

	// Check for number
	if propType == jsonSchemaTypeNumber || propType == jsonSchemaTypeInteger {
		return model.RelationFormat_number
	}

	// Check for array
	if propType == jsonSchemaTypeArray {
		if items, ok := prop[jsonSchemaFieldItems].(map[string]interface{}); ok {
			itemType := getStringField(items, jsonSchemaFieldType)
			if itemType == jsonSchemaTypeObject {
				return model.RelationFormat_object
			}
			// Array of strings with examples = tag
			if _, hasExamples := prop[jsonSchemaFieldExamples]; hasExamples {
				return model.RelationFormat_tag
			}
		}
		// Default array to tag
		return model.RelationFormat_tag
	}

	// Check for object type
	if propType == jsonSchemaTypeObject {
		return model.RelationFormat_object
	}

	// Default to short text
	return model.RelationFormat_shorttext
}

// parseFormatString parses x-format string to RelationFormat
func parseFormatString(format string) model.RelationFormat {
	switch format {
	case anytypeFormatShortText:
		return model.RelationFormat_shorttext
	case anytypeFormatLongText:
		return model.RelationFormat_longtext
	case "number":
		return model.RelationFormat_number
	case anytypeFormatCheckbox:
		return model.RelationFormat_checkbox
	case "date":
		return model.RelationFormat_date
	case anytypeFormatTag:
		return model.RelationFormat_tag
	case anytypeFormatStatus:
		return model.RelationFormat_status
	case "email":
		return model.RelationFormat_email
	case anytypeFormatURL:
		return model.RelationFormat_url
	case anytypeFormatPhone:
		return model.RelationFormat_phone
	case anytypeFormatFile:
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
		anytypeFieldTypeKey, anytypeFieldPlural, anytypeFieldIconEmoji, anytypeFieldIconName,
		anytypeFieldKey, anytypeFieldFormat, anytypeFieldFeatured, anytypeFieldOrder, anytypeFieldHidden,
		anytypeFieldSchemaVersion, anytypeFieldApp,
	}
	for _, known := range knownExtensions {
		if key == known {
			return true
		}
	}
	return false
}

// addSystemPropertiesToType adds system properties to the type's hidden relations if not already present
func (p *JSONSchemaParser) addSystemPropertiesToType(t *Type) {
	// Create a map of existing relations for quick lookup
	existingRelations := make(map[string]bool)
	for _, rel := range t.FeaturedRelations {
		existingRelations[rel] = true
	}
	for _, rel := range t.RecommendedRelations {
		existingRelations[rel] = true
	}
	for _, rel := range t.HiddenRelations {
		existingRelations[rel] = true
	}

	// Add system properties if not already present
	for _, sysPropKey := range SystemProperties {
		if !existingRelations[sysPropKey] {
			t.HiddenRelations = append(t.HiddenRelations, sysPropKey)
		}
	}
}

// checkSchemaVersion validates that the schema version is compatible
func (p *JSONSchemaParser) checkSchemaVersion(schemaVersion string) error {
	// Parse the schema version
	schemaVer, err := ParseVersion(schemaVersion)
	if err != nil {
		return fmt.Errorf("invalid schema version format: %w", err)
	}

	// Parse the current version
	currentVer, err := ParseVersion(VersionCurrent)
	if err != nil {
		// This should never happen with a valid SchemaVersion constant
		return fmt.Errorf("invalid current version: %w", err)
	}

	// Check if the schema major version is greater than current
	if schemaVer.Major > currentVer.Major {
		return fmt.Errorf("schema version %s is not compatible with current version %s: major version is too new", schemaVersion, VersionCurrent)
	}

	return nil
}

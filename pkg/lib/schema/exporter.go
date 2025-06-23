package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// SchemaVersion is the current schema generation version
const SchemaVersion = "1.0"

// orderedRelation represents a relation with its order and flags
type orderedRelation struct {
	name     string
	relation *Relation
	order    int
	featured bool
	hidden   bool // Whether this relation is hidden
}

// JSONSchemaExporter exports Schema to JSON Schema format
type JSONSchemaExporter struct {
	Indent string // Indentation for pretty printing (empty for compact)
}

// NewJSONSchemaExporter creates a new JSON Schema exporter
func NewJSONSchemaExporter(indent string) Exporter {
	return &JSONSchemaExporter{Indent: indent}
}

// Export writes a type from Schema as JSON Schema
func (e *JSONSchemaExporter) Export(schema *Schema, writer io.Writer) error {
	if schema.Type == nil {
		return fmt.Errorf("no type in schema")
	}

	jsonSchema := e.typeToJSONSchema(schema.Type, schema)

	// Use custom marshaler for ordered output
	orderedSchema := orderedJSONSchema{data: jsonSchema}

	var output []byte
	var err error

	if e.Indent != "" {
		// Pretty print with indentation
		output, err = json.MarshalIndent(orderedSchema, "", e.Indent)
	} else {
		output, err = json.Marshal(orderedSchema)
	}

	if err != nil {
		return err
	}

	_, err = writer.Write(output)
	if err != nil {
		return err
	}

	// Add trailing newline
	_, err = writer.Write([]byte("\n"))
	return err
}

// ExportType exports a single type as JSON Schema
func (e *JSONSchemaExporter) ExportType(t *Type, schema *Schema, writer io.Writer) error {
	jsonSchema := e.typeToJSONSchema(t, schema)

	// Use custom marshaler for ordered output
	orderedSchema := orderedJSONSchema{data: jsonSchema}

	var output []byte
	var err error

	if e.Indent != "" {
		// Pretty print with indentation
		output, err = json.MarshalIndent(orderedSchema, "", e.Indent)
	} else {
		output, err = json.Marshal(orderedSchema)
	}

	if err != nil {
		return err
	}

	_, err = writer.Write(output)
	if err != nil {
		return err
	}

	// Add trailing newline
	_, err = writer.Write([]byte("\n"))
	return err
}

// orderedJSONSchema is a wrapper that implements custom JSON marshaling with property ordering
type orderedJSONSchema struct {
	data map[string]interface{}
}

// MarshalJSON implements custom JSON marshaling that preserves property order
func (o orderedJSONSchema) MarshalJSON() ([]byte, error) {
	// Create ordered list of keys
	keys := make([]string, 0, len(o.data))
	for k := range o.data {
		keys = append(keys, k)
	}

	// Sort keys in desired order:
	// 1. Standard JSON Schema keys first ($schema, $id, type, title, description)
	// 2. x-* extension keys
	// 3. properties (which will be handled specially)
	// 4. Everything else alphabetically
	sort.Slice(keys, func(i, j int) bool {
		// Define priority order for standard keys
		priority := map[string]int{
			"$schema":     1,
			"$id":         2,
			"type":        3,
			"title":       4,
			"description": 5,
		}

		iPriority, iHasPriority := priority[keys[i]]
		jPriority, jHasPriority := priority[keys[j]]

		if iHasPriority && jHasPriority {
			return iPriority < jPriority
		}
		if iHasPriority {
			return true
		}
		if jHasPriority {
			return false
		}

		// x-* keys come after standard keys but before others
		iIsExtension := strings.HasPrefix(keys[i], "x-")
		jIsExtension := strings.HasPrefix(keys[j], "x-")

		if iIsExtension && !jIsExtension {
			return true
		}
		if !iIsExtension && jIsExtension {
			return false
		}

		// Properties comes last
		if keys[i] == "properties" {
			return false
		}
		if keys[j] == "properties" {
			return true
		}

		// Otherwise alphabetical
		return keys[i] < keys[j]
	})

	// Build ordered JSON manually
	var buf bytes.Buffer
	buf.WriteString("{")

	for idx, key := range keys {
		if idx > 0 {
			buf.WriteString(",")
		}

		// Marshal key
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal key %s: %w", key, err)
		}
		buf.Write(keyJSON)
		buf.WriteString(":")

		// Special handling for properties to maintain order
		if key == "properties" && o.data[key] != nil {
			if props, ok := o.data[key].(map[string]interface{}); ok {
				propertiesJSON := marshalOrderedProperties(props)
				buf.Write(propertiesJSON)
			} else {
				// Fallback to regular marshaling
				valueJSON, err := json.Marshal(o.data[key])
				if err != nil {
					return nil, fmt.Errorf("failed to marshal properties: %w", err)
				}
				buf.Write(valueJSON)
			}
		} else {
			// Regular marshaling for other fields
			valueJSON, err := json.Marshal(o.data[key])
			if err != nil {
				return nil, fmt.Errorf("failed to marshal field %s: %w", key, err)
			}
			buf.Write(valueJSON)
		}
	}

	buf.WriteString("}")
	return buf.Bytes(), nil
}

// marshalOrderedProperties marshals properties map ordered by x-order field
func marshalOrderedProperties(props map[string]interface{}) []byte {
	type propWithOrder struct {
		name  string
		order int
		data  interface{}
	}

	// Extract properties with their order
	orderedProps := make([]propWithOrder, 0, len(props))
	for name, prop := range props {
		order := 999 // Default high order for properties without x-order
		if propMap, ok := prop.(map[string]interface{}); ok {
			if xOrder, ok := propMap["x-order"].(int); ok {
				order = xOrder
			}
		}
		orderedProps = append(orderedProps, propWithOrder{
			name:  name,
			order: order,
			data:  prop,
		})
	}

	// Sort by order, then by name
	sort.Slice(orderedProps, func(i, j int) bool {
		if orderedProps[i].order != orderedProps[j].order {
			return orderedProps[i].order < orderedProps[j].order
		}
		return orderedProps[i].name < orderedProps[j].name
	})

	// Build ordered properties JSON
	var buf bytes.Buffer
	buf.WriteString("{")

	for idx, prop := range orderedProps {
		if idx > 0 {
			buf.WriteString(",")
		}

		// Marshal property name
		nameJSON, err := json.Marshal(prop.name)
		if err != nil {
			// Skip this property if we can't marshal its name
			continue
		}
		buf.Write(nameJSON)
		buf.WriteString(":")

		// Marshal property data
		propJSON, err := json.Marshal(prop.data)
		if err != nil {
			// Write null if we can't marshal the property data
			buf.WriteString("null")
		} else {
			buf.Write(propJSON)
		}
	}

	buf.WriteString("}")
	return buf.Bytes()
}

// typeToJSONSchema converts a Type to JSON Schema format
func (e *JSONSchemaExporter) typeToJSONSchema(t *Type, schema *Schema) map[string]interface{} {
	// Generate schema ID with current version
	schemaId := fmt.Sprintf("urn:anytype:schema:%s:type-%s:gen-%s",
		time.Now().UTC().Format("2006-01-02"),
		strings.ToLower(strings.ReplaceAll(t.Name, " ", "-")),
		SchemaVersion)

	jsonSchema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id":     schemaId,
		"type":    "object",
		"title":   t.Name,
	}

	// Add description if present
	if t.Description != "" {
		jsonSchema["description"] = t.Description
	}

	// Add Anytype extensions
	jsonSchema["x-type-key"] = t.Key
	jsonSchema["x-app"] = "Anytype"
	jsonSchema["x-genversion"] = SchemaVersion

	if t.PluralName != "" {
		jsonSchema["x-plural"] = t.PluralName
	}

	if t.IconEmoji != "" {
		jsonSchema["x-icon-emoji"] = t.IconEmoji
	}

	if t.IconName != "" {
		jsonSchema["x-icon-name"] = t.IconName
	}

	// Add other extensions (but skip internal fields like "id")
	for key, value := range t.Extension {
		if strings.HasPrefix(key, "x-") { // Only export x-* extensions to schema
			jsonSchema[key] = value
		}
	}

	// Build properties
	properties := make(map[string]interface{})

	// Always add id property first
	properties["id"] = map[string]interface{}{
		"type":        "string",
		"description": "Unique identifier of the Anytype object",
		"readOnly":    true,
		"x-order":     0,
		"x-key":       "id",
		"x-hidden":    true, // Always hidden in JSON Schema
	}

	// Collect all relations and sort by order
	var orderedRels []orderedRelation
	propertyOrder := 1 // Start after id

	hasType := false // Track if we have a key relation
	// Process featured relations first
	for _, relKey := range t.FeaturedRelations {
		if rel, ok := schema.GetRelation(relKey); ok {
			name := rel.Name
			if relKey == bundle.RelationKeyType.String() {
				hasType = true // Track if Type relation is present
			}
			orderedRels = append(orderedRels, orderedRelation{
				name:     name,
				relation: rel,
				order:    propertyOrder,
				featured: true,
			})
			propertyOrder++
		}
	}

	// Then regular relations
	for _, relKey := range t.RecommendedRelations {
		if rel, ok := schema.GetRelation(relKey); ok {
			name := rel.Name
			if relKey == bundle.RelationKeyType.String() {
				hasType = true // Track if Type relation is present
			}
			orderedRels = append(orderedRels, orderedRelation{
				name:     name,
				relation: rel,
				order:    propertyOrder,
				featured: false,
			})
			propertyOrder++
		}
	}

	for _, relKey := range t.HiddenRelations {
		if rel, ok := schema.GetRelation(relKey); ok {
			name := rel.Name
			if relKey == bundle.RelationKeyType.String() {
				hasType = true // Track if Type relation is present
			}
			orderedRels = append(orderedRels, orderedRelation{
				name:     name,
				relation: rel,
				order:    propertyOrder,
				hidden:   true,
			})
			propertyOrder++
		}
	}
	if !hasType {
		typeRel := bundle.MustGetRelation(bundle.RelationKeyType)
		// If Type relation is missing, add it as a hidden property
		orderedRels = append(orderedRels, orderedRelation{
			name:     typeRel.Name,
			relation: &Relation{Key: bundle.RelationKeyType.String(), Name: typeRel.Name, Format: typeRel.Format},
			order:    propertyOrder,
		})
		propertyOrder++
	}

	// Deduplicate names and convert relations to properties
	deduplicatedNames := e.deduplicatePropertyNames(orderedRels)
	for i, or := range orderedRels {
		prop := e.relationToProperty(or.relation)
		prop["x-order"] = or.order
		if or.featured {
			prop["x-featured"] = true
		} else if or.hidden {
			prop["x-hidden"] = true
		}

		// Special handling for Type relation
		if or.relation.Key == bundle.RelationKeyType.String() {
			// Override to be a simple string with const value
			prop["type"] = "string"
			prop["const"] = t.Name
			delete(prop, "items") // Remove items if it was set for object format
		}

		properties[deduplicatedNames[i]] = prop
	}

	// Check if this is a collection type
	if t.Layout == model.ObjectType_collection {
		properties["Collection"] = map[string]interface{}{
			"type":        "array",
			"description": "List of object file paths or names in this collection",
			"items": map[string]interface{}{
				"type": "string",
			},
			"x-order": propertyOrder,
		}
	}

	jsonSchema["properties"] = properties

	return jsonSchema
}

// relationToProperty converts a Relation to JSON Schema property
func (e *JSONSchemaExporter) relationToProperty(r *Relation) map[string]interface{} {
	prop := make(map[string]interface{})

	// Always add x-key and x-format
	prop["x-key"] = r.Key
	prop["x-format"] = r.Format.String()

	// Add description if present
	if r.Description != "" {
		prop["description"] = r.Description
	}

	// Set read-only if applicable
	if r.IsReadOnly {
		prop["readOnly"] = true
	}

	if r.Key == bundle.RelationKeyType.String() {
		// Type relation should have the type name as const
		// This will be set by the caller who has access to the type name
		return prop
	}

	// Handle different formats
	switch r.Format {
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		prop["type"] = "string"
		if r.MaxLength > 0 {
			prop["maxLength"] = r.MaxLength
		}

	case model.RelationFormat_number:
		prop["type"] = "number"

	case model.RelationFormat_checkbox:
		prop["type"] = "boolean"

	case model.RelationFormat_date:
		prop["type"] = "string"
		if r.IncludeTime {
			prop["format"] = "date-time"
		} else {
			prop["format"] = "date"
		}

	case model.RelationFormat_tag:
		prop["type"] = "array"
		prop["items"] = map[string]interface{}{"type": "string"}
		if len(r.Examples) > 0 {
			prop["examples"] = r.Examples
		}

	case model.RelationFormat_status:
		prop["type"] = "string"
		if len(r.Options) > 0 {
			prop["enum"] = r.Options
		}

	case model.RelationFormat_email:
		prop["type"] = "string"
		prop["format"] = "email"

	case model.RelationFormat_url:
		prop["type"] = "string"
		prop["format"] = "uri"

	case model.RelationFormat_phone:
		prop["type"] = "string"
		prop["pattern"] = "^[+]?[0-9\\s()-]+$"

	case model.RelationFormat_file:
		if r.IsMulti {
			prop["type"] = "array"
			prop["items"] = map[string]interface{}{"type": "string"}
		} else {
			prop["type"] = "string"
		}
		prop["description"] = "Path to the file in the export"

	case model.RelationFormat_object:
		prop["type"] = "array"
		prop["items"] = map[string]interface{}{
			"type": "string",
		}

		// Add x-object-types if specified
		if len(r.ObjectTypes) > 0 {
			prop["x-object-types"] = r.ObjectTypes
		}

	default:
		prop["type"] = "string"
	}

	// Add other extensions (but skip internal fields like "id")
	for key, value := range r.Extension {
		if strings.HasPrefix(key, "x-") { // Don't export internal ID to schema
			prop[key] = value
		}
	}

	return prop
}

// deduplicatePropertyNames ensures no duplicate property names in export
// by sorting relations by key and adding index suffixes when needed
// Special handling: Bundled relations always keep their names without suffix
func (e *JSONSchemaExporter) deduplicatePropertyNames(orderedRels []orderedRelation) []string {
	// Create a map to track names and their relation keys
	nameToRelations := make(map[string][]struct {
		index int
		key   string
	})

	// Group relations by name
	for i, or := range orderedRels {
		nameToRelations[or.name] = append(nameToRelations[or.name], struct {
			index int
			key   string
		}{i, or.relation.Key})
	}

	result := make([]string, len(orderedRels))

	// Process each name group
	for name, relations := range nameToRelations {
		if len(relations) == 1 {
			// No duplication, use original name
			result[relations[0].index] = name
		} else {
			// Sort by relation key, but give priority to bundled relations
			sort.Slice(relations, func(i, j int) bool {
				// Bundled relations come first
				iIsBundled := bundle.HasRelation(domain.RelationKey(relations[i].key))
				jIsBundled := bundle.HasRelation(domain.RelationKey(relations[j].key))

				if iIsBundled && !jIsBundled {
					return true
				}
				if !iIsBundled && jIsBundled {
					return false
				}

				// Otherwise sort by key
				return relations[i].key < relations[j].key
			})

			// Add index suffix to duplicated names
			for idx, rel := range relations {
				if idx == 0 {
					// First occurrence keeps original name
					result[rel.index] = name
				} else {
					// Subsequent occurrences get index suffix
					result[rel.index] = fmt.Sprintf("%s %d", name, idx+1)
				}
			}
		}
	}

	return result
}

// ObjectResolver interface for resolving relations and their options
type ObjectResolver interface {
	ResolveRelation(relationId string) (*domain.Details, error)
	ResolveRelationOptions(relationKey string) ([]*domain.Details, error)
}

// SchemaFromObjectDetails creates a Schema from object type and relation details
func SchemaFromObjectDetails(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver func(string) (*domain.Details, error)) (*Schema, error) {
	return schemaFromObjectDetailsInternal(typeDetails, relationDetailsList, resolver, nil)
}

// SchemaFromObjectDetailsWithResolver creates a Schema from object type and relation details with full resolver
func SchemaFromObjectDetailsWithResolver(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver ObjectResolver) (*Schema, error) {
	return schemaFromObjectDetailsInternal(typeDetails, relationDetailsList, resolver.ResolveRelation, resolver)
}

func schemaFromObjectDetailsInternal(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver func(string) (*domain.Details, error), optionResolver ObjectResolver) (*Schema, error) {
	schema := NewSchema()

	// Create type from details
	t, err := TypeFromDetails(typeDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to create type from details: %w", err)
	}

	// Create a map for quick lookup
	featuredSet := make(map[string]bool)
	for _, id := range t.FeaturedRelations {
		featuredSet[id] = true
	}

	// Create a map for quick lookup
	hiddenSet := make(map[string]bool)
	for _, id := range t.HiddenRelations {
		hiddenSet[id] = true
	}

	// Sort relations by their position in the type's lists
	type orderedRelation struct {
		id       string
		details  *domain.Details
		order    int
		featured bool
		hidden   bool
	}

	var orderedRelations []orderedRelation

	// Add featured relations first
	for i, id := range t.FeaturedRelations {
		// Find in provided list first
		var found *domain.Details
		for _, rd := range relationDetailsList {
			if rd.GetString(bundle.RelationKeyId) == id {
				found = rd
				break
			}
		}

		// If not found and resolver provided, try to resolve
		if found == nil && resolver != nil {
			found, _ = resolver(id) // Ignore error as it's optional
		}

		if found != nil {
			orderedRelations = append(orderedRelations, orderedRelation{
				id:       id,
				details:  found,
				order:    i,
				featured: true,
			})
		}
	}

	// Add recommended relations
	for i, id := range t.RecommendedRelations {
		// Find in provided list first
		var found *domain.Details
		for _, rd := range relationDetailsList {
			if rd.GetString(bundle.RelationKeyId) == id {
				found = rd
				break
			}
		}

		// If not found and resolver provided, try to resolve
		if found == nil && resolver != nil {
			found, _ = resolver(id) // Ignore error as it's optional
		}

		if found != nil {
			orderedRelations = append(orderedRelations, orderedRelation{
				id:       id,
				details:  found,
				order:    len(t.FeaturedRelations) + i,
				featured: false,
			})
		}
	}

	// Add hidden relations
	for i, id := range t.HiddenRelations {
		// Find in provided list first
		var found *domain.Details
		for _, rd := range relationDetailsList {
			if rd.GetString(bundle.RelationKeyId) == id {
				found = rd
				break
			}
		}

		// If not found and resolver provided, try to resolve
		if found == nil && resolver != nil {
			found, _ = resolver(id) // Ignore error as it's optional
		}

		if found != nil {
			orderedRelations = append(orderedRelations, orderedRelation{
				id:       id,
				details:  found,
				order:    len(t.FeaturedRelations) + len(t.RecommendedRelations) + i,
				featured: false,
				hidden:   true,
			})
		}
	}

	// Sort by order
	sort.Slice(orderedRelations, func(i, j int) bool {
		return orderedRelations[i].order < orderedRelations[j].order
	})

	// Process relations in order
	for _, or := range orderedRelations {
		rel, err := RelationFromDetails(or.details)
		if err != nil {
			continue
		}

		// Populate relation options for status/tag relations if resolver is available
		if optionResolver != nil && (rel.Format == model.RelationFormat_status || rel.Format == model.RelationFormat_tag) {
			if optionDetails, err := optionResolver.ResolveRelationOptions(rel.Key); err == nil && optionDetails != nil {
				var options []string
				for _, details := range optionDetails {
					if optionName := details.GetString(bundle.RelationKeyName); optionName != "" {
						options = append(options, optionName)
					}
				}
				if rel.Format == model.RelationFormat_status {
					rel.Options = options
				} else if rel.Format == model.RelationFormat_tag {
					rel.Examples = options
				}
			}
		}

		// Add to schema
		if err := schema.AddRelation(rel); err != nil {
			// Log error but continue processing other relations
			continue
		}

		// Add to type
		t.AddRelation(rel.Key, or.featured, or.hidden)
	}

	// Set type for schema
	if err := schema.SetType(t); err != nil {
		return nil, fmt.Errorf("failed to set type: %w", err)
	}

	return schema, nil
}

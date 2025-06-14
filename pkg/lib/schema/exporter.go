package schema

import (
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
	
	encoder := json.NewEncoder(writer)
	if e.Indent != "" {
		encoder.SetIndent("", e.Indent)
	}
	
	return encoder.Encode(jsonSchema)
}

// ExportType exports a single type as JSON Schema
func (e *JSONSchemaExporter) ExportType(t *Type, schema *Schema, writer io.Writer) error {
	jsonSchema := e.typeToJSONSchema(t, schema)
	
	encoder := json.NewEncoder(writer)
	if e.Indent != "" {
		encoder.SetIndent("", e.Indent)
	}
	
	return encoder.Encode(jsonSchema)
}

// typeToJSONSchema converts a Type to JSON Schema format
func (e *JSONSchemaExporter) typeToJSONSchema(t *Type, schema *Schema) map[string]interface{} {
	// Generate schema ID
	schemaId := fmt.Sprintf("urn:anytype:schema:%s:type-%s:gen-%s",
		time.Now().UTC().Format("2006-01-02"),
		strings.ToLower(strings.ReplaceAll(t.Name, " ", "-")),
		"1.0.0")
	
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
	jsonSchema["x-genVersion"] = "1.0.0"
	
	if t.PluralName != "" {
		jsonSchema["x-plural"] = t.PluralName
	}
	
	if t.IconEmoji != "" {
		jsonSchema["x-icon-emoji"] = t.IconEmoji
	}
	
	if t.IconImage != "" {
		jsonSchema["x-icon-name"] = t.IconImage
	}
	
	// Add other extensions
	for key, value := range t.Extension {
		jsonSchema[key] = value
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
	}
	
	// Add Type property
	properties["Type"] = map[string]interface{}{
		"const":   t.Name,
		"x-order": 1,
		"x-key":   bundle.RelationKeyType.String(),
	}
	
	// Collect all relations and sort by order
	type orderedRelation struct {
		name     string
		relation *Relation
		order    int
		featured bool
	}
	
	var orderedRels []orderedRelation
	propertyOrder := 2 // Start after id and Type
	
	// Process featured relations first
	for _, relKey := range t.FeaturedRelations {
		if rel, ok := schema.GetRelation(relKey); ok {
			orderedRels = append(orderedRels, orderedRelation{
				name:     rel.Name,
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
			orderedRels = append(orderedRels, orderedRelation{
				name:     rel.Name,
				relation: rel,
				order:    propertyOrder,
				featured: false,
			})
			propertyOrder++
		}
	}
	
	// Convert relations to properties
	for _, or := range orderedRels {
		prop := e.relationToProperty(or.relation)
		prop["x-order"] = or.order
		if or.featured {
			prop["x-featured"] = true
		}
		properties[or.name] = prop
	}
	
	// Check if this is a collection type
	if t.Layout == model.ObjectType_collection {
		properties["Collection"] = map[string]interface{}{
			"type":        "array",
			"description": "List of objects in this collection",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"Name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the object in the collection",
					},
					"File": map[string]interface{}{
						"type":        "string",
						"description": "Path to the object file (only present if object is included in export)",
					},
					"Id": map[string]interface{}{
						"type":        "string",
						"description": "Unique identifier of the object (only present if object is not included in export)",
					},
				},
				"required": []string{"Name"},
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
	prop["x-format"] = "RelationFormat_" + r.Format.String()
	
	// Add description if present
	if r.Description != "" {
		prop["description"] = r.Description
	}
	
	// Set read-only if applicable
	if r.IsReadOnly {
		prop["readOnly"] = true
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
		objectSchema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"Name": map[string]interface{}{
					"type":        "string",
					"description": "Name of the referenced object",
				},
				"File": map[string]interface{}{
					"type":        "string",
					"description": "Path to the object file in the export (only present if object is included in export)",
				},
				"Id": map[string]interface{}{
					"type":        "string",
					"description": "Unique identifier of the referenced object (only present if object is not included in export)",
				},
				"Object type": map[string]interface{}{
					"type":        "string",
					"description": "Type of the referenced object",
				},
			},
			"required": []string{"Name"},
		}
		
		// Add enum for object types if specified
		if len(r.ObjectTypes) > 0 {
			objectSchema["properties"].(map[string]interface{})["Object type"].(map[string]interface{})["enum"] = r.ObjectTypes
		}
		
		prop["items"] = objectSchema
		
	default:
		prop["type"] = "string"
	}
	
	// Add other extensions
	for key, value := range r.Extension {
		prop[key] = value
	}
	
	return prop
}

// SchemaFromObjectDetails creates a Schema from object type and relation details
func SchemaFromObjectDetails(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver func(string) (*domain.Details, error)) (*Schema, error) {
	schema := NewSchema()
	
	// Create type from details
	t, err := TypeFromDetails(typeDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to create type from details: %w", err)
	}
	
	// Clear relations - we'll rebuild from the relation details
	t.FeaturedRelations = nil
	t.RecommendedRelations = nil
	
	// Get featured and recommended relation IDs from type
	featuredIds := typeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	recommendedIds := typeDetails.GetStringList(bundle.RelationKeyRecommendedRelations)
	
	// Create a map for quick lookup
	featuredSet := make(map[string]bool)
	for _, id := range featuredIds {
		featuredSet[id] = true
	}
	
	// Sort relations by their position in the type's lists
	type orderedRelation struct {
		id       string
		details  *domain.Details
		order    int
		featured bool
	}
	
	var orderedRelations []orderedRelation
	
	// Add featured relations first
	for i, id := range featuredIds {
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
			found, _ = resolver(id)
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
	for i, id := range recommendedIds {
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
			found, _ = resolver(id)
		}
		
		if found != nil {
			orderedRelations = append(orderedRelations, orderedRelation{
				id:       id,
				details:  found,
				order:    len(featuredIds) + i,
				featured: false,
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
		
		// Skip bundled relations
		if rel.IsBundled() && rel.Key != bundle.RelationKeyType.String() {
			continue
		}
		
		// Add to schema
		schema.AddRelation(rel)
		
		// Add to type
		t.AddRelation(rel.Key, or.featured)
	}
	
	// Set type for schema
	schema.SetType(t)
	
	return schema, nil
}
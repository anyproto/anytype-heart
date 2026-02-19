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

// SystemProperties are properties that are always included in export if not empty
var SystemProperties = []string{
	bundle.RelationKeyType.String(),
	bundle.RelationKeyCreator.String(),
	bundle.RelationKeyCreatedDate.String(),
	bundle.RelationKeyIconEmoji.String(),
	bundle.RelationKeyIconImage.String(),
	bundle.RelationKeyCoverId.String(),
}

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
			jsonSchemaFieldSchema:      1,
			jsonSchemaFieldID:          2,
			jsonSchemaFieldType:        3,
			jsonSchemaFieldTitle:       4,
			jsonSchemaFieldDescription: 5,
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
		iIsExtension := strings.HasPrefix(keys[i], extensionPrefix)
		jIsExtension := strings.HasPrefix(keys[j], extensionPrefix)

		if iIsExtension && !jIsExtension {
			return true
		}
		if !iIsExtension && jIsExtension {
			return false
		}

		// Properties comes last
		if keys[i] == jsonSchemaFieldProperties {
			return false
		}
		if keys[j] == jsonSchemaFieldProperties {
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
		if key == jsonSchemaFieldProperties && o.data[key] != nil {
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
			if xOrder, ok := propMap[anytypeFieldOrder].(int); ok {
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
	schemaId := fmt.Sprintf("urn:anytype:schema:%s:type-%s:ver-%s",
		time.Now().UTC().Format("2006-01-02"),
		strings.ToLower(strings.ReplaceAll(t.Key, " ", "-")),
		VersionCurrent)

	jsonSchema := map[string]interface{}{
		jsonSchemaFieldSchema: jsonSchemaVersion,
		jsonSchemaFieldID:     schemaId,
		jsonSchemaFieldType:   jsonSchemaTypeObject,
		jsonSchemaFieldTitle:  t.Name,
	}

	// Add description if present
	if t.Description != "" {
		jsonSchema[jsonSchemaFieldDescription] = t.Description
	}

	// Add Anytype extensions
	jsonSchema[anytypeFieldTypeKey] = t.Key
	jsonSchema[anytypeFieldApp] = anytypeAppName
	jsonSchema[anytypeFieldSchemaVersion] = VersionCurrent

	if t.PluralName != "" {
		jsonSchema[anytypeFieldPlural] = t.PluralName
	}

	if t.IconEmoji != "" {
		jsonSchema[anytypeFieldIconEmoji] = t.IconEmoji
	}

	if t.IconName != "" {
		jsonSchema[anytypeFieldIconName] = t.IconName
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
	properties[propertyNameID] = map[string]interface{}{
		jsonSchemaFieldType:        jsonSchemaTypeString,
		jsonSchemaFieldDescription: "Unique identifier of the Anytype object",
		jsonSchemaFieldReadOnly:    true,
		anytypeFieldOrder:          0,
		anytypeFieldKey:            propertyNameID,
		anytypeFieldHidden:         true, // Always hidden in JSON Schema
	}

	// Collect all relations and sort by order
	orderedRels, propertyOrder, hasType := e.collectOrderedRelations(t, schema)
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
		prop[anytypeFieldOrder] = or.order
		if or.featured {
			prop[anytypeFieldFeatured] = true
		} else if or.hidden {
			prop[anytypeFieldHidden] = true
		}

		// Special handling for Type relation
		if or.relation.Key == bundle.RelationKeyType.String() {
			// Override to be a simple string with const value
			prop[jsonSchemaFieldType] = jsonSchemaTypeString
			prop[jsonSchemaFieldConst] = t.Name
			delete(prop, jsonSchemaFieldItems) // Remove items if it was set for object format
		}

		properties[deduplicatedNames[i]] = prop
	}

	// Check if this is a collection type
	if t.Layout == model.ObjectType_collection {
		properties[propertyNameCollection] = map[string]interface{}{
			jsonSchemaFieldType:        jsonSchemaTypeArray,
			jsonSchemaFieldDescription: "List of object file paths or names in this collection",
			jsonSchemaFieldItems: map[string]interface{}{
				jsonSchemaFieldType: jsonSchemaTypeString,
			},
			anytypeFieldOrder:  propertyOrder,
			anytypeFieldKey:    CollectionPropertyKey,
			anytypeFieldFormat: "object",
		}
	}

	jsonSchema[jsonSchemaFieldProperties] = properties

	return jsonSchema
}

// relationToProperty converts a Relation to JSON Schema property
func (e *JSONSchemaExporter) relationToProperty(r *Relation) map[string]interface{} {
	prop := make(map[string]interface{})

	// Always add x-key and x-format
	prop[anytypeFieldKey] = r.Key
	prop[anytypeFieldFormat] = r.Format.String()

	// Add description if present
	if r.Description != "" {
		prop[jsonSchemaFieldDescription] = r.Description
	}

	// Set read-only if applicable
	if r.IsReadOnly {
		prop[jsonSchemaFieldReadOnly] = true
	}

	if r.Key == bundle.RelationKeyType.String() {
		// Type relation should have the type name as const
		// This will be set by the caller who has access to the type name
		return prop
	}

	// Handle different formats
	switch r.Format {
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		if r.MaxLength > 0 {
			prop[jsonSchemaFieldMaxLength] = r.MaxLength
		}

	case model.RelationFormat_number:
		prop[jsonSchemaFieldType] = jsonSchemaTypeNumber

	case model.RelationFormat_checkbox:
		prop[jsonSchemaFieldType] = jsonSchemaTypeBoolean

	case model.RelationFormat_date:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		if r.IncludeTime {
			prop[jsonSchemaFieldFormat] = jsonSchemaFormatDateTime
		} else {
			prop[jsonSchemaFieldFormat] = jsonSchemaFormatDate
		}

	case model.RelationFormat_tag:
		prop[jsonSchemaFieldType] = jsonSchemaTypeArray
		prop[jsonSchemaFieldItems] = map[string]interface{}{jsonSchemaFieldType: jsonSchemaTypeString}
		if len(r.Examples) > 0 {
			prop[jsonSchemaFieldExamples] = r.Examples
		}

	case model.RelationFormat_status:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		if len(r.Options) > 0 {
			prop[jsonSchemaFieldEnum] = r.Options
		}

	case model.RelationFormat_email:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		prop[jsonSchemaFieldFormat] = jsonSchemaFormatEmail

	case model.RelationFormat_url:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		prop[jsonSchemaFieldFormat] = jsonSchemaFormatURI

	case model.RelationFormat_phone:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
		prop[jsonSchemaFieldPattern] = phoneNumberPattern

	case model.RelationFormat_file:
		if r.IsMulti {
			prop[jsonSchemaFieldType] = jsonSchemaTypeArray
			prop[jsonSchemaFieldItems] = map[string]interface{}{jsonSchemaFieldType: jsonSchemaTypeString}
		} else {
			prop[jsonSchemaFieldType] = jsonSchemaTypeString
		}
		prop[jsonSchemaFieldDescription] = "Path to the file in the export"

	case model.RelationFormat_object:
		prop[jsonSchemaFieldType] = jsonSchemaTypeArray
		prop[jsonSchemaFieldItems] = map[string]interface{}{
			jsonSchemaFieldType: jsonSchemaTypeString,
		}

		// Add x-object-types if specified
		if len(r.ObjectTypes) > 0 {
			prop[anytypeFieldObjectTypes] = r.ObjectTypes
		}

	default:
		prop[jsonSchemaFieldType] = jsonSchemaTypeString
	}

	// Add other extensions (but skip internal fields like "id")
	for key, value := range r.Extension {
		if strings.HasPrefix(key, extensionPrefix) { // Don't export internal ID to schema
			prop[key] = value
		}
	}

	return prop
}

// deduplicatePropertyNames ensures no duplicate property names in export
// by sorting relations by key and adding index suffixes when needed
// Special handling: Bundled relations always keep their names without suffix
// collectOrderedRelations collects and orders all relations from the type
func (e *JSONSchemaExporter) collectOrderedRelations(t *Type, schema *Schema) ([]orderedRelation, int, bool) {
	var orderedRels []orderedRelation
	propertyOrder := 1 // Start after id
	hasType := false   // Track if we have a type relation
	existingKeys := make(map[string]struct{})

	// Define relation categories with their properties
	relationCategories := []struct {
		relations []string
		featured  bool
		hidden    bool
	}{
		{relations: t.FeaturedRelations, featured: true, hidden: false},
		{relations: t.RecommendedRelations, featured: false, hidden: false},
		{relations: t.HiddenRelations, featured: false, hidden: true},
	}

	// Process each category
	for _, category := range relationCategories {
		for _, relKey := range category.relations {
			if _, exists := existingKeys[relKey]; exists {
				// Skip if this relation is already added
				continue
			}
			if rel, ok := schema.GetRelation(relKey); ok {
				if relKey == bundle.RelationKeyType.String() {
					hasType = true
				}
				orderedRels = append(orderedRels, orderedRelation{
					name:     rel.Name,
					relation: rel,
					order:    propertyOrder,
					featured: category.featured,
					hidden:   category.hidden,
				})
				propertyOrder++
				existingKeys[relKey] = struct{}{}
			}
		}
	}

	return orderedRels, propertyOrder, hasType
}

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
	// RelationById gets relation details by its ID
	RelationById(relationId string) (*domain.Details, error)
	// RelationByKey gets relation details by its key
	RelationByKey(relationKey string) (*domain.Details, error)
	// RelationOptions gets relation options for a given relation key
	RelationOptions(relationKey string) ([]*domain.Details, error)
}

// SchemaFromObjectDetails creates a Schema from object type and relation details
func SchemaFromObjectDetails(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver func(string) (*domain.Details, error)) (*Schema, error) {
	return schemaFromObjectDetailsInternal(typeDetails, relationDetailsList, resolver, nil)
}

// SchemaFromObjectDetailsWithResolver creates a Schema from object type and relation details with full resolver
func SchemaFromObjectDetailsWithResolver(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver ObjectResolver) (*Schema, error) {
	// For backward compatibility, wrap the old resolver function
	legacyResolver := func(id string) (*domain.Details, error) {
		return resolver.RelationById(id)
	}
	return schemaFromObjectDetailsInternal(typeDetails, relationDetailsList, legacyResolver, resolver)
}

func schemaFromObjectDetailsInternal(typeDetails *domain.Details, relationDetailsList []*domain.Details, resolver func(string) (*domain.Details, error), optionResolver ObjectResolver) (*Schema, error) {
	schema := NewSchema()

	// Create maps for quick lookup
	relationDetailsById := make(map[string]*domain.Details)
	relationDetailsByKey := make(map[string]*domain.Details)
	for _, rd := range relationDetailsList {
		if id := rd.GetString(bundle.RelationKeyId); id != "" {
			relationDetailsById[id] = rd
		}
		if key := rd.GetString(bundle.RelationKeyRelationKey); key != "" {
			relationDetailsByKey[key] = rd
		}
	}

	// Create ID to key resolver function that also adds resolved relations to our maps
	idToKeyResolver := func(relationId string) (string, error) {
		// Try to find in provided list
		if rd, ok := relationDetailsById[relationId]; ok {
			if key := rd.GetString(bundle.RelationKeyRelationKey); key != "" {
				return key, nil
			}
		}

		// Try resolver if available
		if resolver != nil {
			if details, err := resolver(relationId); err == nil && details != nil {
				if key := details.GetString(bundle.RelationKeyRelationKey); key != "" {
					// Add to our maps so we can find it later
					relationDetailsById[relationId] = details
					relationDetailsByKey[key] = details
					return key, nil
				}
			}
		}

		return "", fmt.Errorf("could not resolve relation ID %s to key", relationId)
	}

	// Create type from details with ID to key resolution
	t, err := TypeFromDetailsWithResolver(typeDetails, idToKeyResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to create type from details: %w", err)
	}

	// Helper function to find relation details by key
	findRelationDetailsByKey := func(relationKey string) *domain.Details {
		// First check provided list by key
		if details, ok := relationDetailsByKey[relationKey]; ok {
			return details
		}

		// For bundled relations, create details from bundle
		if bundledRel, err := bundle.GetRelation(domain.RelationKey(relationKey)); err == nil {
			details := domain.NewDetails()
			details.SetString(bundle.RelationKeyRelationKey, relationKey)
			details.SetString(bundle.RelationKeyName, bundledRel.Name)
			details.SetInt64(bundle.RelationKeyRelationFormat, int64(bundledRel.Format))
			details.SetBool(bundle.RelationKeyIsReadonly, bundledRel.ReadOnly)
			details.SetBool(bundle.RelationKeyIsHidden, bundledRel.Hidden)
			return details
		}

		return nil
	}

	// Collect all relations with their metadata
	type orderedRelation struct {
		key      string
		details  *domain.Details
		order    int
		featured bool
		hidden   bool
	}

	var orderedRelations []orderedRelation
	currentOrder := 0

	// Process relation categories in order (now using keys only)
	relationCategories := []struct {
		relationKeys []string
		featured     bool
		hidden       bool
	}{
		{relationKeys: t.FeaturedRelations, featured: true, hidden: false},
		{relationKeys: t.RecommendedRelations, featured: false, hidden: false},
		{relationKeys: t.HiddenRelations, featured: false, hidden: true},
		{relationKeys: SystemProperties, featured: false, hidden: true},
	}

	var keySet = make(map[string]struct{})
	for _, category := range relationCategories {
		for _, relationKey := range category.relationKeys {
			if _, exists := keySet[relationKey]; exists {
				continue
			}
			keySet[relationKey] = struct{}{}
			if details := findRelationDetailsByKey(relationKey); details != nil {
				orderedRelations = append(orderedRelations, orderedRelation{
					key:      relationKey,
					details:  details,
					order:    currentOrder,
					featured: category.featured,
					hidden:   category.hidden,
				})
				currentOrder++
			}
		}
	}

	// Sort by order (though it's already in order, this ensures consistency)
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
			if optionDetails, err := optionResolver.RelationOptions(rel.Key); err == nil && optionDetails != nil {
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

package markdown

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// SchemaInfo holds information about a parsed JSON schema
type SchemaInfo struct {
	TypeName       string
	TypeKey        string   // x-type-key from schema
	Properties     []string // Property keys in order
	Relations      map[string]*RelationInfo
	SchemaFileName string
}

// RelationInfo holds information about a relation from schema
type RelationInfo struct {
	Name        string
	Key         string // x-key from schema
	Format      model.RelationFormat
	Description string
	Featured    bool
	Order       int
	IncludeTime bool     // For date relations
	Options     []string // For status relations (enum values)
	Examples    []string // For tag relations (example values)
}

// SchemaImporter handles schema-based import
type SchemaImporter struct {
	schemas       map[string]*SchemaInfo  // typeName -> SchemaInfo
	relations     map[string]*RelationInfo // relationKey -> RelationInfo
	existingTypes map[string]string       // typeKey -> typeId
	existingRels  map[string]string       // relationKey -> relationId
}

// NewSchemaImporter creates a new schema importer
func NewSchemaImporter() *SchemaImporter {
	return &SchemaImporter{
		schemas:       make(map[string]*SchemaInfo),
		relations:     make(map[string]*RelationInfo),
		existingTypes: make(map[string]string),
		existingRels:  make(map[string]string),
	}
}

// LoadSchemas loads all schemas from the schemas folder in the import source
func (si *SchemaImporter) LoadSchemas(importSource source.Source, allErrors *common.ConvertError) error {
	schemasFound := false
	
	// Iterate through files looking for schemas folder
	err := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) bool {
		defer fileReader.Close()
		
		// Check if file is in schemas folder and is a JSON file
		if strings.HasPrefix(fileName, "schemas/") && strings.HasSuffix(fileName, ".schema.json") {
			schemasFound = true
			
			// Read schema file
			data, err := io.ReadAll(fileReader)
			if err != nil {
				allErrors.Add(fmt.Errorf("failed to read schema %s: %w", fileName, err))
				return true
			}
			
			// Parse schema
			if err := si.parseSchema(fileName, data); err != nil {
				allErrors.Add(fmt.Errorf("failed to parse schema %s: %w", fileName, err))
			}
		}
		
		return true
	})
	
	if err != nil {
		return err
	}
	
	if !schemasFound {
		// No schemas folder found, this is not an error
		return nil
	}
	
	log.Infof("Loaded %d schemas with %d relations", len(si.schemas), len(si.relations))
	return nil
}

// parseSchema parses a single JSON schema file
func (si *SchemaImporter) parseSchema(fileName string, data []byte) error {
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		return err
	}
	
	// Extract type information
	typeName, _ := schema["title"].(string)
	if typeName == "" {
		// Try to derive from filename
		base := filepath.Base(fileName)
		typeName = strings.TrimSuffix(base, ".schema.json")
		typeName = strings.ReplaceAll(typeName, "_", " ")
		typeName = strings.Title(typeName)
	}
	
	info := &SchemaInfo{
		TypeName:       typeName,
		TypeKey:        getStringField(schema, "x-type-key"),
		Relations:      make(map[string]*RelationInfo),
		SchemaFileName: fileName,
	}
	
	// Parse properties
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		// Sort properties by x-order
		type propWithOrder struct {
			name  string
			order int
			prop  map[string]interface{}
		}
		
		var sortedProps []propWithOrder
		for name, propData := range props {
			if prop, ok := propData.(map[string]interface{}); ok {
				order := 999 // Default high order for props without x-order
				if o, ok := prop["x-order"].(float64); ok {
					order = int(o)
				}
				sortedProps = append(sortedProps, propWithOrder{name, order, prop})
			}
		}
		
		// Sort by order
		for i := 0; i < len(sortedProps); i++ {
			for j := i + 1; j < len(sortedProps); j++ {
				if sortedProps[i].order > sortedProps[j].order {
					sortedProps[i], sortedProps[j] = sortedProps[j], sortedProps[i]
				}
			}
		}
		
		// Process sorted properties
		for _, sp := range sortedProps {
			if sp.name == "id" {
				// Skip id property as it's system-managed
				continue
			}
			
			relInfo := si.parseRelationFromProperty(sp.name, sp.prop)
			if relInfo != nil {
				relInfo.Order = sp.order
				
				// Store relation globally
				if relInfo.Key != "" {
					si.relations[relInfo.Key] = relInfo
				}
				
				// Add to type's relations
				info.Relations[relInfo.Key] = relInfo
				info.Properties = append(info.Properties, relInfo.Key)
			}
		}
	}
	
	// Store schema info
	si.schemas[typeName] = info
	
	return nil
}

// parseRelationFromProperty parses relation info from a schema property
func (si *SchemaImporter) parseRelationFromProperty(name string, prop map[string]interface{}) *RelationInfo {
	rel := &RelationInfo{
		Name:        name,
		Key:         getStringField(prop, "x-key"),
		Description: getStringField(prop, "description"),
		Featured:    getBoolField(prop, "x-featured"),
		Format:      model.RelationFormat_shorttext, // Default
	}
	
	// If no x-key, generate one
	if rel.Key == "" {
		rel.Key = bson.NewObjectId().Hex()
	}
	
	// Determine format from property schema
	propType := getStringField(prop, "type")
	format := getStringField(prop, "format")
	
	switch propType {
	case "boolean":
		rel.Format = model.RelationFormat_checkbox
		
	case "number":
		rel.Format = model.RelationFormat_number
		
	case "string":
		switch format {
		case "date":
			rel.Format = model.RelationFormat_date
			rel.IncludeTime = false
		case "date-time":
			rel.Format = model.RelationFormat_date
			rel.IncludeTime = true
		case "email":
			rel.Format = model.RelationFormat_email
		case "uri":
			rel.Format = model.RelationFormat_url
		default:
			// Check for enum (status)
			if enumValues, hasEnum := prop["enum"]; hasEnum {
				rel.Format = model.RelationFormat_status
				// Extract enum values as options
				if enumArray, ok := enumValues.([]interface{}); ok {
					for _, val := range enumArray {
						if strVal, ok := val.(string); ok {
							rel.Options = append(rel.Options, strVal)
						}
					}
				}
			} else if desc, _ := prop["description"].(string); strings.Contains(desc, "Long text") {
				rel.Format = model.RelationFormat_longtext
			} else if strings.Contains(desc, "Path to the file") {
				rel.Format = model.RelationFormat_file
			}
		}
		
	case "array":
		// Check items type
		if items, ok := prop["items"].(map[string]interface{}); ok {
			if itemType, _ := items["type"].(string); itemType == "string" {
				rel.Format = model.RelationFormat_tag
				// Extract examples if present
				if examples, hasExamples := prop["examples"]; hasExamples {
					if exampleArray, ok := examples.([]interface{}); ok {
						for _, val := range exampleArray {
							if strVal, ok := val.(string); ok {
								rel.Examples = append(rel.Examples, strVal)
							}
						}
					}
				}
			}
		}
		
	case "object":
		// This is an object relation
		rel.Format = model.RelationFormat_object
		
	default:
		// Check for const (like Type property)
		if _, hasConst := prop["const"]; hasConst {
			// This is likely the Type property
			rel.Format = model.RelationFormat_object
		}
	}
	
	return rel
}

// CreateRelationSnapshots creates snapshots for all discovered relations
func (si *SchemaImporter) CreateRelationSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot
	
	for key, rel := range si.relations {
		// Skip if this is a bundled relation
		if _, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
			continue
		}
		
		details := si.getRelationDetails(rel)
		
		snapshot := &common.Snapshot{
			Id: propIdPrefix + key,
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeRelation,
				Data: &common.StateSnapshot{
					Details:       details,
					RelationLinks: bundledRelationLinks(details),
					ObjectTypes:   []string{bundle.TypeKeyRelation.String()},
					Key:           key,
				},
			},
		}
		
		snapshots = append(snapshots, snapshot)
		
		// Track the relation ID for deduplication
		si.existingRels[key] = snapshot.Id
	}
	
	return snapshots
}

// CreateRelationOptionSnapshots creates snapshots for relation options (status and tag examples)
func (si *SchemaImporter) CreateRelationOptionSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot

	for key, rel := range si.relations {
		// Skip if this is a bundled relation
		if _, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
			continue
		}

		// Create options for status relations
		if rel.Format == model.RelationFormat_status && len(rel.Options) > 0 {
			for _, option := range rel.Options {
				details := si.getRelationOptionDetails(option, key)
				optionKey := bson.NewObjectId().Hex()

				snapshot := &common.Snapshot{
					Id: "opt_" + optionKey,
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypeRelationOption,
						Data: &common.StateSnapshot{
							Details:       details,
							RelationLinks: bundledRelationLinks(details),
							ObjectTypes:   []string{bundle.TypeKeyRelationOption.String()},
							Key:           optionKey,
						},
					},
				}

				snapshots = append(snapshots, snapshot)
			}
		}

		// Create options for tag relations with examples
		if rel.Format == model.RelationFormat_tag && len(rel.Examples) > 0 {
			for _, example := range rel.Examples {
				details := si.getRelationOptionDetails(example, key)
				optionKey := bson.NewObjectId().Hex()

				snapshot := &common.Snapshot{
					Id: "opt_" + optionKey,
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypeRelationOption,
						Data: &common.StateSnapshot{
							Details:       details,
							RelationLinks: bundledRelationLinks(details),
							ObjectTypes:   []string{bundle.TypeKeyRelationOption.String()},
							Key:           optionKey,
						},
					},
				}

				snapshots = append(snapshots, snapshot)
			}
		}
	}

	return snapshots
}

// CreateTypeSnapshots creates snapshots for all discovered types
func (si *SchemaImporter) CreateTypeSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot
	
	for typeName, schema := range si.schemas {
		// Use schema's x-type-key if available, otherwise generate
		typeKey := schema.TypeKey
		if typeKey == "" {
			typeKey = bson.NewObjectId().Hex()
		}
		
		// Collect relation IDs
		var relationIds []string
		var featuredIds []string
		var regularIds []string
		
		for _, relKey := range schema.Properties {
			relId := si.getRelationId(relKey)
			if relId != "" {
				relationIds = append(relationIds, relId)
				
				// Check if featured
				if rel, ok := schema.Relations[relKey]; ok && rel.Featured {
					featuredIds = append(featuredIds, relId)
				} else {
					regularIds = append(regularIds, relId)
				}
			}
		}
		
		// If no featured relations specified, use first 3
		if len(featuredIds) == 0 && len(relationIds) > 0 {
			maxFeatured := 3
			if len(relationIds) < maxFeatured {
				maxFeatured = len(relationIds)
			}
			featuredIds = relationIds[:maxFeatured]
			if len(relationIds) > maxFeatured {
				regularIds = relationIds[maxFeatured:]
			}
		}
		
		details := si.getObjectTypeDetails(typeName, typeKey, featuredIds, regularIds)
		
		snapshot := &common.Snapshot{
			Id: typeIdPrefix + typeKey,
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeObjectType,
				Data: &common.StateSnapshot{
					Details:       details,
					RelationLinks: bundledRelationLinks(details),
					ObjectTypes:   []string{bundle.TypeKeyObjectType.String()},
					Key:           typeKey,
				},
			},
		}
		
		snapshots = append(snapshots, snapshot)
		
		// Track the type ID for lookup
		si.existingTypes[typeKey] = snapshot.Id
	}
	
	return snapshots
}

// GetTypeKeyByName returns the type key for a given type name
func (si *SchemaImporter) GetTypeKeyByName(typeName string) string {
	if schema, ok := si.schemas[typeName]; ok {
		if schema.TypeKey != "" {
			return schema.TypeKey
		}
		// Return the generated ID if no x-type-key
		if typeId, ok := si.existingTypes[schema.TypeKey]; ok {
			return strings.TrimPrefix(typeId, typeIdPrefix)
		}
	}
	return ""
}

// GetRelationKeyByName returns the relation key for a given property name
func (si *SchemaImporter) GetRelationKeyByName(propName string) string {
	// First check if any schema has this property
	for _, schema := range si.schemas {
		for relKey, rel := range schema.Relations {
			if rel.Name == propName {
				return relKey
			}
		}
	}
	
	// Check global relations
	for key, rel := range si.relations {
		if rel.Name == propName {
			return key
		}
	}
	
	return ""
}

// HasSchemas returns true if any schemas were loaded
func (si *SchemaImporter) HasSchemas() bool {
	return len(si.schemas) > 0
}

// Helper functions

func getStringField(data map[string]interface{}, field string) string {
	if v, ok := data[field].(string); ok {
		return v
	}
	return ""
}

func getBoolField(data map[string]interface{}, field string) bool {
	if v, ok := data[field].(bool); ok {
		return v
	}
	return false
}

func (si *SchemaImporter) getRelationId(relKey string) string {
	// First check if it's a bundled relation
	if _, err := bundle.GetRelation(domain.RelationKey(relKey)); err == nil {
		return relKey
	}
	
	// Check our tracked relations
	if id, ok := si.existingRels[relKey]; ok {
		return id
	}
	
	// Generate ID for this relation
	return propIdPrefix + relKey
}

func (si *SchemaImporter) getRelationDetails(rel *RelationInfo) *domain.Details {
	details := domain.NewDetails()
	details.SetFloat64(bundle.RelationKeyRelationFormat, float64(rel.Format))
	details.SetString(bundle.RelationKeyName, rel.Name)
	details.SetString(bundle.RelationKeyRelationKey, rel.Key)
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))
	
	if rel.Description != "" {
		details.SetString(bundle.RelationKeyDescription, rel.Description)
	}
	
	// Set includeTime for date relations
	if rel.Format == model.RelationFormat_date {
		details.SetBool(bundle.RelationKeyRelationFormatIncludeTime, rel.IncludeTime)
	}
	
	// Create unique key
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, rel.Key)
	if err != nil {
		log.Warnf("failed to create unique key for schema relation: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	
	return details
}

func (si *SchemaImporter) getRelationOptionDetails(name, relationKey string) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeyRelationKey, relationKey)
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))
	
	// Create unique key for the relation option
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, bson.NewObjectId().Hex())
	if err != nil {
		log.Warnf("failed to create unique key for schema relation option: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	
	return details
}

func (si *SchemaImporter) getObjectTypeDetails(name, key string, featuredIds, regularIds []string) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_objectType))
	details.SetInt64(bundle.RelationKeyRecommendedLayout, int64(model.ObjectType_basic))
	details.SetString(bundle.RelationKeyType, bundle.TypeKeyObjectType.String())
	
	if len(featuredIds) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, featuredIds)
	}
	if len(regularIds) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedRelations, regularIds)
	}
	
	// Create unique key for the object type
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, key)
	if err != nil {
		log.Warnf("failed to create unique key for schema object type: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	
	return details
}


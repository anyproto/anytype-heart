package markdown

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// Verify that SchemaImporter implements schema.PropertyResolver
var _ schema.PropertyResolver = (*SchemaImporter)(nil)

// SchemaImporter handles schema-based import workflow
type SchemaImporter struct {
	schemas         map[string]*schema.Schema    // filename -> parsed schema
	existingTypes   map[string]string            // typeKey -> typeId
	existingRels    map[string]string            // relationKey -> relationId
	relationOptions map[string]map[string]string // relationKey -> optionName -> optionId
	parser          schema.Parser

	// ID prefixes for import
	propIdPrefix string
	typeIdPrefix string
}

// GetSchemas returns all loaded schemas
func (si *SchemaImporter) GetSchemas() map[string]*schema.Schema {
	return si.schemas
}

func NewSchemaImporter() *SchemaImporter {
	return &SchemaImporter{
		schemas:         make(map[string]*schema.Schema),
		existingTypes:   make(map[string]string),
		existingRels:    make(map[string]string),
		relationOptions: make(map[string]map[string]string),
		parser:          schema.NewJSONSchemaParser(),
		propIdPrefix:    "import_prop_",
		typeIdPrefix:    "import_type_",
	}
}

// LoadSchemas loads all JSON schema files from import source
func (si *SchemaImporter) LoadSchemas(importSource source.Source, allErrors *common.ConvertError) error {
	return importSource.Iterate(func(fileName string, fileReader io.ReadCloser) bool {
		if strings.HasSuffix(fileName, ".json") {
			defer fileReader.Close()

			schemaData, err := io.ReadAll(fileReader)
			if err != nil {
				allErrors.Add(fmt.Errorf("failed to read schema file: %w", err))
				return true // continue iteration
			}

			// Quick check: must have x-app field to be an Anytype schema
			if !strings.Contains(string(schemaData), `"x-app"`) {
				// Not an Anytype schema file, skip silently
				return true // continue iteration
			}

			// Try to parse as JSON Schema
			parsedSchema, err := si.parser.Parse(bytes.NewReader(schemaData))
			if err != nil {
				// Not a valid schema file, skip silently
				return true // continue iteration
			}

			// Additional validation: must have at least a type or relations
			if parsedSchema.Type == nil && len(parsedSchema.Relations) == 0 {
				// Empty schema, skip
				return true // continue iteration
			}

			si.schemas[fileName] = parsedSchema
		}
		return true // continue iteration
	})
}

// CreateRelationSnapshots creates snapshots for all relations found in schemas
func (si *SchemaImporter) CreateRelationSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot
	relMap := make(map[string]bool) // track processed relations

	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			if relMap[rel.Key] {
				continue // already processed
			}
			relMap[rel.Key] = true

			// Check if it's a bundled relation
			relationId := si.propIdPrefix + rel.Key
			si.existingRels[rel.Key] = relationId

			// Skip creating snapshot for collection relation, but still register it
			if rel.Key == schema.CollectionPropertyKey {
				continue // skip collection relation snapshot, handled separately
			}

			details := rel.ToDetails()
			snapshot := &common.Snapshot{
				Id: relationId,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeRelation,
					Data: &common.StateSnapshot{
						Blocks: []*model.Block{{
							Id: relationId,
							Content: &model.BlockContentOfSmartblock{
								Smartblock: &model.BlockContentSmartblock{},
							},
						}},
						Details:       details,
						Key:           rel.Key,
						ObjectTypes: []string{
							bundle.TypeKeyRelation.String(),
						},
					},
				},
			}
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots
}

func (si *SchemaImporter) optionId(relationKey, optionName string) string {
	// Generate a unique ID for the option based on relation key and option name
	return si.propIdPrefix + "option_" + relationKey + "_" + optionName
}

// CreateRelationOptionSnapshots creates snapshots for relation options (for select/multi-select relations)
func (si *SchemaImporter) CreateRelationOptionSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot

	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			var optionsToCreate []string

			// Collect options based on relation format
			switch rel.Format {
			case model.RelationFormat_status:
				optionsToCreate = rel.Options
			case model.RelationFormat_tag:
				optionsToCreate = rel.Examples
			default:
				continue
			}

			if len(optionsToCreate) == 0 {
				continue
			}

			// Initialize option map for this relation
			if si.relationOptions[rel.Key] == nil {
				si.relationOptions[rel.Key] = make(map[string]string)
			}

			for _, opt := range optionsToCreate {
				optionId := si.optionId(rel.Key, opt)

				// Track option ID
				si.relationOptions[rel.Key][opt] = optionId

				snapshot := &common.Snapshot{
					Id: optionId,
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypeRelationOption,
						Data: &common.StateSnapshot{
							Blocks: []*model.Block{{
								Id: optionId,
								Content: &model.BlockContentOfSmartblock{
									Smartblock: &model.BlockContentSmartblock{},
								},
							}},
							Details: rel.CreateOptionDetails(opt, ""),
							ObjectTypes: []string{
								bundle.TypeKeyRelationOption.String(),
							},
						},
					},
				}
				snapshots = append(snapshots, snapshot)
			}
		}
	}

	return snapshots
}

// randomIconColor returns a random color for (1-10)
func randomIconColor() int {
	return rand.Intn(10) + 1 // returns a number between 1 and 10
}

// CreateTypeSnapshots creates snapshots for all object types found in schemas
func (si *SchemaImporter) CreateTypeSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot
	typeMap := make(map[string]bool) // track processed types

	for _, s := range si.schemas {
		if s.Type != nil {
			t := s.Type
			if typeMap[t.Key] {
				continue // already processed
			}
			typeMap[t.Key] = true

			// Check if it's a bundled type
			if t.IsBundled() {
				// continue // it's a bundled type, skip
			}

			typeId := si.typeIdPrefix + t.Key
			si.existingTypes[t.Key] = typeId

			// Set KeyToIdFunc to convert relation keys to IDs
			t.KeyToIdFunc = func(key string) string {
				if relId, exists := si.existingRels[key]; exists {
					return relId
				}
				// For bundled relations, return the key as-is
				return key
			}

			details := t.ToDetails()
			// inject random color if icon set
			if details.GetString(bundle.RelationKeyIconName) != "" && !details.Get(bundle.RelationKeyIconOption).IsInt64() {
				// Set a random color for the icon
				details.SetInt64(bundle.RelationKeyIconOption, int64(randomIconColor()))
			}

			snapshot := &common.Snapshot{
				Id: typeId,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeObjectType,
					Data: &common.StateSnapshot{
						Blocks: []*model.Block{{
							Id: typeId,
							Content: &model.BlockContentOfSmartblock{
								Smartblock: &model.BlockContentSmartblock{},
							},
						}},
						Details:       details,
						Key:           t.Key,
						ObjectTypes: []string{
							bundle.TypeKeyObjectType.String(),
						},
					},
				},
			}
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots
}

// GetTypeKeyByName returns type key for given type name
func (si *SchemaImporter) GetTypeKeyByName(name string) string {
	// Check if it's a known type in our schemas by type name
	for _, s := range si.schemas {
		if s.Type != nil && s.Type.Name == name {
			return s.Type.Key
		}
	}

	return ""
}

// GetRelationKeyByName returns relation key by property name
func (si *SchemaImporter) GetRelationKeyByName(name string) (string, bool) {
	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			if rel.Name == name {
				return rel.Key, true
			}
		}
	}
	return "", false
}

// HasSchemas returns true if any schemas were loaded
func (si *SchemaImporter) HasSchemas() bool {
	return len(si.schemas) > 0
}

// ResolvePropertyKey returns the property key for a given name from schemas
// This implements the yaml.PropertyResolver interface
func (si *SchemaImporter) ResolvePropertyKey(name string) string {
	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			// Check if the relation name matches (case-sensitive)
			if rel.Name == name {
				return rel.Key
			}
		}
	}
	return ""
}

// GetRelationFormat returns the format for a given relation key
func (si *SchemaImporter) GetRelationFormat(key string) model.RelationFormat {
	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			if rel.Key == key {
				return rel.Format
			}
		}
	}
	return model.RelationFormat_shorttext
}

// ResolveOptionValue converts option name to option ID for a given relation
func (si *SchemaImporter) ResolveOptionValue(relationKey string, optionName string) string {
	if options, exists := si.relationOptions[relationKey]; exists {
		if optionId, found := options[optionName]; found {
			return optionId
		}
	}
	// If no schema option found, return the name as-is
	return si.optionId(relationKey, optionName)
}

// ResolveOptionValues converts option names to option IDs for a given relation
func (si *SchemaImporter) ResolveOptionValues(relationKey string, optionNames []string) []string {
	result := make([]string, 0, len(optionNames))
	for _, name := range optionNames {
		result = append(result, si.ResolveOptionValue(relationKey, name))
	}
	return result
}

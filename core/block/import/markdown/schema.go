package markdown

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// SchemaImporter handles schema-based import workflow
type SchemaImporter struct {
	schemas       map[string]*schema.Schema // filename -> parsed schema
	existingTypes map[string]string         // typeKey -> typeId
	existingRels  map[string]string         // relationKey -> relationId
	parser        schema.Parser

	// ID prefixes for import
	propIdPrefix string
	typeIdPrefix string
}

func NewSchemaImporter() *SchemaImporter {
	return &SchemaImporter{
		schemas:       make(map[string]*schema.Schema),
		existingTypes: make(map[string]string),
		existingRels:  make(map[string]string),
		parser:        schema.NewJSONSchemaParser(),
		propIdPrefix:  "import_prop_",
		typeIdPrefix:  "import_type_",
	}
}

// LoadSchemas loads all JSON schema files from import source
func (si *SchemaImporter) LoadSchemas(importSource source.Source, allErrors *common.ConvertError) error {
	importSource.Iterate(func(fileName string, fileReader io.ReadCloser) bool {
		if strings.HasSuffix(fileName, ".json") {
			defer fileReader.Close()

			schemaData, err := io.ReadAll(fileReader)
			if err != nil {
				allErrors.Add(fmt.Errorf("failed to read schema file %s: %w", fileName, err))
				return true // continue iteration
			}

			// Try to parse as JSON Schema
			parsedSchema, err := si.parser.Parse(bytes.NewReader(schemaData))
			if err != nil {
				// Not a schema file, skip silently
				return true // continue iteration
			}

			si.schemas[fileName] = parsedSchema
		}
		return true // continue iteration
	})

	return nil
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
						RelationLinks: bundledRelationLinks(details),
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

// CreateRelationOptionSnapshots creates snapshots for relation options (for select/multi-select relations)
func (si *SchemaImporter) CreateRelationOptionSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot

	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			if rel.Format != model.RelationFormat_status {
				continue
			}

			// Get options from relation
			if len(rel.Options) == 0 {
				continue
			}

			for _, opt := range rel.Options {
				optionId := si.propIdPrefix + "option_" + rel.Key + "_" + opt

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
				continue // it's a bundled type, skip
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
						RelationLinks: bundledRelationLinks(details),
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

	// Default to generic Page type
	return bundle.TypeKeyPage.String()
}

// GetRelationKeyByName returns relation key by property name
func (si *SchemaImporter) GetRelationKeyByName(name string) (string, bool) {
	for _, s := range si.schemas {
		for _, rel := range s.Relations {
			if rel.Key == name {
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
// This implements the YAMLPropertyResolver interface
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

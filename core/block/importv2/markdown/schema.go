package markdown

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
)

// schemaSet holds the Anytype JSON schemas found in the source (files
// containing the "x-app" marker). Presence of schemas adopts their keys and
// formats for front-matter properties and disables directory pages and
// properties-as-blocks (v1 behavior).
type schemaSet struct {
	// ordered by file name for deterministic emission
	fileNames []string
	schemas   map[string]*schema.Schema
}

// loadSchemas parses every candidate schema file during pass 1 (schemas are
// small; this is listing-scale work, not content-scale).
func loadSchemas(ctx context.Context, src source.Source) (*schemaSet, error) {
	set := &schemaSet{schemas: map[string]*schema.Schema{}}
	parser := schema.NewJSONSchemaParser()
	err := src.Walk(ctx, func(e source.Entry) error {
		if !strings.HasSuffix(e.Name, ".json") {
			return nil
		}
		reader, err := src.Open(ctx, e.Name)
		if err != nil {
			return fmt.Errorf("open schema candidate %q: %w", e.Name, err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return fmt.Errorf("read schema candidate %q: %w", e.Name, err)
		}
		if !bytes.Contains(data, []byte(`"x-app"`)) {
			return nil // not an anytype schema
		}
		parsed, err := parser.Parse(bytes.NewReader(data))
		if err != nil || (parsed.Type == nil && len(parsed.Relations) == 0) {
			return nil // invalid or empty: skipped silently, as in v1
		}
		set.fileNames = append(set.fileNames, e.Name)
		set.schemas[e.Name] = parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(set.fileNames) == 0 {
		return nil, nil
	}
	sort.Strings(set.fileNames)
	return set, nil
}

func (s *schemaSet) propertyKey(typeName, name string) string {
	if typeName != "" {
		for _, fileName := range s.fileNames {
			parsed := s.schemas[fileName]
			if parsed.Type == nil || parsed.Type.Name != typeName {
				continue
			}
			for _, relation := range parsed.Relations {
				if relation.Name == name {
					return relation.Key
				}
			}
		}
	}
	for _, fileName := range s.fileNames {
		for _, relation := range s.schemas[fileName].Relations {
			if relation.Name == name {
				return relation.Key
			}
		}
	}
	return ""
}

func (s *schemaSet) relationFormat(typeName, key string) (model.RelationFormat, bool) {
	if typeName != "" {
		for _, fileName := range s.fileNames {
			parsed := s.schemas[fileName]
			if parsed.Type == nil || parsed.Type.Name != typeName {
				continue
			}
			for _, relation := range parsed.Relations {
				if relation.Key == key {
					return relation.Format, true
				}
			}
		}
	}
	for _, fileName := range s.fileNames {
		for _, relation := range s.schemas[fileName].Relations {
			if relation.Key == key {
				return relation.Format, true
			}
		}
	}
	return 0, false
}

func (s *schemaSet) typeKeyByName(name string) string {
	for _, fileName := range s.fileNames {
		if parsed := s.schemas[fileName]; parsed.Type != nil && parsed.Type.Name == name {
			return parsed.Type.Key
		}
	}
	return ""
}

// emitDefinitions streams every schema-declared relation, option and type at
// the start of pass 2, and pre-populates the converter's dedup sets so YAML
// processing never redefines them.
func (c *Converter) emitSchemaDefinitions(ctx context.Context, sink importv2.Sink) error {
	if c.schemas == nil {
		return nil
	}
	emittedRelationKeys := map[string]bool{}
	for _, fileName := range c.schemas.fileNames {
		parsed := c.schemas.schemas[fileName]
		relationKeys := make([]string, 0, len(parsed.Relations))
		for key := range parsed.Relations {
			relationKeys = append(relationKeys, key)
		}
		sort.Strings(relationKeys)
		for _, key := range relationKeys {
			relation := parsed.Relations[key]
			if key == schema.CollectionPropertyKey || emittedRelationKeys[key] {
				continue
			}
			emittedRelationKeys[key] = true
			if !bundle.HasRelation(domain.RelationKey(key)) && !c.emittedRelations[key] {
				c.emittedRelations[key] = true
				if err := sink.Object(ctx, schemaRelationObject(relation)); err != nil {
					return err
				}
			}
			if err := c.emitSchemaOptions(ctx, relation, sink); err != nil {
				return err
			}
		}
		if parsed.Type != nil {
			if err := c.emitSchemaType(ctx, parsed.Type, sink); err != nil {
				return err
			}
		}
	}
	return nil
}

// emitSchemaOptions emits the options a schema declares up front (status
// Options, tag Examples); YAML-encountered values follow lazily per page.
func (c *Converter) emitSchemaOptions(ctx context.Context, relation *schema.Relation, sink importv2.Sink) error {
	var declared []string
	switch relation.Format {
	case model.RelationFormat_status:
		declared = relation.Options
	case model.RelationFormat_tag:
		declared = relation.Examples
	default:
		return nil
	}
	for _, optionName := range declared {
		sourceKey := optionSourceKey(relation.Key, optionName)
		if c.emittedOptions[sourceKey] {
			continue
		}
		c.emittedOptions[sourceKey] = true
		if err := sink.Object(ctx, optionObject(relation.Key, optionName)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Converter) emitSchemaType(ctx context.Context, schemaType *schema.Type, sink importv2.Sink) error {
	if _, ok := c.emittedTypes[schemaType.Name]; ok {
		return nil
	}
	c.emittedTypes[schemaType.Name] = schemaType.Key

	// Relation key references inside the type become resolvable targets:
	// bundled → bundled URL (passthrough), schema-defined → relation source
	// key (resolved to the final relation object id).
	schemaType.KeyToIdFunc = func(key string) string {
		if bundle.HasRelation(domain.RelationKey(key)) {
			return domain.RelationKey(key).BundledURL()
		}
		return relationSourceKey(key)
	}
	details := schemaType.ToDetails()
	if details.GetString(bundle.RelationKeyIconName) != "" && !details.Get(bundle.RelationKeyIconOption).IsInt64() {
		details.SetInt64(bundle.RelationKeyIconOption, stableIconOption(schemaType.Key))
	}
	object := &importv2.Object{
		SourceKey: typeSourceKey(schemaType.Name),
		SbType:    coresb.SmartBlockTypeObjectType,
		Payload: &importv2.Snapshot{
			Key:         schemaType.Key,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyObjectType.String()},
		},
	}
	return sink.Object(ctx, object)
}

func schemaRelationObject(relation *schema.Relation) *importv2.Object {
	return &importv2.Object{
		SourceKey: relationSourceKey(relation.Key),
		SbType:    coresb.SmartBlockTypeRelation,
		Payload: &importv2.Snapshot{
			Key:         relation.Key,
			Details:     relation.ToDetails(),
			ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		},
	}
}

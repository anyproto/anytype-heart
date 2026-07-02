package markdown

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
)

// stableResolver mints deterministic relation keys from property names, so
// the same source always yields the same keys (and re-import converges to
// the same derived relation objects). v1 minted random bson ids here.
type stableResolver struct {
	nameToKey map[string]string
}

func newStableResolver() *stableResolver {
	return &stableResolver{nameToKey: map[string]string{}}
}

func (r *stableResolver) ResolvePropertyKey(_ string, name string) string {
	if key, ok := r.nameToKey[name]; ok {
		return key
	}
	key := stableKey("md", name)
	r.nameToKey[name] = key
	return key
}

func (r *stableResolver) GetRelationFormat(_ string, _ string) model.RelationFormat {
	return model.RelationFormat_longtext
}

func (r *stableResolver) GetRelationOptions(string) map[string]string { return nil }

func (r *stableResolver) ResolveOptionValue(_ string, optionValue string) string {
	return optionValue
}

func (r *stableResolver) ResolveOptionValues(_ string, optionValues []string) []string {
	return optionValues
}

func stableKey(prefix, name string) string {
	sum := sha256.Sum256([]byte(name))
	return prefix + hex.EncodeToString(sum[:8])
}

// sourcePathHash mirrors v1's hashed sourceFilePath detail so re-import
// dedup keeps working across engine versions.
func sourcePathHash(sourcePath string) string {
	sum := sha256.Sum256([]byte(sourcePath))
	return hex.EncodeToString(sum[:])
}

// optionColors is the anytype option palette; the color is derived from the
// option name so output is deterministic (v1 rolled dice).
var optionColors = []string{"grey", "yellow", "orange", "red", "pink", "purple", "blue", "ice", "teal", "lime"}

func stableOptionColor(name string) string {
	sum := sha256.Sum256([]byte(name))
	return optionColors[int(sum[0])%len(optionColors)]
}

func relationSourceKey(key string) string     { return "relation:" + key }
func optionSourceKey(key, name string) string { return "option:" + key + ":" + name }
func typeSourceKey(name string) string        { return "type:" + name }

// emitPropertyDefinitions streams the relation, option and type objects a
// page's front-matter introduces, before the page itself (definitions before
// use). Bundled relations are never redefined. It returns the page's detail
// entries with option values rewritten to option source keys.
func (c *Converter) emitPropertyDefinitions(ctx context.Context, properties []yaml.Property, typeName string, sink importv2.Sink) (details []domain.Detail, typeKey string, err error) {
	for _, property := range properties {
		if !bundle.HasRelation(domain.RelationKey(property.Key)) && !c.emittedRelations[property.Key] {
			c.emittedRelations[property.Key] = true
			if err := sink.Object(ctx, relationObject(property)); err != nil {
				return nil, "", err
			}
		}
		value := property.Value
		if property.Format == model.RelationFormat_object || property.Format == model.RelationFormat_file {
			resolved, err := c.resolveObjectValues(ctx, value, sink)
			if err != nil {
				return nil, "", err
			}
			value = resolved
		}
		if property.Format == model.RelationFormat_tag || property.Format == model.RelationFormat_status {
			optionKeys := make([]string, 0)
			for _, optionName := range stringOrList(property.Value) {
				if optionName == "" {
					continue
				}
				sourceKey := optionSourceKey(property.Key, optionName)
				if !c.emittedOptions[sourceKey] {
					c.emittedOptions[sourceKey] = true
					if err := sink.Object(ctx, optionObject(property.Key, optionName)); err != nil {
						return nil, "", err
					}
				}
				optionKeys = append(optionKeys, sourceKey)
			}
			value = domain.StringList(optionKeys)
		}
		details = append(details, domain.Detail{Key: domain.RelationKey(property.Key), Value: value})
	}

	typeKey = bundle.TypeKeyPage.String()
	if typeName != "" {
		key, err := c.emitTypeDefinition(ctx, typeName, properties, sink)
		if err != nil {
			return nil, "", err
		}
		typeKey = key
	}
	return details, typeKey, nil
}

// stringOrList reads a value that yaml may deliver as either a scalar or a
// list (single status vs multi tag).
func stringOrList(value domain.Value) []string {
	if single, ok := value.TryString(); ok {
		return []string{single}
	}
	return value.StringList()
}

// resolveObjectValues rewrites object/file property values (source-relative
// paths from the yaml parser) to entry source keys, emitting file objects
// for non-page targets. Unknown values are left as-is (resolver leniency).
func (c *Converter) resolveObjectValues(ctx context.Context, value domain.Value, sink importv2.Sink) (domain.Value, error) {
	resolveOne := func(raw string) (string, error) {
		entryName, found := c.lookupEntry(raw)
		if !found {
			return raw, nil
		}
		if !isPageEntry(entryName) {
			if err := c.emitFileObject(ctx, entryName, sink); err != nil {
				return "", err
			}
		}
		return entryName, nil
	}
	if single, ok := value.TryString(); ok {
		resolved, err := resolveOne(single)
		if err != nil {
			return domain.Value{}, err
		}
		return domain.String(resolved), nil
	}
	values := value.StringList()
	resolved := make([]string, len(values))
	for i, item := range values {
		mapped, err := resolveOne(item)
		if err != nil {
			return domain.Value{}, err
		}
		resolved[i] = mapped
	}
	return domain.StringList(resolved), nil
}

// emitTypeDefinition emits an object type on first use. A name matching a
// bundled type resolves to the bundled key instead (v1 parity). The type's
// recommended relations are the first-use page's properties — deterministic,
// unlike v1's map-order last-file-wins.
func (c *Converter) emitTypeDefinition(ctx context.Context, typeName string, properties []yaml.Property, sink importv2.Sink) (string, error) {
	if key, ok := c.emittedTypes[typeName]; ok {
		return key, nil
	}
	if typeKey, err := bundle.GetTypeKeyByName(typeName); err == nil {
		c.emittedTypes[typeName] = typeKey.String()
		return typeKey.String(), nil
	}
	key := stableKey("mdtype", typeName)
	c.emittedTypes[typeName] = key

	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key)
	if err != nil {
		return "", fmt.Errorf("type unique key %q: %w", key, err)
	}
	recommended := make([]string, 0, len(properties))
	for _, property := range properties {
		if bundle.HasRelation(domain.RelationKey(property.Key)) {
			recommended = append(recommended, domain.RelationKey(property.Key).BundledURL())
			continue
		}
		recommended = append(recommended, relationSourceKey(property.Key))
	}
	object := &importv2.Object{
		SourceKey: typeSourceKey(typeName),
		SbType:    coresb.SmartBlockTypeObjectType,
		Payload: &importv2.Snapshot{
			Key: key,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                 domain.String(typeName),
				bundle.RelationKeyUniqueKey:            domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyRecommendedLayout:    domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyRecommendedRelations: domain.StringList(recommended),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			}),
			ObjectTypes: []string{bundle.TypeKeyObjectType.String()},
		},
	}
	return key, sink.Object(ctx, object)
}

func relationObject(property yaml.Property) *importv2.Object {
	uniqueKey, _ := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, property.Key)
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:           domain.String(property.Name),
		bundle.RelationKeyRelationKey:    domain.String(property.Key),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(property.Format)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	})
	if uniqueKey != nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}
	if property.Format == model.RelationFormat_date {
		details.SetBool(bundle.RelationKeyRelationFormatIncludeTime, property.IncludeTime)
	}
	return &importv2.Object{
		SourceKey: relationSourceKey(property.Key),
		SbType:    coresb.SmartBlockTypeRelation,
		Payload: &importv2.Snapshot{
			Key:         property.Key,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		},
	}
}

func optionObject(relationKey, optionName string) *importv2.Object {
	optionKey := stableKey("mdopt", relationKey+"\x00"+optionName)
	uniqueKey, _ := domain.NewUniqueKey(coresb.SmartBlockTypeRelationOption, optionKey)
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:                domain.String(optionName),
		bundle.RelationKeyRelationKey:         domain.String(relationKey),
		bundle.RelationKeyRelationOptionColor: domain.String(stableOptionColor(optionName)),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
	})
	if uniqueKey != nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}
	return &importv2.Object{
		SourceKey: optionSourceKey(relationKey, optionName),
		SbType:    coresb.SmartBlockTypeRelationOption,
		Payload: &importv2.Snapshot{
			Key:         optionKey,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyRelationOption.String()},
		},
	}
}

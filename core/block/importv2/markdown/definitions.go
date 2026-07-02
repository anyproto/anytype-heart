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

// mdResolver implements schema.PropertyResolver for the yaml parser. Keys
// come from loaded schemas when available, else are minted deterministically
// from the property name (v1 minted random bson ids). Option values resolve
// to option source keys and are recorded for lazy emission before the page —
// one mechanism for both the schema and schema-less paths.
type mdResolver struct {
	schemas     *schemaSet
	nameToKey   map[string]string
	pending     []pendingOption
	pendingSeen map[string]bool
}

type pendingOption struct {
	relationKey string
	optionName  string
}

func newResolver(schemas *schemaSet) *mdResolver {
	return &mdResolver{
		schemas:     schemas,
		nameToKey:   map[string]string{},
		pendingSeen: map[string]bool{},
	}
}

func (r *mdResolver) ResolvePropertyKey(objectTypeName, name string) string {
	if r.schemas != nil {
		if key := r.schemas.propertyKey(objectTypeName, name); key != "" {
			return key
		}
	}
	if key, ok := r.nameToKey[name]; ok {
		return key
	}
	key := stableKey("md", name)
	r.nameToKey[name] = key
	return key
}

func (r *mdResolver) GetRelationFormat(objectTypeName, key string) model.RelationFormat {
	if r.schemas != nil {
		if format, ok := r.schemas.relationFormat(objectTypeName, key); ok {
			return format
		}
	}
	return model.RelationFormat_longtext
}

func (r *mdResolver) GetRelationOptions(string) map[string]string { return nil }

func (r *mdResolver) ResolveOptionValue(relationKey string, optionName string) string {
	sourceKey := optionSourceKey(relationKey, optionName)
	if !r.pendingSeen[sourceKey] {
		r.pendingSeen[sourceKey] = true
		r.pending = append(r.pending, pendingOption{relationKey: relationKey, optionName: optionName})
	}
	return sourceKey
}

func (r *mdResolver) ResolveOptionValues(relationKey string, optionNames []string) []string {
	resolved := make([]string, 0, len(optionNames))
	for _, name := range optionNames {
		resolved = append(resolved, r.ResolveOptionValue(relationKey, name))
	}
	return resolved
}

// takePending drains options encountered since the last call.
func (r *mdResolver) takePending() []pendingOption {
	pending := r.pending
	r.pending = nil
	return pending
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
// use). Bundled and schema-emitted relations are never redefined; option
// values were already resolved to option source keys by the resolver.
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
		details = append(details, domain.Detail{Key: domain.RelationKey(property.Key), Value: value})
	}

	// Options the resolver encountered while parsing this page's values.
	for _, option := range c.resolver.takePending() {
		sourceKey := optionSourceKey(option.relationKey, option.optionName)
		if c.emittedOptions[sourceKey] {
			continue
		}
		c.emittedOptions[sourceKey] = true
		if err := sink.Object(ctx, optionObject(option.relationKey, option.optionName)); err != nil {
			return nil, "", err
		}
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

// stableIconOption derives a 1..10 icon color deterministically.
func stableIconOption(key string) int64 {
	sum := sha256.Sum256([]byte(key))
	return int64(sum[0])%10 + 1
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

package schemaplan

import (
	"crypto/sha256"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// RelationRef is the stream reference for a plan property target: the bundled
// URL for bundled keys, the minted relation's source key otherwise.
func RelationRef(key domain.RelationKey) string {
	if bundle.HasRelation(key) {
		return key.BundledURL()
	}
	return "relation:" + CustomRelationKey(key).String()
}

// RelationObject builds the relation object for a non-bundled plan property
// target. Both converters emit the same object for the same plan key, and the
// engine's identity dedup collapses them across containers.
func RelationObject(planKey domain.RelationKey, name string, format model.RelationFormat) *importv2.Object {
	key := CustomRelationKey(planKey)
	if name == "" {
		name = string(planKey)
	}
	if format == 0 {
		format = model.RelationFormat_longtext
	}
	uniqueKey, _ := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key.String())
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyRelationKey:    domain.String(key.String()),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(format)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	})
	if uniqueKey != nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}
	return &importv2.Object{
		SourceKey: RelationRef(planKey),
		SbType:    coresb.SmartBlockTypeRelation,
		Payload: &importv2.Snapshot{
			Key:         key.String(),
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		},
	}
}

// defaultIconName is the icon for a type the plan left unnamed — the closest
// thing the layout says about what its members are.
func defaultIconName(layout model.ObjectTypeLayout) string {
	switch layout {
	case model.ObjectType_todo:
		return "checkbox"
	case model.ObjectType_profile:
		return "person"
	default:
		return "document"
	}
}

// StableIconOption picks one of Anytype's ten icon colors for a key. Types
// want the variety of a random color, but a re-import must not repaint the
// space, so the color is a hash of the key rather than a draw.
func StableIconOption(key string) int64 {
	sum := sha256.Sum256([]byte(key))
	return int64(sum[0])%10 + 1
}

// TypeSourceKey is the stream reference for a plan-defined type.
func TypeSourceKey(planKey domain.TypeKey) string {
	return "type:" + CustomTypeKey(planKey).String()
}

// TypeObject builds the object-type object for a plan-defined type. Its
// recommended relations reference the plan properties by RelationRef, so the
// caller must emit RelationObject for every non-bundled property first
// (definitions before use). Returns the minted type key pages reference in
// ObjectTypes.
func TypeObject(def TypeDefinition) (*importv2.Object, domain.TypeKey, error) {
	key := CustomTypeKey(def.Key)
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key.String())
	if err != nil {
		return nil, "", fmt.Errorf("plan type unique key %q: %w", key, err)
	}
	var featured, regular []string
	for _, prop := range def.Properties {
		if prop.Featured {
			featured = append(featured, RelationRef(prop.Key))
		} else {
			regular = append(regular, RelationRef(prop.Key))
		}
	}
	layout := def.Layout
	if layout == 0 {
		layout = model.ObjectType_basic
	}
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:              domain.String(def.Name),
		bundle.RelationKeyUniqueKey:         domain.String(uniqueKey.Marshal()),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(layout)),
		bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
	})
	// Every bundled type has a plural name and an icon; a minted one reads as
	// unfinished without them, and minting is now the only path. Both arrive
	// from the plan already bounded and vocabulary-checked by Sanitize.
	if def.PluralName != "" {
		details.SetString(bundle.RelationKeyPluralName, def.PluralName)
	}
	// An icon is not optional: with none the client falls back to the default
	// glyph, so a plan that named no icon — or whose icon Sanitize dropped —
	// would land the type among the bundled ones wearing a puzzle piece. The
	// layout is the only thing known about such a type, so it picks the icon.
	iconName := def.IconName
	if iconName == "" {
		iconName = defaultIconName(layout)
	}
	details.SetString(bundle.RelationKeyIconName, iconName)
	details.SetInt64(bundle.RelationKeyIconOption, StableIconOption(string(def.Key)))
	if len(featured) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, featured)
	}
	if len(regular) > 0 {
		details.SetStringList(bundle.RelationKeyRecommendedRelations, regular)
	}
	return &importv2.Object{
		SourceKey: TypeSourceKey(def.Key),
		SbType:    coresb.SmartBlockTypeObjectType,
		Payload: &importv2.Snapshot{
			Key:         key.String(),
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyObjectType.String()},
		},
	}, key, nil
}

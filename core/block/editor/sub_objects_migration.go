package editor

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type objectDeriver interface {
	DeriveTreeObject(ctx context.Context, spaceID string, params objectcache.TreeDerivationParams) (sb smartblock.SmartBlock, err error)
}

// Migrate legacy sub-objects to ordinary objects
type subObjectsMigration struct {
	workspace     *Workspaces
	objectDeriver objectDeriver
}

func (m *subObjectsMigration) migrateSubObjects(st *state.State) {
	m.iterateAllSubObjects(
		st,
		func(info smartblock.DocInfo) {
			uniqueKeyRaw := pbtypes.GetString(info.Details, bundle.RelationKeyUniqueKey.String())
			id, err := m.migrateSubObject(context.Background(), uniqueKeyRaw, info.Details, info.Type)
			if err == nil {
				log.With("objectId", id, "uniqueKey", uniqueKeyRaw).Warnf("migrated sub-object")
			} else if !errors.Is(err, treestorage.ErrTreeExists) {
				log.With("objectID", id).Errorf("failed to migrate subobject: %v", err)
			}
		},
	)
}

func (m *subObjectsMigration) migrateSubObject(
	ctx context.Context,
	uniqueKeyRaw string,
	details *types.Struct,
	typeKey domain.TypeKey,
) (id string, err error) {
	uniqueKey, err := domain.UnmarshalUniqueKey(uniqueKeyRaw)
	if err != nil {
		return "", fmt.Errorf("unmarshal unique key: %w", err)
	}
	sb, err := m.objectDeriver.DeriveTreeObject(ctx, m.workspace.SpaceID(), objectcache.TreeDerivationParams{
		Key: uniqueKey,
		InitFunc: func(id string) *smartblock.InitContext {
			st := state.NewDocWithUniqueKey(id, nil, uniqueKey).NewState()
			st.SetDetails(details)
			st.SetObjectTypeKey(typeKey)
			return &smartblock.InitContext{
				IsNewObject: true,
				State:       st,
				SpaceID:     m.workspace.SpaceID(),
			}
		},
	})
	if err != nil {
		return "", err
	}

	return sb.Id(), nil
}

const (
	collectionKeyRelationOptions = "opt"
	collectionKeyRelations       = "rel"
	collectionKeyObjectTypes     = "ot"
)

var objectTypeToCollection = map[domain.TypeKey]string{
	bundle.TypeKeyObjectType:     collectionKeyObjectTypes,
	bundle.TypeKeyRelation:       collectionKeyRelations,
	bundle.TypeKeyRelationOption: collectionKeyRelationOptions,
}

func collectionKeyToTypeKey(collKey string) (domain.TypeKey, bool) {
	for ot, v := range objectTypeToCollection {
		if v == collKey {
			return ot, true
		}
	}
	return "", false
}

func (m *subObjectsMigration) iterateAllSubObjects(st *state.State, proc func(smartblock.DocInfo)) {
	for typeKey, coll := range objectTypeToCollection {
		collection := st.GetSubObjectCollection(coll)
		if collection == nil {
			continue
		}

		for subObjectId, subObjectStruct := range collection.GetFields() {
			if v, ok := subObjectStruct.Kind.(*types.Value_StructValue); ok {
				uk, err := m.getUniqueKey(coll, subObjectId)
				if err != nil {
					log.With("collection", coll).Errorf("subobject migration: failed to get uniqueKey: %s", err.Error())
					continue
				}

				details := v.StructValue
				details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())

				proc(smartblock.DocInfo{
					SpaceID:    m.workspace.SpaceID(),
					Links:      nil,
					FileHashes: nil,
					Heads:      nil,
					Type:       typeKey,
					Details:    details,
				})
			} else {
				log.Errorf("got invalid value for %s.%s:%t", coll, subObjectId, subObjectStruct.Kind)
				continue
			}
		}
	}
	return
}

func (m *subObjectsMigration) getUniqueKey(collection, key string) (domain.UniqueKey, error) {
	typeKey, ok := collectionKeyToTypeKey(collection)
	if !ok {
		return nil, fmt.Errorf("unknown collection %s", collection)
	}

	var sbt smartblock2.SmartBlockType
	switch typeKey {
	case bundle.TypeKeyRelation:
		sbt = smartblock2.SmartBlockTypeRelation
	case bundle.TypeKeyObjectType:
		sbt = smartblock2.SmartBlockTypeObjectType
	case bundle.TypeKeyRelationOption:
		sbt = smartblock2.SmartBlockTypeRelationOption
	default:
		return nil, fmt.Errorf("unknown type key %s", typeKey)
	}

	return domain.NewUniqueKey(sbt, key)
}

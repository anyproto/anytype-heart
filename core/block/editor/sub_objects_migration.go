package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type subObjectsMigrator interface {
	migrateSubObjects(st *state.State)
}

type objectDeriver interface {
	DeriveTreeObject(ctx context.Context, params objectcache.TreeDerivationParams) (sb smartblock.SmartBlock, err error)
}

// Migrate legacy sub-objects to ordinary objects
type subObjectsMigration struct {
	workspace *Workspaces
}

// we use this key to store the flag that subobject has been migrated, so we never migrate it again
const (
	migratedKey = "_migrated"
)

func migrationSettingName(path []string) string {
	return migratedKey + "_" + strings.Join(path, "-")
}

func (m *subObjectsMigration) migrateSubObjects(st *state.State) {
	migratedSubObjects := 0
	m.iterateAllSubObjects(
		st,
		func(info smartblock.DocInfo, path []string) {
			if st.GetSetting(migrationSettingName(path)) != nil {
				// already migrated
				return
			}

			if pbtypes.GetBool(info.Details, migratedKey) {
				return
			}
			uniqueKeyRaw := pbtypes.GetString(info.Details, bundle.RelationKeyUniqueKey.String())
			id, err := m.migrateSubObject(context.Background(), uniqueKeyRaw, info.Details, info.Type)
			if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
				log.With("objectID", id).Errorf("failed to migrate subobject: %v", err)
				return
			}
			key := path[len(path)-1]

			// now, lets set some additional restrictions for old clients, to limit the ability to edit sub-objects and cause inconsistencies
			needToAddRestrictions := false
			switch info.Type {
			case bundle.TypeKeyRelation:
				format := pbtypes.GetInt64(info.Details, bundle.RelationKeyRelationFormat.String())
				if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
					// tags and statuses relations values are become readonly
					st.SetInStore(append(path, bundle.RelationKeyRelationReadonlyValue.String()), pbtypes.Bool(true))
				}
				if !lo.Contains(bundle.SystemRelations, domain.RelationKey(key)) {
					// no need to restrict system types, they are already restricted
					needToAddRestrictions = true
				}
			case bundle.TypeKeyObjectType:
				if !lo.Contains(bundle.SystemTypes, domain.TypeKey(key)) {
					// no need to restrict system types, they are already restricted
					needToAddRestrictions = true
				}
			case bundle.TypeKeyRelationOption:
				needToAddRestrictions = true
			default:
				// unsupported collection? skip
				return
			}

			st.SetSetting(migrationSettingName(path), pbtypes.Bool(true))

			// restrict all edits for older clients to avoid inconsistencies (migration only done once, changes are not going to sync)
			if needToAddRestrictions {
				// we can't add restrictions as it can lead to removing this field on the old client
				// todo: revise this
				// st.SetInStore(append(path, bundle.RelationKeyRestrictions.String()), pbtypes.IntList(1, 3, 4))
			}

			migratedSubObjects++
		},
	)
	if migratedSubObjects == 0 {
		return
	}
	log.With("migrated", migratedSubObjects).Warnf("migrated sub-objects")
	err := m.workspace.Apply(st)
	if err != nil {
		log.Errorf("failed to apply state: %v", err)
	}
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
	deriver, ok := m.workspace.Space().(objectDeriver)
	if !ok {
		return "", fmt.Errorf("can't get object deriver")
	}
	sb, err := deriver.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
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

func (m *subObjectsMigration) iterateAllSubObjects(st *state.State, proc func(info smartblock.DocInfo, path []string)) {
	for typeKey, coll := range objectTypeToCollection {
		collection := st.GetSubObjectCollection(coll)
		if collection == nil {
			continue
		}

		for subObjectId, subObjectStruct := range collection.GetFields() {
			if v, ok := subObjectStruct.Kind.(*types.Value_StructValue); ok {
				uk, err := m.getUniqueKey(coll, subObjectId)
				if err != nil {
					log.With("collection", coll).Errorf("subobject migration: failed to get uniqueKey: %s", err)
					continue
				}

				details := v.StructValue
				details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())

				proc(smartblock.DocInfo{
					Links:   nil,
					Heads:   nil,
					Type:    typeKey,
					Details: details,
				}, []string{coll, subObjectId})

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

package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) createRelation(ctx context.Context, space clientspace.Space, details *domain.Details) (id string, object *domain.Details, err error) {
	if details == nil {
		return "", nil, fmt.Errorf("create relation: no data")
	}

	if !details.Has(bundle.RelationKeyRelationFormat) {
		return "", nil, fmt.Errorf("missing relation format")
	} else if i, ok := details.TryInt64(bundle.RelationKeyRelationFormat); !ok {
		return "", nil, fmt.Errorf("invalid relation format: not a number")
	} else if model.RelationFormat(int(i)).String() == "" {
		return "", nil, fmt.Errorf("invalid relation format: unknown enum")
	}

	if details.GetString(bundle.RelationKeyName) == "" {
		return "", nil, fmt.Errorf("missing relation name")
	}

	if !details.Has(bundle.RelationKeyCreatedDate) {
		details.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())
	}

	object = details.Copy()

	key := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))

	if err := s.injectAndEnsureUniqueApiObjectKey(space.Id(), object, key.String(), coresb.SmartBlockTypeRelation); err != nil {
		return "", nil, fmt.Errorf("inject and ensure unique apiObjectKey: %w", err)
	}

	if key == "" {
		key = domain.RelationKey(bson.NewObjectId().Hex())
	} else if bundle.HasRelation(key) {
		object.SetString(bundle.RelationKeySourceObject, string(addr.BundledRelationURLPrefix+key))
	}
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, string(key))
	if err != nil {
		return "", nil, err
	}
	object.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	object.SetString(bundle.RelationKeyId, id)
	object.SetString(bundle.RelationKeyRelationKey, string(key))

	if details.GetInt64(bundle.RelationKeyRelationFormat) == int64(model.RelationFormat_status) {
		object.SetInt64(bundle.RelationKeyRelationMaxCount, 1)
	}

	if err = fillRelationFormatObjectTypes(ctx, space, object); err != nil {
		return "", nil, fmt.Errorf("failed to fill relation format object types: %w", err)
	}
	// todo: check the existence of objectTypes in space. InstallBundledObjects should be called same as for recommendedRelations on type creation

	object.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	setOriginalCreatedTimestamp(createState, details)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelation}, createState)
}

func fillRelationFormatObjectTypes(ctx context.Context, spc clientspace.Space, details *domain.Details) error {
	objectTypes := details.GetStringList(bundle.RelationKeyRelationFormatObjectTypes)

	for i, objectType := range objectTypes {
		// replace object type url with id
		typeKey, err := bundle.TypeKeyFromUrl(objectType)
		if err != nil {
			if i == 0 {
				// relationFormatObjectTypes detail already contains list of types' ids
				return nil
			}
			// should never happen
			return err
		}
		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, typeKey.String())
		if err != nil {
			// should never happen
			return err
		}
		id, err := spc.DeriveObjectID(ctx, uniqueKey)
		if err != nil {
			// should never happen
			return err
		}
		objectTypes[i] = id
	}
	details.SetStringList(bundle.RelationKeyRelationFormatObjectTypes, objectTypes)
	return nil
}

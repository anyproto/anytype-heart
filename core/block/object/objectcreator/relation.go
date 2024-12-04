package objectcreator

import (
	"context"
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createRelation(ctx context.Context, space clientspace.Space, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create relation: no data")
	}

	if v, ok := details.GetFields()[bundle.RelationKeyRelationFormat.String()]; !ok {
		return "", nil, fmt.Errorf("missing relation format")
	} else if i, ok := v.Kind.(*types.Value_NumberValue); !ok {
		return "", nil, fmt.Errorf("invalid relation format: not a number")
	} else if model.RelationFormat(int(i.NumberValue)).String() == "" {
		return "", nil, fmt.Errorf("invalid relation format: unknown enum")
	}

	if pbtypes.GetString(details, bundle.RelationKeyName.String()) == "" {
		return "", nil, fmt.Errorf("missing relation name")
	}

	object = pbtypes.CopyStruct(details, false)
	key := pbtypes.GetString(details, bundle.RelationKeyRelationKey.String())
	if key == "" {
		key = bson.NewObjectId().Hex()
	} else if bundle.HasRelation(key) {
		object.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(addr.BundledRelationURLPrefix + key)
	}

	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key)
	if err != nil {
		return "", nil, err
	}
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	if pbtypes.GetInt64(details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_status) {
		object.Fields[bundle.RelationKeyRelationMaxCount.String()] = pbtypes.Int64(1)
	}

	if err = FillRelationFormatObjectTypes(ctx, space, object); err != nil {
		return "", nil, fmt.Errorf("failed to fill relation format object types: %w", err)
	}
	// todo: check the existence of objectTypes in space. InstallBundledObjects should be called same as for recommendedRelations on type creation

	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relation))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelation}, createState)
}

func FillRelationFormatObjectTypes(ctx context.Context, spc clientspace.Space, details *types.Struct) error {
	objectTypes := pbtypes.GetStringList(details, bundle.RelationKeyRelationFormatObjectTypes.String())

	for i, objectType := range objectTypes {
		// replace object type url with id
		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, strings.TrimPrefix(objectType, addr.BundledObjectTypeURLPrefix))
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
	details.Fields[bundle.RelationKeyRelationFormatObjectTypes.String()] = pbtypes.StringList(objectTypes)
	return nil
}

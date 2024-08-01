package objectcreator

import (
	"context"
	"fmt"

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
	} else if i, ok := details.GetInt64(bundle.RelationKeyRelationFormat); !ok {
		return "", nil, fmt.Errorf("invalid relation format: not a number")
	} else if model.RelationFormat(int(i)).String() == "" {
		return "", nil, fmt.Errorf("invalid relation format: unknown enum")
	}

	if details.GetString(bundle.RelationKeyName) == "" {
		return "", nil, fmt.Errorf("missing relation name")
	}

	object = details.ShallowCopy()
	key := domain.RelationKey(details.GetString(bundle.RelationKeyRelationKey))
	if key == "" {
		key = domain.RelationKey(bson.NewObjectId().Hex())
	} else if bundle.HasRelation(key) {
		object.Set(bundle.RelationKeySourceObject, addr.BundledRelationURLPrefix+key)
	}
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, string(key))
	if err != nil {
		return "", nil, err
	}
	object.Set(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	object.Set(bundle.RelationKeyId, id)
	object.Set(bundle.RelationKeyRelationKey, key)
	if details.GetInt64OrDefault(bundle.RelationKeyRelationFormat, 0) == int64(model.RelationFormat_status) {
		object.Set(bundle.RelationKeyRelationMaxCount, 1)
	}
	// objectTypes := object.GetStringListOrDefault(bundle.RelationKeyRelationFormatObjectTypes, nil)
	// todo: check the objectTypes
	object.Set(bundle.RelationKeyLayout, model.ObjectType_relation)

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelation}, createState)
}

package objectcreator

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createRelationOption(ctx context.Context, space clientspace.Space, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create option: no data")
	}

	if pbtypes.GetString(details, "relationOptionText") != "" {
		return "", nil, fmt.Errorf("use name instead of relationOptionText")
	} else if pbtypes.GetString(details, "name") == "" {
		return "", nil, fmt.Errorf("name is empty")
	} else if pbtypes.GetString(details, bundle.RelationKeyRelationKey.String()) == "" {
		return "", nil, fmt.Errorf("invalid relation Key: unknown enum")
	}

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeRelationOption, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}

	object = pbtypes.CopyStruct(details, false)
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelationOption}, createState)
}

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *types.Struct) (domain.UniqueKey, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	if uniqueKey == "" {
		return domain.NewUniqueKey(sbType, bson.NewObjectId().Hex())
	}
	return domain.UnmarshalUniqueKey(uniqueKey)
}

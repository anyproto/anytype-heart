package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (c *creator) createRelationOption(ctx context.Context, spaceID string, details *types.Struct) (id string, object *types.Struct, err error) {
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

	object = pbtypes.CopyStruct(details)
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return c.CreateSmartBlockFromState(ctx, spaceID, []domain.TypeKey{bundle.TypeKeyRelationOption}, createState)
}

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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) createRelationOption(ctx context.Context, space clientspace.Space, details *domain.Details) (id string, object *domain.Details, err error) {
	if details == nil {
		return "", nil, fmt.Errorf("create option: no data")
	}

	if details.GetString(bundle.RelationKeyName) == "" {
		return "", nil, fmt.Errorf("name is empty")
	}
	if details.GetString(bundle.RelationKeyRelationKey) == "" {
		return "", nil, fmt.Errorf("relation key is empty")
	}
	if !details.Has(bundle.RelationKeyCreatedDate) {
		details.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())
	}
	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeRelationOption, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}

	object = details.Copy()
	object.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	object.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_relationOption))
	object.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	setOriginalCreatedTimestamp(createState, details)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelationOption}, createState)
}

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *domain.Details) (domain.UniqueKey, error) {
	uniqueKey := details.GetString(bundle.RelationKeyUniqueKey)
	if uniqueKey == "" {
		newUniqueKey, err := domain.NewUniqueKey(sbType, bson.NewObjectId().Hex())
		if err != nil {
			return nil, err
		}
		details.SetString(bundle.RelationKeyUniqueKey, newUniqueKey.Marshal())
		return newUniqueKey, err
	}
	return domain.UnmarshalUniqueKey(uniqueKey)
}

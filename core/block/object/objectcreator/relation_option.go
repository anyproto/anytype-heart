package objectcreator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
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
	uniqueKey, wasGenerated, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeRelationOption, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}

	object = details.Copy()
	object.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	object.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))
	if err = s.setOptionOrderId(object, space); err != nil {
		log.With("spaceID", space.Id()).Errorf("failed to to set orderId to new relation option: %v", err)
	}

	var objectKey string
	if !wasGenerated {
		objectKey = uniqueKey.InternalKey()
	}
	injectApiObjectKey(object, objectKey)

	if strings.TrimSpace(object.GetString(bundle.RelationKeyApiObjectKey)) == "" {
		object.SetString(bundle.RelationKeyApiObjectKey, transliterate(object.GetString(bundle.RelationKeyName)))
	}

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	setOriginalCreatedTimestamp(createState, details)
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyRelationOption}, createState)
}

func (s *service) setOptionOrderId(details *domain.Details, spc clientspace.Space) error {
	records, err := s.objectStore.SpaceIndex(spc.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       details.Get(bundle.RelationKeyRelationKey),
			},
			{
				RelationKey: bundle.RelationKeyOrderId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyOrderId,
			Type:        model.BlockContentDataviewSort_Asc,
			NoCollate:   true,
		}},
		Limit: 1,
	})

	if err != nil {
		return fmt.Errorf("failed to query relation options with orders: %w", err)
	}

	if len(records) > 0 {
		smallestOrderId := records[0].Details.GetString(bundle.RelationKeyOrderId)
		details.SetString(bundle.RelationKeyOrderId, order.GetSmallestOrder(smallestOrderId))
	}
	return nil
}

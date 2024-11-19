package date

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

func BuildDetailsFromTimestamp(
	ctx context.Context, spaceService space.Service, spaceId string, timestamp int64,
) (details *types.Struct, err error) {
	spc, err := spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get space service: %w", err)
	}

	dateTypeId, err := spc.GetTypeIdByKey(ctx, bundle.TypeKeyDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get date type id: %w", err)
	}

	dateSource := source.NewDate(source.DateSourceParams{
		Id: domain.FullID{
			SpaceID:  spaceId,
			ObjectID: dateutil.TimeToDateId(time.Unix(timestamp, 0), false),
		},
		DateObjectTypeId: dateTypeId,
	})

	detailsGetter, ok := dateSource.(source.SourceIdEndodedDetails)
	if !ok {
		return nil, fmt.Errorf("date object does not implement SourceIdEndodedDetails: %w", err)
	}
	return detailsGetter.DetailsFromId()
}

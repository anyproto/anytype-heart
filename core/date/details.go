package date

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

func BuildDetailsFromTimestamp(
	ctx context.Context, spaceService space.Service, spaceId string, timestamp int64,
) (details *domain.Details, err error) {
	spc, err := spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get space service: %w", err)
	}

	dateTypeId, err := spc.GetTypeIdByKey(ctx, bundle.TypeKeyDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get date type id: %w", err)
	}

	dateSource := sourceimpl.NewDate(sourceimpl.DateSourceParams{
		Id: domain.FullID{
			SpaceID:  spaceId,
			ObjectID: dateutil.NewDateObject(time.Unix(timestamp, 0), false).Id(),
		},
		DateObjectTypeId: dateTypeId,
	})

	detailsGetter, ok := dateSource.(sourceimpl.SourceIdEndodedDetails)
	if !ok {
		return nil, fmt.Errorf("date object does not implement SourceIdEndodedDetails: %w", err)
	}
	return detailsGetter.DetailsFromId()
}

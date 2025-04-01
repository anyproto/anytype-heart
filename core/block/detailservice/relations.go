package detailservice

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/dateutil"
	timeutil "github.com/anyproto/anytype-heart/util/time"
)

func (s *service) ListRelationsWithValue(spaceId string, value domain.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error) {
	var (
		countersByKeys     = make(map[domain.RelationKey]int64)
		detailHandlesValue = generateFilter(value)
	)

	err := s.store.SpaceIndex(spaceId).QueryIterate(
		database.Query{Filters: nil},
		func(details *domain.Details) {
			for key, valueToCheck := range details.Iterate() {
				if detailHandlesValue(valueToCheck) {
					if counter, ok := countersByKeys[key]; ok {
						countersByKeys[key] = counter + 1
					} else {
						countersByKeys[key] = 1
					}
				}
			}
		})

	if err != nil {
		return nil, fmt.Errorf("failed to query objects: %w", err)
	}

	keys := maps.Keys(countersByKeys)
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == bundle.RelationKeyMentions {
			return true
		}
		if keys[j] == bundle.RelationKeyMentions {
			return false
		}
		return keys[i] < keys[j]
	})

	list := make([]*pb.RpcRelationListWithValueResponseResponseItem, len(keys))
	for i, key := range keys {
		list[i] = &pb.RpcRelationListWithValueResponseResponseItem{
			RelationKey: string(key),
			Counter:     countersByKeys[key],
		}
	}

	return list, nil
}

func generateFilter(value domain.Value) func(v domain.Value) bool {
	equalOrHasFilter := func(v domain.Value) bool {
		if list, ok := v.TryListValues(); ok {
			for _, element := range list {
				if element.Equal(value) {
					return true
				}
			}
		}
		return v.Equal(value)
	}

	stringValue := value.String()
	if stringValue == "" {
		return equalOrHasFilter
	}

	sbt, err := typeprovider.SmartblockTypeFromID(stringValue)
	if err != nil {
		log.Error("failed to determine smartblock type", zap.Error(err))
	}

	if sbt != coresb.SmartBlockTypeDate {
		return equalOrHasFilter
	}

	// date object section

	dateObject, err := dateutil.BuildDateObjectFromId(stringValue)
	if err != nil {
		log.Error("failed to parse Date object id", zap.Error(err))
		return equalOrHasFilter
	}

	start := timeutil.CutToDay(dateObject.Time())
	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	startDateObject := dateutil.NewDateObject(start, false)
	shortId := startDateObject.Id()

	// filter for date objects is able to find relations with values between the borders of queried day
	// - for relations with number format it checks timestamp value is between timestamps of this day midnights
	// - for relations carrying string list it checks if some of the strings has day prefix, e.g.
	// if _date_2023-12-12-08-30-50Z-0200 is queried, then all relations with prefix _date_2023-12-12 will be returned
	return func(v domain.Value) bool {
		numberValue := v.Int64()
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}

		for _, element := range v.WrapToList() {
			if element.Equal(value) {
				return true
			}
			if strings.HasPrefix(element.String(), shortId) {
				return true
			}
		}

		return v.Equal(value)
	}
}

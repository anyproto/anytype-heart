package date

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

const spaceId = "space1"

func TestBuildDetailsFromTimestamp(t *testing.T) {
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyDate).Return(bundle.TypeKeyDate.URL(), nil)
	spcService := mock_space.NewMockService(t)
	spcService.EXPECT().Get(mock.Anything, mock.Anything).Return(spc, nil)

	for _, ts := range []int64{0, 1720035620, 1731099336,
		time.Date(2024, time.November, 30, 23, 55, 55, 0, time.UTC).Unix(),
		time.Date(2024, time.November, 30, 23, 55, 55, 0, time.FixedZone("Berlin", +2*60*60)).Unix(),
	} {
		t.Run("date object details - "+strconv.FormatInt(ts, 10), func(t *testing.T) {
			// when
			details, err := BuildDetailsFromTimestamp(nil, spcService, spaceId, ts)

			// then
			assert.NoError(t, err)
			assert.Equal(t, spaceId, details.GetString(bundle.RelationKeySpaceId))
			assert.Equal(t, bundle.TypeKeyDate.URL(), details.GetString(bundle.RelationKeyType))

			dateObject := dateutil.NewDateObject(time.Unix(ts, 0), false)
			assert.Equal(t, dateObject.Id(), details.GetString(bundle.RelationKeyId))
			assert.Equal(t, dateObject.Name(), details.GetString(bundle.RelationKeyName))
			ts2 := details.GetInt64(bundle.RelationKeyTimestamp)
			tt := time.Unix(ts2, 0)
			assert.Zero(t, tt.Hour())
			assert.Zero(t, tt.Minute())
			assert.Zero(t, tt.Second())
			assert.Len(t, details.GetInt64List(bundle.RelationKeyRestrictions), 9)
		})
	}
}

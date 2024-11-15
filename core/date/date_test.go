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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
			assert.Equal(t, spaceId, pbtypes.GetString(details, bundle.RelationKeySpaceId.String()))
			assert.Equal(t, bundle.TypeKeyDate.URL(), pbtypes.GetString(details, bundle.RelationKeyType.String()))

			tt := time.Unix(ts, 0)
			assert.Equal(t, dateutil.TimeToDateId(tt, false), pbtypes.GetString(details, bundle.RelationKeyId.String()))
			assert.Equal(t, dateutil.TimeToDateName(tt), pbtypes.GetString(details, bundle.RelationKeyName.String()))
			ts2 := pbtypes.GetInt64(details, bundle.RelationKeyTimestamp.String())
			tt2 := time.Unix(ts2, 0)
			assert.Zero(t, tt2.Hour())
			assert.Zero(t, tt2.Minute())
			assert.Zero(t, tt2.Second())
		})
	}
}

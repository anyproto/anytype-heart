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

	for _, ts := range []int64{0, 1720035620, 1731099336} {
		t.Run("date object details - "+strconv.FormatInt(ts, 10), func(t *testing.T) {
			details, err := BuildDetailsFromTimestamp(nil, spcService, spaceId, ts)
			assert.NoError(t, err)
			assert.Equal(t, spaceId, pbtypes.GetString(details, bundle.RelationKeySpaceId.String()))
			tt := time.Unix(ts, 0)
			assert.Equal(t, dateutil.TimeToDateId(tt), pbtypes.GetString(details, bundle.RelationKeyId.String()))
			assert.Equal(t, dateutil.TimeToDateName(tt, nil), pbtypes.GetString(details, bundle.RelationKeyName.String()))
			assert.Equal(t, bundle.TypeKeyDate.URL(), pbtypes.GetString(details, bundle.RelationKeyType.String()))
			assert.Equal(t, ts, pbtypes.GetInt64(details, bundle.RelationKeyTimestamp.String()))
		})
	}
}
